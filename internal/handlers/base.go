package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"math"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"arsippro"
	"arsippro/internal/config"
	"arsippro/internal/database"
	"arsippro/internal/middleware"
	"arsippro/internal/models"
	"arsippro/internal/services"

	"github.com/gin-gonic/gin"
)

// TemplateSets stores per-page template sets (layouts + page, isolated)
var TemplateSets map[string]*template.Template

// assetVersion is a stable cache-busting key derived from the content of the
// embedded static files. It changes ONLY when an asset actually changes, so
// browsers and the Vercel CDN can cache CSS/JS/images long-term (the ?v=
// query used to be time.Now().Unix(), which changed every second and defeated
// all caching — the main cause of slow repeat page loads).
var assetVersion = computeAssetVersion()

func computeAssetVersion() string {
	h := sha256.New()
	err := fs.WalkDir(arsippro.Embedded, "web/static", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		fmt.Fprintf(h, "%s:%d:", path, info.Size())
		if f, err := arsippro.Embedded.Open(path); err == nil {
			// Hash only headers for large binaries to keep startup fast; sizes
			// are already part of the digest so renames/replacements still bust.
			buf := make([]byte, 4096)
			n, _ := f.Read(buf)
			h.Write(buf[:n])
			f.Close()
		}
		return nil
	})
	if err != nil {
		return "dev"
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// Render renders a template with base data injected
func Render(c *gin.Context, status int, tmpl string, data gin.H) {
	if data == nil {
		data = gin.H{}
	}

	// Inject current user
	if user, ok := c.Get("user"); ok {
		data["AuthUser"] = user
	}

	// Inject flash messages
	data["Success"] = middleware.GetFlash(c, "success")
	data["Error"] = middleware.GetFlash(c, "error")
	data["Errors"] = middleware.GetFlash(c, "errors")

	// Old input
	if old, ok := c.Get("old_input"); ok {
		data["Old"] = old
	}

	data["CSRFToken"] = middleware.GetCSRFToken(c)
	data["AppName"] = config.App.AppName
	data["AppURL"] = strings.TrimRight(config.App.AppURL, "/")
	data["Year"] = time.Now().Year()
	data["CurrentPath"] = c.Request.URL.Path
	data["AssetVersion"] = assetVersion

	// Use page-specific template set (isolated {{define "content"}})
	if ts, ok := TemplateSets[tmpl]; ok {
		var buf bytes.Buffer
		if err := ts.ExecuteTemplate(&buf, tmpl, data); err != nil {
			log.Printf("TEMPLATE ERROR for %s: %s", tmpl, err.Error())
			c.String(http.StatusInternalServerError, "Template error: %s", err.Error())
			return
		}
		c.Data(status, "text/html; charset=utf-8", buf.Bytes())
		return
	}

	c.HTML(status, tmpl, data)
}

// RedirectWithSuccess redirects with a success flash
func RedirectWithSuccess(c *gin.Context, url, message string) {
	middleware.SetFlash(c, "success", message)
	c.Redirect(http.StatusFound, url)
}

// RedirectWithError redirects with an error flash
func RedirectWithError(c *gin.Context, url, message string) {
	middleware.SetFlash(c, "error", message)
	c.Redirect(http.StatusFound, url)
}

// Abort403 returns 403 page
func Abort403(c *gin.Context) {
	c.HTML(http.StatusForbidden, "errors/403.html", gin.H{"title": "Akses Ditolak"})
	c.Abort()
}

// Render404 renders a 404 page
func Render404(c *gin.Context) {
	Render(c, http.StatusNotFound, "errors/404.html", gin.H{"title": "Halaman Tidak Ditemukan"})
}

// HasPermission checks if current user has a permission
func HasPermission(c *gin.Context, perm string) bool {
	user := middleware.GetCurrentUser(c)
	if user == nil {
		return false
	}
	return userHasPerm(user, perm)
}

func userHasPerm(user *models.User, perm string) bool {
	if user.IsAdmin() {
		return true
	}
	var count int64
	database.DB.Table("permission_role").
		Joins("JOIN permissions ON permission_role.permission_id = permissions.id").
		Where("permission_role.role_id = ? AND permissions.name = ? AND permissions.is_active = 1",
			user.RoleID, perm).
		Count(&count)
	return count > 0
}

// HasAnyPermission checks if current user has any of the given permissions
func HasAnyPermission(c *gin.Context, perms ...string) bool {
	user := middleware.GetCurrentUser(c)
	if user == nil {
		return false
	}
	if user.IsAdmin() {
		return true
	}
	var count int64
	database.DB.Table("permission_role").
		Joins("JOIN permissions ON permission_role.permission_id = permissions.id").
		Where("permission_role.role_id = ? AND permissions.name IN ? AND permissions.is_active = 1",
			user.RoleID, perms).
		Count(&count)
	return count > 0
}

// GetLANIP returns the local LAN IP address by checking the preferred
// outbound interface. Falls back to 127.0.0.1 if unavailable.
func GetLANIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	addr := conn.LocalAddr().(*net.UDPAddr)
	if addr.IP.IsLoopback() {
		return "127.0.0.1"
	}
	return addr.IP.String()
}

