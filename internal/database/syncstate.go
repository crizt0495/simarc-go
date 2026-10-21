package database

import (
	"time"

	"arsippro/internal/models"
)

// ── SYNC STATE (watermark) + META helpers ─────────────────────────────────────

// GetSyncState membuat (bila belum ada) dan mengembalikan record watermark tabel.
func GetSyncState(tableName string) models.SyncState {
	var st models.SyncState
	if err := DB.Where("table_name = ?", tableName).First(&st).Error; err != nil {
		st = models.SyncState{Table: tableName}
		DB.Create(&st)
	}
	return st
}

// SaveSyncState menyimpan watermark push/pull tabel (nil = biarkan).
func SaveSyncState(tableName string, pushAt, pullAt *time.Time) {
	st := GetSyncState(tableName)
	if pushAt != nil {
		st.LastPushAt = *pushAt
	}
	if pullAt != nil {
		st.LastPullAt = *pullAt
	}
	DB.Save(&st)
}

// GetSyncMeta membaca nilai metadata sync_state (kolom Meta).
func GetSyncMeta(key string) string {
	var st models.SyncState
	if err := DB.Where("table_name = ?", "__meta:"+key).First(&st).Error; err == nil {
		return st.Meta
	}
	return ""
}

// MarkLocalBootstrap mencatat batas data seed (bootstrap): hanya perubahan
// nyata dari pengguna setelah momen ini yang diikutkan rescan push.
func MarkLocalBootstrap(at time.Time) {
	SaveSyncState("_bootstrap", &at, nil)
	if GetSyncMeta("bootstrap_at") == "" {
		SetSyncMeta("bootstrap_at", at.Format(time.RFC3339))
	}
}

// LocalBootstrapAt mengembalikan waktu bootstrap lokal (zero bila belum ada).
func LocalBootstrapAt() time.Time {
	return GetSyncState("_bootstrap").LastPushAt
}

// SetSyncMeta menyimpan nilai metadata sync_state (kolom Meta).
func SetSyncMeta(key, value string) {
	st := GetSyncState("__meta:" + key)
	st.Meta = value
	DB.Save(&st)
}
