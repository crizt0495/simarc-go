package handlers

import (
	"net/http"

	"arsippro/internal/database"
	"arsippro/internal/desksync"

	"github.com/gin-gonic/gin"
)

// ── SINKRONISASI OFFLINE-FIRST ────────────────────────────────────────────────
//
// API kecil untuk badge status & tombol "Sinkron Sekarang" pada shell desktop:
//   GET  /api/sync/status — ikhtisar engine (online, pending, last sync)
//   POST /api/sync/run    — picu satu siklus sinkronisasi segera

type SyncHandler struct{}

func syncMode() string {
	if database.IsSQLite {
		return "local"
	}
	return "mysql"
}

// Status mengirim ikhtisar mesin sinkronisasi (selalu HTTP 200 agar badge
// dapat menampilkan kondisi apa pun, termasuk nonaktif).
func (h *SyncHandler) Status(c *gin.Context) {
	engine := desksync.Default()
	if engine == nil {
		c.JSON(http.StatusOK, gin.H{
			"enabled": false,
			"mode":    syncMode(),
			"online":  false,
			"pending": 0,
		})
		return
	}
	st := engine.Snapshot()
	st.Enabled = true
	st.Mode = syncMode()
	st.Pending = database.OutboxCountPending()
	c.JSON(http.StatusOK, st)
}

// Run memicu satu siklus sinkronisasi segera (non-blocking).
func (h *SyncHandler) Run(c *gin.Context) {
	engine := desksync.Default()
	if engine == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Sinkronisasi hanya aktif pada mode offline-first (DB_MODE=local).",
		})
		return
	}
	if engine.Snapshot().Running {
		c.JSON(http.StatusAccepted, gin.H{"status": "already_running", "message": "Sinkronisasi sedang berjalan."})
		return
	}
	engine.TriggerSync()
	c.JSON(http.StatusAccepted, gin.H{"status": "started", "message": "Sinkronisasi dimulai."})
}