// TemplateFuncs returns common template functions
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"SheetsWebURL": func(m models.Integration) string {
			return services.SheetsWebURL(&m)
		},
		"add": func(a, b interface{}) int {
			ai, aok := toInt(a)
			bi, bok := toInt(b)
			if !aok || !bok {
				return 0
			}
			return ai + bi
		},
		"sub": func(a, b interface{}) int {
			ai, aok := toInt(a)
			bi, bok := toInt(b)
			if !aok || !bok {
				return 0
			}
			return ai - bi
		},
		"mul": func(a, b int) int { return a * b },
		"div": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"int": func(v interface{}) int {
			switch n := v.(type) {
			case int:
				return n
			case int64:
				return int(n)
			case float64:
				return int(n)
			}
			return 0
		},
		"percent": func(part, total int) float64 {
			if total == 0 {
				return 0
			}
			return math.Round(float64(part)/float64(total)*1000) / 10
		},
		"formatDate": func(t interface{}) string {
			if t == nil {
				return "-"
			}
			switch v := t.(type) {
			case *time.Time:
				if v == nil {
					return "-"
				}
				return v.Format("02 Jan 2006")
			case time.Time:
				return v.Format("02 Jan 2006")
			}
			return "-"
		},
		"formatDateTime": func(t interface{}) string {
			if t == nil {
				return "-"
			}
			switch v := t.(type) {
			case *time.Time:
				if v == nil {
					return "-"
				}
				return v.Format("02 Jan 2006 15:04")
			case time.Time:
				return v.Format("02 Jan 2006 15:04")
			}
			return "-"
		},
		"formatDateInput": func(t *time.Time) string {
			if t == nil {
				return ""
			}
			return t.Format("2006-01-02")
		},
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
		"derefUint": func(u *uint) uint {
			if u == nil {
				return 0
			}
			return *u
		},
		"slice": func(s string, start, end int) string {
			r := []rune(s)
			if start >= len(r) {
				return ""
			}
			if end > len(r) {
				end = len(r)
			}
			return string(r[start:end])
		},
		"now":  time.Now,
		"year": func() int { return time.Now().Year() },
		"eq":   func(a, b interface{}) bool { return a == b },
		"ne":   func(a, b interface{}) bool { return a != b },
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
		"dict": func(values ...interface{}) map[string]interface{} {
			m := make(map[string]interface{})
			for i := 0; i < len(values)-1; i += 2 {
				if key, ok := values[i].(string); ok {
					m[key] = values[i+1]
				}
			}
			return m
		},
		"rupiahFormat": func(v float64) string {
			return "Rp " + formatNumber(v)
		},
		"formatNumber": func(v interface{}) string {
			switch n := v.(type) {
			case float64:
				return formatNumber(n)
			case *float64:
				if n == nil {
					return "0"
				}
				return formatNumber(*n)
			case int:
				return formatNumber(float64(n))
			case int64:
				return formatNumber(float64(n))
			}
			return "0"
		},
		"truncate": func(s string, n int) string {
			r := []rune(s)
			if len(r) <= n {
				return s
			}
			return string(r[:n]) + "..."
		},
		"formatDateDMY": func(t interface{}) string {
			if t == nil {
				return "-"
			}
			switch v := t.(type) {
			case *time.Time:
				if v == nil {
					return "-"
				}
				return v.Format("02/01/2006")
			case time.Time:
				return v.Format("02/01/2006")
			}
			return "-"
		},
		"formatDateISO": func(t interface{}) string {
			if t == nil {
				return ""
			}
			switch v := t.(type) {
			case *time.Time:
				if v == nil {
					return ""
				}
				return v.Format("2006-01-02")
			case time.Time:
				return v.Format("2006-01-02")
			}
			return ""
		},
		"formatTime": func(t interface{}) string {
			if t == nil {
				return "-"
			}
			switch v := t.(type) {
			case *time.Time:
				if v == nil {
					return "-"
				}
				return v.Format("15:04")
			case time.Time:
				return v.Format("15:04")
			}
			return "-"
		},
		"formatDateLong": func(t interface{}) string {
			if t == nil {
				return "-"
			}
			switch v := t.(type) {
			case *time.Time:
				if v == nil {
					return "-"
				}
				return v.Format("2 January 2006")
			case time.Time:
				return v.Format("2 January 2006")
			}
			return "-"
		},
		"formatFileSize": func(bytes interface{}) string {
			var b int64
			switch v := bytes.(type) {
			case int:
				b = int64(v)
			case int64:
				b = v
			case int32:
				b = int64(v)
			case uint:
				b = int64(v)
			case uint64:
				b = int64(v)
			case float64:
				b = int64(v)
			case float32:
				b = int64(v)
			default:
				return "0 B"
			}
			if b <= 0 {
				return "0 B"
			}
			var sizes = []string{"B", "KB", "MB", "GB", "TB"}
			var i int
			size := float64(b)
			for size >= 1024 && i < len(sizes)-1 {
				size /= 1024
				i++
			}
			return fmt.Sprintf("%.2f %s", size, sizes[i])
		},
		"replaceUnderscore": func(s string) string {
			return strings.ReplaceAll(s, "_", " ")
		},
		"ext": func(path string) string {
			return strings.TrimPrefix(filepath.Ext(path), ".")
		},
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,
		"contains":  strings.Contains,
		"toUpper":   strings.ToUpper,
		"toLower":   strings.ToLower,
		"lower":     strings.ToLower,
		"splitTags": func(s string) []string {
			if s == "" {
				return nil
			}
			parts := strings.Split(s, ",")
			var tags []string
			for _, p := range parts {
				t := strings.TrimSpace(p)
				if t != "" {
					tags = append(tags, t)
				}
			}
			return tags
		},
		"upper": func(s string) string { if s == "" { return "" }; return strings.ToUpper(s[:1]) + s[1:] },
		"upperFirst": func(s string) string { if s == "" { return "" }; return strings.ToUpper(s[:1]) + s[1:] },
		"split":     strings.Split,
		"join":      strings.Join,
		"replace":   strings.ReplaceAll,
		"inArray": func(item interface{}, arr interface{}) bool {
			switch v := arr.(type) {
			case []string:
				s, ok := item.(string)
				if !ok { return false }
				for _, x := range v { if x == s { return true } }
			case []interface{}:
				for _, x := range v { if x == item { return true } }
			}
			return false
		},
		"merge": func(key string, value interface{}, data interface{}) map[string]interface{} {
			result := make(map[string]interface{})
			if m, ok := data.(map[string]interface{}); ok {
				for k, v := range m {
					result[k] = v
				}
			}
			result[key] = value
			return result
		},
		"limitString": func(s string, n int) string {
			r := []rune(s)
			if len(r) <= n {
				return s
			}
			return string(r[:n]) + "..."
		},
		"highlight": func(text, keyword string) template.HTML {
			if keyword == "" {
				return template.HTML(template.HTMLEscapeString(text))
			}
			lower := strings.ToLower(text)
			kw := strings.ToLower(keyword)
			var result strings.Builder
			start := 0
			for {
				idx := strings.Index(lower[start:], kw)
				if idx == -1 {
					result.WriteString(template.HTMLEscapeString(text[start:]))
					break
				}
				pos := start + idx
				result.WriteString(template.HTMLEscapeString(text[start:pos]))
				result.WriteString("<mark class=\"bg-warning text-dark px-1 rounded\">")
				result.WriteString(template.HTMLEscapeString(text[pos:pos+len(kw)]))
				result.WriteString("</mark>")
				start = pos + len(kw)
			}
			return template.HTML(result.String())
		},
		"formatBytes": func(bytes int64) string {
			if bytes < 1024 {
				return fmt.Sprintf("%d B", bytes)
			} else if bytes < 1024*1024 {
				return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
			} else if bytes < 1024*1024*1024 {
				return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
			}
			return fmt.Sprintf("%.1f GB", float64(bytes)/(1024*1024*1024))
		},
		"default": func(d, v interface{}) interface{} {
			if v == nil {
				return d
			}
			switch s := v.(type) {
			case string:
				if s == "" {
					return d
				}
			case bool:
				if !s {
					return d
				}
			case int:
				if s == 0 {
					return d
				}
			case int64:
				if s == 0 {
					return d
				}
			case float64:
				if s == 0 {
					return d
				}
			}
			return v
		},
	}
}

