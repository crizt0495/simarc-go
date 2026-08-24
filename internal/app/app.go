// Package app wires together configuration, database, templates, and routes.
// It is shared between the standalone server binary (cmd/server) and the
// Vercel serverless adapter (api/index.go), guaranteeing identical behavior
// in both environments.
package app

import (
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"arsippro"
	"arsippro/internal/config"
	"arsippro/internal/database"
	"arsippro/internal/handlers"
	"arsippro/internal/middleware"
	"arsippro/internal/services"

	"github.com/gin-gonic/gin"
)

var (
	router      *gin.Engine
	initialized bool
)

// getProjectRoot determines the project root directory by looking at the
// executable location, handling both production binary and Air hot-reload modes.
func getProjectRoot() string {
	// Vercel: use /var/task which is the deployment directory
	if os.Getenv("VERCEL") == "1" {
		if _, err := os.Stat("/var/task/web/templates"); err == nil {
			return "/var/task"
		}
	}
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		if filepath.Base(dir) == "tmp" {
			dir = filepath.Dir(dir)
		}
		if _, err := os.Stat(filepath.Join(dir, "web", "static", "css")); err == nil {
			return dir
		}
	}
	wd, _ := os.Getwd()
	return wd
}

// listEmbeddedFiles returns file paths from the embedded FS under the given directories.
func listEmbeddedFiles(dirs ...string) []string {
	var result []string
	for _, dir := range dirs {
		entries, err := fs.ReadDir(arsippro.Embedded, dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".html") {
				result = append(result, filepath.Join(dir, entry.Name()))
			}
		}
	}
	return result
}

// serveStaticFile returns a Gin handler that serves static files with correct
// Content-Type and the supplied Cache-Control policy.
func serveStaticFile(dir string, cacheControl string) gin.HandlerFunc {
	return func(c *gin.Context) {
		file := c.Param("filepath")
		if file == "" || file == "/" {
			c.Status(http.StatusNotFound)
			return
		}
		if cacheControl != "" {
			c.Header("Cache-Control", cacheControl)
		}
		// Try embedded FS first (Vercel), then disk (local)
		embedPath := filepath.Join(dir, file)
		if f, err := arsippro.Embedded.Open(embedPath); err == nil {
			f.Close()
			c.FileFromFS(embedPath, http.FS(arsippro.Embedded))
			return
		}
		// Fallback to disk
		target := filepath.Join(dir, file)
		if !strings.HasPrefix(target, dir+string(filepath.Separator)) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		if _, err := os.Stat(target); os.IsNotExist(err) {
			c.Status(http.StatusNotFound)
			return
		}
		http.ServeFile(c.Writer, c.Request, target)
	}
}

// securityHeaders sets baseline response headers on every request.
func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		c.Next()
	}
}

// Init performs one-time bootstrap: config, database, migrations, session,
// queue workers, templates, and all routes. Safe to call multiple times;
// subsequent calls are no-ops (important for serverless warm invocations).
func Init() (*gin.Engine, error) {
	if initialized {
		return router, nil
	}

	config.Load()

	isVercel := os.Getenv("VERCEL") == "1"

	rootDir := getProjectRoot()
	if !isVercel {
		// On Vercel the working directory is already correct; chdir may fail on read-only FS.
		if err := os.Chdir(rootDir); err != nil {
			log.Printf("[WARN] Gagal mengubah working directory ke %s: %v", rootDir, err)
		}
	}
	log.Printf("Project root: %s", rootDir)

	if err := database.Connect(); err != nil {
		log.Printf("[WARN] Database tidak dapat dihubungkan: %v", err)
		log.Printf("[WARN] Server tetap berjalan; halaman akan menampilkan error 503 hingga koneksi database pulih.")
	} else {
		if err := database.Migrate(); err != nil {
			log.Printf("[WARN] Migration error: %v", err)
		} else {
			// SeedIfNeeded never overwrites existing users/passwords.
			database.SeedIfNeeded()
		}
	}
	middleware.InitSession()
	services.InitQueue(3, services.ProcessJob)

	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())
	r.MaxMultipartMemory = 32 << 20 // 32 MB
	r.Use(securityHeaders())

	buildTemplates(r)

	registerStaticRoutes(r)
	recoveryGuard(r)
	registerAppRoutes(r)

	router = r
	initialized = true
	return router, nil
}

