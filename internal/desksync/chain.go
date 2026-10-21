package desksync

import (
	"log"
	"os"
	"time"

	"arsippro/internal/database"
	"arsippro/internal/models"
	"arsippro/internal/services"

	"gorm.io/gorm"
)

// ── RINGKASAN RANTAI BLOCKCHAIN ──────────────────────────────────────────────
//
// Setiap siklus sync yang memindahkan data menambah SATU blok:
//   — di pusat, ketika ada push (jejak aktivitas klien masuk rantai global);
//   — di lokal, ketika ada pull (jejak pengambilan data pusat).
// Pendekatan per-siklus (bukan per-baris) menjaga rantai tetap ringkas.

func (e *Engine) writeChainSummary(remote, local *gorm.DB, pushed, pulled int) {
	if pushed > 0 {
		details := map[string]any{"client_id": e.ClientID, "pushed": pushed}
		if err := (&services.BlockchainAuditService{}).RecordAuditOn(
			remote, "sync", e.ClientID, "sync-push", details, "", "", "",
		); err != nil {
			log.Printf("[SYNC][blockchain] gagal menulis blok pusat: %v", err)
		}
	}
	if pulled > 0 {
		details := map[string]any{"client_id": e.ClientID, "pulled": pulled}
		// Reset UpdatedAt agar blok lokal tidak dipakai LWW (tabel ini
		// tidak ikut sync — aman).
		if err := (&services.BlockchainAuditService{}).RecordAuditOn(
			local, "sync", e.ClientID, "sync-pull", details, "", "", "",
		); err != nil {
			log.Printf("[SYNC][blockchain] gagal menulis blok lokal: %v", err)
		}
	}
}

// ── REKONSILIASI SEED LOKAL SETELAH SYNC PERTAMA ─────────────────────────────

// reconcileAfterFirstPull: ketika pusat juga memeliki akun `admin` (dari
// seed server), akun admin seed lokal (tanda bootstrap) dihapus agar tidak
// ada dua akun admin di daftar users; pengguna kemudian memakai akun pusat.
func (e *Engine) reconcileAfterFirstPull(local *gorm.DB) {
	if database.GetSyncMeta("local_admin_merged") == "1" {
		return
	}
	seedAdminID := database.GetSyncMeta("local_admin_id")
	if seedAdminID == "" {
		return
	}

	// Pusat dikatakan "menghadirkan user" bila baris users lokal melebihi
	// satu baris dengan username admin (seed + pusat punya id berbeda).
	var dualAdmin int64
	local.Model(&models.User{}).Where("username = ?", "admin").Count(&dualAdmin)
	var totalUsers int64
	local.Model(&models.User{}).Count(&totalUsers)

	if dualAdmin > 1 {
		// Hapus SEED lokal (id != id akun admin milik pusat yang termuat).
		var pulledAdmin models.User
		if err := local.Where("username = ? AND id != ?", "admin", seedAdminID).First(&pulledAdmin).Error; err == nil {
			if err := local.Unscoped().Delete(&models.User{}, seedAdminID).Error; err == nil {
				database.SetSyncMeta("local_admin_merged", "1")
				database.SetSyncMeta("leftover_admin_id", pulledAdmin.ID)
				log.Printf("[SYNC] Admin seed lokal digabung ke akun di pusat (%s)", pulledAdmin.ID)
			}
		}
		return
	}

	// Bila total users masih 1 dan adminnya seed lokal, tak ada apa-apa yang
	// harus dirapikan (pusat boleh saja belum punya akun admin).
	if totalUsers <= 1 {
		database.SetSyncMeta("local_admin_merged", "1")
	}
}

// ── FLAG ENV KECIL (penanda waktu sync terakhir untuk log/diagnostik) ────────

func markLastSyncEnv(t time.Time) {
	_ = os.Setenv("SIMARC_LAST_SYNC", t.Format(time.RFC3339))
}
