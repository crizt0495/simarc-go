package handlers

import (
	"net/http"
	"strings"

	"arsippro/internal/config"
	"arsippro/internal/database"
	"arsippro/internal/middleware"
	"arsippro/internal/services"

	"github.com/gin-gonic/gin"
)

// dbSettingsAllowed returns true when the current request may manage the
// database settings: only logged-in admins.
func dbSettingsAllowed(c *gin.Context) bool {
	if !database.Connected() {
		return false
	}
	user := middleware.GetCurrentUser(c)
	return user != nil && user.IsAdmin()
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

	// Test the new settings first — never switch to a broken connection
	if err := database.TestConnection(host, port, name, user, pass); err != nil {
		middleware.SetFlash(c, "error", "Koneksi ke database baru gagal, pengaturan TIDAK disimpan: "+err.Error())
		c.Redirect(http.StatusFound, redirectTo)
		return
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
