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

// offsiteAllowed returns true when the current request may manage the offsite
// (cloud) backup settings: only logged-in admins.
func offsiteAllowed(c *gin.Context) bool {
	if !database.Connected() {
		return false
	}
	user := middleware.GetCurrentUser(c)
	return user != nil && user.IsAdmin()
}

const offsiteRedirect = "/pengaturan?tab=cloud"

func offsiteBoolStr(on bool) string {
	if on {
		return "true"
	}
	return "false"
}

func offsiteDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.TrimSpace(v)
}

// offsiteConfigFromForm builds an OffsiteConfig from posted form values,
// falling back to the saved values for blank secrets.
func offsiteConfigFromForm(c *gin.Context) config.OffsiteConfig {
	cur := config.LoadOffsite()
	cfg := cur

	cfg.DBHost = strings.TrimSpace(c.PostForm("offsite_db_host"))
	cfg.DBPort = offsiteDefault(c.PostForm("offsite_db_port"), "3306")
	cfg.DBName = strings.TrimSpace(c.PostForm("offsite_db_name"))
	cfg.DBUser = strings.TrimSpace(c.PostForm("offsite_db_user"))
	cfg.DBTLS = offsiteDefault(c.PostForm("offsite_db_tls"), "required")
	cfg.DBCAFile = strings.TrimSpace(c.PostForm("offsite_db_ca_file"))
	if p := c.PostForm("offsite_db_pass"); p != "" {
		cfg.DBPass = p
	}

	cfg.S3Endpoint = strings.TrimSpace(c.PostForm("offsite_s3_endpoint"))
	cfg.S3Region = offsiteDefault(c.PostForm("offsite_s3_region"), "auto")
	cfg.S3Bucket = strings.TrimSpace(c.PostForm("offsite_s3_bucket"))
	cfg.S3AccessKey = strings.TrimSpace(c.PostForm("offsite_s3_access_key"))
	cfg.S3Prefix = offsiteDefault(c.PostForm("offsite_s3_prefix"), "simarc")
	if s := c.PostForm("offsite_s3_secret_key"); s != "" {
		cfg.S3SecretKey = s
	}

	cfg.FilesMode = offsiteDefault(c.PostForm("offsite_files_mode"), "auto")
	cfg.RcloneRemote = strings.TrimSpace(c.PostForm("offsite_rclone_remote"))
	cfg.RclonePath = offsiteDefault(c.PostForm("offsite_rclone_path"), "simarc-backup")
	return cfg
}

// OffsiteTestDB tests the submitted cloud database settings without saving them.
func (h *PengaturanAdvancedHandler) OffsiteTestDB(c *gin.Context) {
	if !offsiteAllowed(c) {
		middleware.SetFlash(c, "error", "Akses ditolak.")
		c.Redirect(http.StatusFound, offsiteRedirect)
		return
	}
	cfg := offsiteConfigFromForm(c)
	if err := services.TestCloudDatabase(cfg); err != nil {
		middleware.SetFlash(c, "error", "Koneksi database cloud GAGAL: "+err.Error())
	} else {
		middleware.SetFlash(c, "success", "Koneksi database cloud BERHASIL ke "+cfg.DBHost+":"+cfg.DBPort+"/"+cfg.DBName)
	}
	c.Redirect(http.StatusFound, offsiteRedirect)
}

// OffsiteTestS3 tests the submitted object-storage settings without saving them.
func (h *PengaturanAdvancedHandler) OffsiteTestS3(c *gin.Context) {
	if !offsiteAllowed(c) {
		middleware.SetFlash(c, "error", "Akses ditolak.")
		c.Redirect(http.StatusFound, offsiteRedirect)
		return
	}
	cfg := offsiteConfigFromForm(c)
	if err := services.TestS3(cfg); err != nil {
		middleware.SetFlash(c, "error", "Koneksi object storage GAGAL: "+err.Error())
	} else {
		middleware.SetFlash(c, "success", "Koneksi object storage BERHASIL ke bucket "+cfg.S3Bucket)
	}
	c.Redirect(http.StatusFound, offsiteRedirect)
}

// OffsiteTestFiles tests the file-storage backend selected in the form: the
// S3-compatible bucket or the rclone remote (e.g. Google Drive).
func (h *PengaturanAdvancedHandler) OffsiteTestFiles(c *gin.Context) {
	if !offsiteAllowed(c) {
		middleware.SetFlash(c, "error", "Akses ditolak.")
		c.Redirect(http.StatusFound, offsiteRedirect)
		return
	}
	cfg := offsiteConfigFromForm(c)
	switch cfg.FileBackend() {
	case "s3":
		if err := services.TestS3(cfg); err != nil {
			middleware.SetFlash(c, "error", "Koneksi object storage GAGAL: "+err.Error())
		} else {
			middleware.SetFlash(c, "success", "Koneksi object storage BERHASIL ke bucket "+cfg.S3Bucket)
		}
	case "rclone":
		if err := services.TestRclone(cfg); err != nil {
			middleware.SetFlash(c, "error", "Koneksi rclone remote GAGAL: "+err.Error())
		} else {
			middleware.SetFlash(c, "success", "Koneksi rclone remote BERHASIL: "+strings.TrimRight(cfg.RcloneRemote, ":")+":"+strings.Trim(cfg.RclonePath, "/"))
		}
	default:
		middleware.SetFlash(c, "error", "Penyimpanan berkas belum dikonfigurasi (isi bucket S3 atau rclone remote).")
	}
	c.Redirect(http.StatusFound, offsiteRedirect)
}