func BuildPagination(currentPage, totalPages int, queryString string) template.HTML {
	if totalPages <= 1 {
		return ""
	}
	qs := queryString
	if qs != "" {
		qs = "&amp;" + qs
	}
	var buf strings.Builder
	buf.WriteString(`<nav aria-label="Page navigation"><ul class="pagination mb-0">`)

	// First + Prev
	if currentPage > 1 {
		buf.WriteString(fmt.Sprintf(`<li class="page-item"><a class="page-link" href="?page=1%s" title="Halaman Pertama"><i class="bi bi-chevron-double-left"></i></a></li>`, qs))
		buf.WriteString(fmt.Sprintf(`<li class="page-item"><a class="page-link" href="?page=%d%s" title="Sebelumnya"><i class="bi bi-chevron-left"></i></a></li>`, currentPage-1, qs))
	} else {
		buf.WriteString(`<li class="page-item disabled"><span class="page-link"><i class="bi bi-chevron-double-left"></i></span></li>`)
		buf.WriteString(`<li class="page-item disabled"><span class="page-link"><i class="bi bi-chevron-left"></i></span></li>`)
	}

	// Smart page numbers with ellipsis
	pages := buildPageNumbers(currentPage, totalPages)
	for _, p := range pages {
		if p == -1 {
			buf.WriteString(`<li class="page-item disabled"><span class="page-link">&hellip;</span></li>`)
		} else if p == currentPage {
			buf.WriteString(fmt.Sprintf(`<li class="page-item active"><span class="page-link">%d</span></li>`, p))
		} else {
			buf.WriteString(fmt.Sprintf(`<li class="page-item"><a class="page-link" href="?page=%d%s">%d</a></li>`, p, qs, p))
		}
	}

	// Next + Last
	if currentPage < totalPages {
		buf.WriteString(fmt.Sprintf(`<li class="page-item"><a class="page-link" href="?page=%d%s" title="Selanjutnya"><i class="bi bi-chevron-right"></i></a></li>`, currentPage+1, qs))
		buf.WriteString(fmt.Sprintf(`<li class="page-item"><a class="page-link" href="?page=%d%s" title="Halaman Terakhir"><i class="bi bi-chevron-double-right"></i></a></li>`, totalPages, qs))
	} else {
		buf.WriteString(`<li class="page-item disabled"><span class="page-link"><i class="bi bi-chevron-right"></i></span></li>`)
		buf.WriteString(`<li class="page-item disabled"><span class="page-link"><i class="bi bi-chevron-double-right"></i></span></li>`)
	}

	buf.WriteString(`</ul></nav>`)
	return template.HTML(buf.String())
}