// buildTemplates parses page templates either from embedded FS (Vercel) or disk.
//
// Performance: the shared layout/component files are parsed exactly ONCE into
// a base template; each page then CLONES that base and parses only its own
// file. Cloning shares the already-parsed trees, so cold-start cost drops from
// O(pages × shared files) full parses to 1 shared parse + O(pages) clones.
func buildTemplates(r *gin.Engine) {
	useEmbedded := false
	layoutFiles, _ := filepath.Glob("web/templates/layouts/*.html")
	compFiles, _ := filepath.Glob("web/templates/components/*.html")
	sharedFiles := append(layoutFiles, compFiles...)

	if len(sharedFiles) == 0 {
		sharedFiles = listEmbeddedFiles("web/templates/layouts", "web/templates/components")
		if len(sharedFiles) > 0 {
			useEmbedded = true
			log.Println("[INFO] Using embedded template files")
		} else {
			log.Println("[WARN] No template files found — template rendering may fail")
		}
	}

	handlers.TemplateSets = make(map[string]*template.Template)

	parseShared := func() (*template.Template, error) {
		if useEmbedded {
			return template.New("").Funcs(handlers.TemplateFuncs()).ParseFS(arsippro.Embedded, sharedFiles...)
		}
		return template.New("").Funcs(handlers.TemplateFuncs()).ParseFiles(sharedFiles...)
	}
	base, baseErr := parseShared()
	if baseErr != nil && !useEmbedded {
		// Disk mode used template.Must before; keep loud failure semantics.
		panic(baseErr)
	}
	if baseErr != nil {
		log.Printf("[WARN] Shared template error: %v", baseErr)
	}

	registerPage := func(ts *template.Template) {
		for _, t := range ts.Templates() {
			n := t.Name()
			if !strings.Contains(n, "/") {
				continue
			}
			if strings.HasPrefix(n, "layouts/") || strings.HasPrefix(n, "components/") {
				continue
			}
			handlers.TemplateSets[n] = ts
		}
	}

	pageDirs := []string{
		"auth", "errors", "dashboard", "arsip", "kode-klasifikasi",
		"unit-kerja", "lokasi-arsip", "jenis-arsip", "pemberkasan",
		"pemusnahan", "jadwal-retensi", "users", "role", "profil",
		"pengaturan", "search", "peminjaman",
		"laporan", "laporan/lokasi", "qrcode", "ocr", "blockchain",
		"backup", "monitoring", "disposal", "import-export",
		"integrations", "retention", "settings", "mobile", "supervision",
	}

	pagesParsed := 0
	if base != nil {
		for _, dir := range pageDirs {
			var pageFiles []string
			if useEmbedded {
				pageFiles = listEmbeddedFiles("web/templates/" + dir)
			} else {
				pageFiles, _ = filepath.Glob("web/templates/" + dir + "/*.html")
			}
			for _, pf := range pageFiles {
				ts, err := base.Clone()
				if err != nil {
					log.Printf("[WARN] Template clone error %s: %v", pf, err)
					continue
				}
				if useEmbedded {
					if _, err := ts.ParseFS(arsippro.Embedded, pf); err != nil {
						log.Printf("[WARN] Template parse error %s: %v", pf, err)
						continue
					}
				} else {
					if _, err := ts.ParseFiles(pf); err != nil {
						log.Printf("[WARN] Template parse error %s: %v", pf, err)
						continue
					}
				}
				registerPage(ts)
				pagesParsed++
			}
		}
	}
	log.Printf("[INFO] Templates ready: %d pages (shared layout parsed once)", pagesParsed)

	if len(sharedFiles) > 0 && base != nil {
		r.SetHTMLTemplate(base)
	}
}

