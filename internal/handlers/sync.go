package handlers

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"arsippro/internal/config"
	"arsippro/internal/middleware"
	"arsippro/internal/syncdb"

	"github.com/gin-gonic/gin"
)

const syncRedirect = "/pengaturan?tab=sync"

// syncFormData is the (secret-free) synchronisation config shown in the form.
type syncFormData struct {
	Enabled         bool
	NodeID          string
	IntervalMinutes int
	HubHost         string
	HubPort         string
	HubDBName       string
	HubUser         string
	HubTLS          string

	HubURL       string
	HubTokenSet  bool
	FilesEnabled bool
	FilesMaxMB   int
}

func syncSettingsForTemplate() syncFormData {
	cfg := config.LoadSync()
	return syncFormData{
		Enabled:         cfg.Enabled,
		NodeID:          cfg.NodeID,
		IntervalMinutes: cfg.IntervalMinutes,
		HubHost:         cfg.HubDBHost,
		HubPort:         cfg.HubDBPort,
		HubDBName:       cfg.HubDBName,
		HubUser:         cfg.HubDBUser,
		HubTLS:          cfg.HubDBTLS,

		HubURL:       cfg.HubURL,
		HubTokenSet:  cfg.HubToken != "",
		FilesEnabled: cfg.FilesEnabled,
		FilesMaxMB:   cfg.FilesMaxMB,
	}
}

// SyncSave stores the multi-computer synchronisation settings in .env.
func (h *PengaturanAdvancedHandler) SyncSave(c *gin.Context) {
	if !offsiteAllowed(c) {
		middleware.SetFlash(c, "error", "Akses ditolak.")
		c.Redirect(http.StatusFound, syncRedirect)
		return
	}

	updates := map[string]string{
		"SYNC_ENABLED":          offsiteBoolStr(c.PostForm("sync_enabled") == "on"),
		"SYNC_NODE_ID":          strings.TrimSpace(c.PostForm("sync_node_id")),
		"SYNC_INTERVAL_MINUTES": offsiteDefault(c.PostForm("sync_interval_minutes"), "15"),
		"SYNC_HUB_DB_HOST":      strings.TrimSpace(c.PostForm("sync_hub_db_host")),
		"SYNC_HUB_DB_PORT":      offsiteDefault(c.PostForm("sync_hub_db_port"), "3306"),
		"SYNC_HUB_DB_NAME":      strings.TrimSpace(c.PostForm("sync_hub_db_name")),
		"SYNC_HUB_DB_USER":      strings.TrimSpace(c.PostForm("sync_hub_db_user")),
		"SYNC_HUB_DB_TLS":       offsiteDefault(c.PostForm("sync_hub_db_tls"), "preferred"),

		"SYNC_FILES_ENABLED": offsiteBoolStr(c.PostForm("sync_files_enabled") == "on"),
		"SYNC_HUB_URL":       strings.TrimRight(strings.TrimSpace(c.PostForm("sync_hub_url")), "/"),
		"SYNC_FILES_MAX_MB":  offsiteDefault(c.PostForm("sync_files_max_mb"), "128"),
	}
	// Blank password keeps the current value (never echo secrets back).
	if p := c.PostForm("sync_hub_db_pass"); p != "" {
		updates["SYNC_HUB_DB_PASS"] = p
	}
	if t := strings.TrimSpace(c.PostForm("sync_hub_token")); t != "" {
		updates["SYNC_HUB_TOKEN"] = t
	}

	if err := config.UpdateEnv(updates); err != nil {
		middleware.SetFlash(c, "error", "Gagal menyimpan pengaturan sinkronisasi: "+err.Error())
		c.Redirect(http.StatusFound, syncRedirect)
		return
	}

	syncdb.StartScheduler()

	if user := middleware.GetCurrentUser(c); user != nil {
		logActivity(user.ID, "sync_settings", "Memperbarui pengaturan sinkronisasi multi-komputer", "settings", "sync", c.ClientIP(), c.GetHeader("User-Agent"))
	}
	middleware.SetFlash(c, "success", "Pengaturan sinkronisasi disimpan.")
	c.Redirect(http.StatusFound, syncRedirect)
}

// SyncStatus reports the current multi-computer synchronisation state, including
// the node identity, hub target, and the most recent runs.
func SyncStatus(c *gin.Context) {
	st := syncdb.Mgr.Status(c.Request.Context())
	c.JSON(http.StatusOK, st)
}

// SyncRun starts a synchronisation in the background and returns immediately.
// The UI polls SyncStatus for progress and the result.
func SyncRun(c *gin.Context) {
	if syncdb.Mgr.Running() {
		c.JSON(http.StatusConflict, gin.H{"error": "sinkronisasi sedang berjalan"})
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if _, err := syncdb.Mgr.RunNow(ctx); err != nil && err != syncdb.ErrBusy {
			log.Printf("[sync] manual: %v", err)
		}
	}()
	c.JSON(http.StatusAccepted, gin.H{"status": "started"})
}