// buildPageNumbers generates smart page numbers with ellipsis.
// Returns slice where -1 means "show ellipsis".
// Example for page 5 of 20: [1, -1, 3, 4, 5, 6, 7, -1, 20]
func buildPageNumbers(current, total int) []int {
	if total <= 7 {
		pages := make([]int, total)
		for i := range pages {
			pages[i] = i + 1
		}
		return pages
	}
	var pages []int
	// Always show first page
	pages = append(pages, 1)
	// Calculate window
	start := current - 1
	end := current + 1
	if start < 2 {
		start = 2
		end = start + 2
	}
	if end > total-1 {
		end = total - 1
		start = end - 2
	}
	if start > 2 {
		pages = append(pages, -1) // ellipsis
	}
	for i := start; i <= end; i++ {
		pages = append(pages, i)
	}
	if end < total-1 {
		pages = append(pages, -1) // ellipsis
	}
	// Always show last page
	pages = append(pages, total)
	return pages
}

func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case *int:
		if n != nil {
			return *n, true
		}
	case string:
		return 0, true
	}
	return 0, false
}

func formatNumber(v float64) string {
	s := ""
	n := int64(v)
	if n < 0 {
		s = "-"
		n = -n
	}
	str := ""
	for n > 0 {
		str = string(rune('0'+n%10)) + str
		n /= 10
		if n > 0 && len(str)%4 == 3 {
			str = "." + str
		}
	}
	if str == "" {
		str = "0"
	}
	return s + str
}
