package handlers

import (
	"os"
	"strings"

	"arsippro/internal/config"
	"arsippro/internal/embeddeddb"
	"arsippro/internal/syncdb"

	"github.com/gin-gonic/gin"
)

// AppModeInfo returns a human-readable mode label and a CSS modifier class for
// the login page indicator.
//
// Possible values:
//
//	label                         class
//	"Klien ke server hub"         "client"
//	"Offline mandiri + Sinkron"   "offline-sync"
//	"Offline mandiri"             "offline"
//	"Server lokal"                "local"
func AppModeInfo() (label, class string) {
	// Mode klien: aplikasi hanya membuka jendela ke server pusat.
	if u := strings.TrimSpace(os.Getenv("SIMARC_SERVER_URL")); u != "" {
		return "Klien → " + u, "client"
	}

	// Mode offline mandiri: MariaDB portabel aktif.
	if embeddeddb.Enabled() {
		cfg := config.LoadSync()
		if cfg.Active() {
			return "Offline mandiri + Sinkron (" + cfg.NodeID + ")", "offline-sync"
		}
		return "Offline mandiri", "offline"
	}

	// Mode server lokal biasa.
	cfg := config.LoadSync()
	if cfg.Active() {
		return "Server lokal + Sinkron (" + cfg.NodeID + ")", "local-sync"
	}
	return "Server lokal", "local"
}

// LoginSyncInfo holds the condensed sync info shown on the login page.
type LoginSyncInfo struct {
	Enabled  bool   // SYNC_ENABLED=true dan hub dikonfigurasi
	NodeID   string // identitas simpul komputer ini
	HubHost  string // host hub
	LastSync string // "belum pernah" atau waktu sinkronisasi terakhir (WIB)
	LastOK   bool   // sinkronisasi terakhir sukses?
}

// GetSyncStatus returns a lightweight sync summary for the login page.
// Returns (info, true) bila sinkronisasi dikonfigurasi, otherwise (_, false).
func GetSyncStatus(c *gin.Context) (LoginSyncInfo, bool) {
	cfg := config.LoadSync()
	if !cfg.Active() {
		return LoginSyncInfo{}, false
	}

	info := LoginSyncInfo{
		Enabled: true,
		NodeID:  cfg.NodeID,
		HubHost: cfg.HubDBHost,
	}

	// Ambil status run terakhir (non-blocking: baca dari Manager.Last()).
	if last, ok := syncdb.Mgr.Last(); ok {
		info.LastSync = last.FinishedAt.Local().Format("02/01 15:04")
		info.LastOK = last.Errors == 0
	} else {
		info.LastSync = "belum pernah"
	}

	return info, true
}
