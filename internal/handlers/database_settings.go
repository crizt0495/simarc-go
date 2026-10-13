package handlers

import (
	"log"
	"net"
	"net/http"
	"strings"

	"arsippro/internal/config"
	"arsippro/internal/database"
	"arsippro/internal/middleware"
	"arsippro/internal/services"

	"github.com/gin-gonic/gin"
)

// dbSettingsAllowed reports whether the current request may manage database
// settings.
//
//   - Normal mode (DB connected): only logged-in admins.
//   - Recovery mode (DB unreachable): the app is unusable and nobody can log
//     in, so the connection MAY be fixed — but only from the local machine
//     (loopback), so a remote LAN client cannot hijack the configuration.
func dbSettingsAllowed(c *gin.Context) bool {
	if database.Connected() {
		user := middleware.GetCurrentUser(c)
		return user != nil && user.IsAdmin()
	}
	return isLoopbackClient(c)
}

// isLoopbackClient reports whether the request originated from this machine.
//
// It inspects the raw TCP peer (RemoteAddr) rather than c.ClientIP(), because
// the latter honours X-Forwarded-For and could be spoofed by a remote LAN
// client to bypass the recovery-mode restriction.
func isLoopbackClient(c *gin.Context) bool {
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		host = c.Request.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// DatabaseSetupPage renders the standalone database connection form. It is
// reachable WITHOUT login so that it also works when the database is
// unreachable (recoveryGuard keeps this path open in recovery mode).
func (h *PengaturanAdvancedHandler) DatabaseSetupPage(c *gin.Context) {
	Render(c, http.StatusOK, "auth/database-setup.html", gin.H{
		"title":     "Konfigurasi Database - SIMARC",
		"pageTitle": "Konfigurasi Database",
		"DBInfo": gin.H{
			"Host": config.App.DBHost,
			"Port": config.App.DBPort,
			"Name": config.App.DBName,
			"User": config.App.DBUser,
		},
	})
}

// dbSettingsRedirect returns where to send the user after a save attempt.
func dbSettingsRedirect(_ *gin.Context) string {
	return "/pengaturan"
}

// DatabaseTest tests a database connection with the submitted settings
// without saving anything. Returns JSON for the AJAX "Uji Koneksi" button.
func (h *PengaturanAdvancedHandler) DatabaseTest(c *gin.Context) {
	if !dbSettingsAllowed(c) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "Akses ditolak."})
		return
	}

	host := strings.TrimSpace(c.PostForm("db_host"))
	port := strings.TrimSpace(c.PostForm("db_port"))
	name := strings.TrimSpace(c.PostForm("db_database"))
	user := strings.TrimSpace(c.PostForm("db_username"))
	pass := c.PostForm("db_password")

	if host == "" || name == "" || user == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Host, nama database, dan username wajib diisi."})
		return
	}
	if port == "" {
		port = "3306"
	}

	if err := database.TestConnection(host, port, name, user, pass); err != nil {
		// Distinguish "server reachable but database missing" from a genuine
		// connection failure, so the user knows saving will create it.
		if rerr := database.ServerReachable(host, port, user, pass); rerr == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "Server database terjangkau, tetapi database \"" + name + "\" belum ada. Simpan untuk membuatnya otomatis."})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Gagal terhubung: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Koneksi berhasil! Server database dapat dijangkau dan database " + name + " ada."})
}