// registerStaticRoutes mounts static asset handlers. CSS/JS/images are
// content-versioned via the ?v= query param, so they can be cached
// aggressively at the browser/CDN.
func registerStaticRoutes(r *gin.Engine) {
	const immutable = "public, max-age=31536000, immutable"
	r.GET("/css/*filepath", serveStaticFile("web/static/css", immutable))
	r.HEAD("/css/*filepath", serveStaticFile("web/static/css", immutable))
	r.GET("/js/*filepath", serveStaticFile("web/static/js", immutable))
	r.HEAD("/js/*filepath", serveStaticFile("web/static/js", immutable))
	r.GET("/images/*filepath", serveStaticFile("web/static/images", immutable))
	r.HEAD("/images/*filepath", serveStaticFile("web/static/images", immutable))
	r.GET("/storage/*filepath", serveStaticFile("public/storage", "public, max-age=3600"))
	r.HEAD("/storage/*filepath", serveStaticFile("public/storage", "public, max-age=3600"))
	r.GET("/sw.js", func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=0, must-revalidate")
		if f, err := arsippro.Embedded.Open("web/static/sw.js"); err == nil {
			f.Close()
			c.FileFromFS("/web/static/sw.js", http.FS(arsippro.Embedded))
			return
		}
		c.File("web/static/sw.js")
	})
	r.GET("/manifest.json", func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=3600")
		if f, err := arsippro.Embedded.Open("web/static/manifest.json"); err == nil {
			f.Close()
			c.FileFromFS("/web/static/manifest.json", http.FS(arsippro.Embedded))
			return
		}
		c.File("web/static/manifest.json")
	})
	r.GET("/favicon.ico", func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=604800")
		if f, err := arsippro.Embedded.Open("web/static/images/logo-icon.svg"); err == nil {
			f.Close()
			c.FileFromFS("/web/static/images/logo-icon.svg", http.FS(arsippro.Embedded))
			return
		}
		c.File("web/static/images/logo-icon.svg")
	})
}

// recoveryGuard returns HTTP 503 for every request when the database
// connection could not be established, except health endpoints.
//
// Unlike a hard 503, the guard first attempts to (re)connect when the live
// connection is missing. This makes the app self-heal: if the database was
// unreachable during cold start (serverless warm instance or a long-running
// server started before the DB), subsequent requests retry the connection
// instead of being locked out with 503 forever.
// Reconnection attempts are throttled to at most one per interval so requests
// do not pile up on the (capped) connection timeout while the DB is down.
func recoveryGuard(r *gin.Engine) {
	var (
		retryMu       sync.Mutex
		lastAttempt   time.Time
		retryInterval = 10 * time.Second
	)

	tryReconnect := func() {
		retryMu.Lock()
		defer retryMu.Unlock()
		if time.Since(lastAttempt) < retryInterval {
			return
		}
		lastAttempt = time.Now()
		if err := database.Connect(); err == nil {
			log.Println("[INFO] Database tersambung kembali (self-heal)")
			database.Migrate()
			database.SeedIfNeeded()
		}
	}

	r.Use(func(c *gin.Context) {
		if database.DB == nil && c.Request.URL.Path != "/health" && c.Request.URL.Path != "/ping" {
			tryReconnect()
		}
		if database.DB == nil {
			path := c.Request.URL.Path
			if path == "/health" || path == "/ping" {
				c.Next()
				return
			}
			handlers.Render(c, http.StatusServiceUnavailable, "errors/503.html", gin.H{
				"title":     "Database Tidak Tersedia",
				"pageTitle": "Database Tidak Tersedia",
				"message":   "Aplikasi tidak dapat terhubung ke database. Hubungi administrator.",
			})
			c.Abort()
			return
		}
		c.Next()
	})
}

// registerAppRoutes mounts all application routes.
func registerAppRoutes(r *gin.Engine) {
	registerRoutes(r)
}

// Handler is the net/http entry point usable by any hosting platform,
// including Vercel serverless functions.
func Handler(w http.ResponseWriter, req *http.Request) {
	r, err := Init()
	if err != nil {
		http.Error(w, fmt.Sprintf("Gagal menginisialisasi aplikasi: %v", err), http.StatusInternalServerError)
		return
	}
	r.ServeHTTP(w, req)
}