// OffsiteSave persists the offsite backup settings to .env and (re)starts the
// scheduler so the new schedule takes effect without a restart.
func (h *PengaturanAdvancedHandler) OffsiteSave(c *gin.Context) {
	if !offsiteAllowed(c) {
		middleware.SetFlash(c, "error", "Akses ditolak.")
		c.Redirect(http.StatusFound, "/pengaturan")
		return
	}

	updates := map[string]string{
		"OFFSITE_ENABLED":       offsiteBoolStr(c.PostForm("offsite_enabled") == "on"),
		"OFFSITE_HOUR":          offsiteDefault(c.PostForm("offsite_hour"), "2"),
		"OFFSITE_MINUTE":        offsiteDefault(c.PostForm("offsite_minute"), "0"),
		"OFFSITE_BACKUP_DB":     offsiteBoolStr(c.PostForm("offsite_backup_db") == "on"),
		"OFFSITE_BACKUP_FILES":  offsiteBoolStr(c.PostForm("offsite_backup_files") == "on"),
		"OFFSITE_DB_HOST":       strings.TrimSpace(c.PostForm("offsite_db_host")),
		"OFFSITE_DB_PORT":       offsiteDefault(c.PostForm("offsite_db_port"), "3306"),
		"OFFSITE_DB_NAME":       strings.TrimSpace(c.PostForm("offsite_db_name")),
		"OFFSITE_DB_USER":       strings.TrimSpace(c.PostForm("offsite_db_user")),
		"OFFSITE_DB_TLS":        offsiteDefault(c.PostForm("offsite_db_tls"), "required"),
		"OFFSITE_DB_CA_FILE":    strings.TrimSpace(c.PostForm("offsite_db_ca_file")),
		"OFFSITE_S3_ENDPOINT":   strings.TrimSpace(c.PostForm("offsite_s3_endpoint")),
		"OFFSITE_S3_REGION":     offsiteDefault(c.PostForm("offsite_s3_region"), "auto"),
		"OFFSITE_S3_BUCKET":     strings.TrimSpace(c.PostForm("offsite_s3_bucket")),
		"OFFSITE_S3_ACCESS_KEY": strings.TrimSpace(c.PostForm("offsite_s3_access_key")),
		"OFFSITE_S3_PREFIX":     offsiteDefault(c.PostForm("offsite_s3_prefix"), "simarc"),
		"OFFSITE_S3_PATH_STYLE": offsiteBoolStr(c.PostForm("offsite_s3_path_style") == "on"),
		"OFFSITE_FILES_MODE":    offsiteDefault(c.PostForm("offsite_files_mode"), "auto"),
		"OFFSITE_RCLONE_REMOTE": strings.TrimSpace(c.PostForm("offsite_rclone_remote")),
		"OFFSITE_RCLONE_PATH":   offsiteDefault(c.PostForm("offsite_rclone_path"), "simarc-backup"),
	}
	// Blank secrets keep the current value (never echo secrets back to the form).
	if p := c.PostForm("offsite_db_pass"); p != "" {
		updates["OFFSITE_DB_PASS"] = p
	}
	if s := c.PostForm("offsite_s3_secret_key"); s != "" {
		updates["OFFSITE_S3_SECRET_KEY"] = s
	}

	if err := config.UpdateEnv(updates); err != nil {
		middleware.SetFlash(c, "error", "Gagal menyimpan pengaturan backup cloud: "+err.Error())
		c.Redirect(http.StatusFound, offsiteRedirect)
		return
	}

	services.StartOffsiteScheduler()

	if user := middleware.GetCurrentUser(c); user != nil {
		logActivity(user.ID, "offsite_settings", "Memperbarui pengaturan backup cloud (offsite)", "settings", "offsite", c.ClientIP(), c.GetHeader("User-Agent"))
	}
	middleware.SetFlash(c, "success", "Pengaturan backup cloud disimpan. Penjadwal harian diperbarui.")
	c.Redirect(http.StatusFound, offsiteRedirect)
}

// OffsiteRun runs an offsite backup immediately.
func (h *PengaturanAdvancedHandler) OffsiteRun(c *gin.Context) {
	if !offsiteAllowed(c) {
		middleware.SetFlash(c, "error", "Akses ditolak.")
		c.Redirect(http.StatusFound, offsiteRedirect)
		return
	}
	if services.OffsiteRunning() {
		middleware.SetFlash(c, "error", "Backup cloud sedang berjalan, tunggu hingga selesai.")
		c.Redirect(http.StatusFound, offsiteRedirect)
		return
	}

	res, err := services.RunOffsiteNow()
	if err != nil {
		middleware.SetFlash(c, "error", "Backup cloud gagal: "+err.Error())
		c.Redirect(http.StatusFound, offsiteRedirect)
		return
	}

	msg := "Backup cloud selesai."
	if res.Database.Attempted {
		msg += " Database: " + res.Database.Detail + "."
	}
	if res.Files.Attempted {
		msg += " File: " + res.Files.Detail + "."
	}
	if !res.Database.OK && res.Database.Attempted || !res.Files.OK && res.Files.Attempted {
		middleware.SetFlash(c, "error", msg)
	} else {
		middleware.SetFlash(c, "success", msg)
	}
	c.Redirect(http.StatusFound, offsiteRedirect)
}