// DatabaseSave persists new database settings to .env and reconnects the app
// immediately (no restart required). The new database is tested first; on
// success the old connection is swapped live.
func (h *PengaturanAdvancedHandler) DatabaseSave(c *gin.Context) {
	if !dbSettingsAllowed(c) {
		middleware.SetFlash(c, "error", "Akses ditolak.")
		c.Redirect(http.StatusFound, dbSettingsRedirect(c))
		return
	}
	redirectTo := dbSettingsRedirect(c)

	host := strings.TrimSpace(c.PostForm("db_host"))
	port := strings.TrimSpace(c.PostForm("db_port"))
	name := strings.TrimSpace(c.PostForm("db_database"))
	user := strings.TrimSpace(c.PostForm("db_username"))
	pass := c.PostForm("db_password")
	clearPass := c.PostForm("db_password_clear") == "on"

	if host == "" || name == "" || user == "" {
		middleware.SetFlash(c, "error", "Host, nama database, dan username wajib diisi.")
		c.Redirect(http.StatusFound, redirectTo)
		return
	}
	if port == "" {
		port = "3306"
	}
	// "Clear password" forces an empty password; otherwise an empty field
	// means keep the current password.
	if clearPass {
		pass = ""
	} else if pass == "" {
		pass = config.App.DBPass
	}

	// Test the new settings first — never switch to a broken connection.
	// If the database simply does not exist yet (fresh MySQL/MariaDB install),
	// create it automatically when the credentials allow it, then retry.
	if err := database.TestConnection(host, port, name, user, pass); err != nil {
		if cerr := database.EnsureDatabase(host, port, name, user, pass); cerr != nil {
			middleware.SetFlash(c, "error", "Koneksi ke database baru gagal, pengaturan TIDAK disimpan: "+err.Error())
			c.Redirect(http.StatusFound, redirectTo)
			return
		}
		if err2 := database.TestConnection(host, port, name, user, pass); err2 != nil {
			middleware.SetFlash(c, "error", "Database \""+name+"\" dibuat, tetapi masih gagal terhubung: "+err2.Error())
			c.Redirect(http.StatusFound, redirectTo)
			return
		}
		log.Printf("Database %q dibuat otomatis dan berhasil terhubung", name)
	}

	// Remember the previous values so we can roll back on failure
	oldValues := map[string]string{
		"DB_HOST":     config.App.DBHost,
		"DB_PORT":     config.App.DBPort,
		"DB_DATABASE": config.App.DBName,
		"DB_USERNAME": config.App.DBUser,
		"DB_PASSWORD": config.App.DBPass,
	}
	newValues := map[string]string{
		"DB_HOST":     host,
		"DB_PORT":     port,
		"DB_DATABASE": name,
		"DB_USERNAME": user,
		"DB_PASSWORD": pass,
	}

	// Apply to in-memory config
	config.App.DBHost = host
	config.App.DBPort = port
	config.App.DBName = name
	config.App.DBUser = user
	config.App.DBPass = pass

	// Persist to .env (also updates process env used by mysqldump backup)
	if err := config.UpdateEnv(newValues); err != nil {
		middleware.SetFlash(c, "error", "Pengaturan database diubah di memori, tetapi gagal menyimpan file .env: "+err.Error())
		c.Redirect(http.StatusFound, redirectTo)
		return
	}

	// Swap the live connection (runs migration + seed for the new database)
	if err := database.Reconnect(); err != nil {
		// Roll everything back so config, .env, and the live connection agree
		config.App.DBHost = oldValues["DB_HOST"]
		config.App.DBPort = oldValues["DB_PORT"]
		config.App.DBName = oldValues["DB_DATABASE"]
		config.App.DBUser = oldValues["DB_USERNAME"]
		config.App.DBPass = oldValues["DB_PASSWORD"]
		_ = config.UpdateEnv(oldValues)
		middleware.SetFlash(c, "error", "Pengaturan TIDAK diterapkan: gagal menyambungkan ulang. Konfigurasi sebelumnya dipulihkan. Detail: "+err.Error())
		c.Redirect(http.StatusFound, redirectTo)
		return
	}

	// Bring background services up if the app started in recovery mode
	services.InitQueue(3, services.ProcessJob)
	services.StartAutoDisposal()

	if user := middleware.GetCurrentUser(c); user != nil {
		logActivity(user.ID, "database_settings", "Mengganti pengaturan database ke "+host+":"+port+"/"+name, "settings", "database", c.ClientIP(), c.GetHeader("User-Agent"))
	}
	middleware.SetFlash(c, "success", "Pengaturan database disimpan dan diterapkan. Terhubung ke "+name+" ("+host+":"+port+").")
	c.Redirect(http.StatusFound, "/pengaturan")
}
