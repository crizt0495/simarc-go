package database

import (
	"fmt"
	"log"

	"gorm.io/gorm/logger"
)

// migrateSQLite menyiapkan skema database lokal SQLite (mode offline-first).
//
// Berbeda dengan jalur MySQL yang penuh perbaikan skema legacy, database
// SQLite selalu dibuat baru oleh AutoMigrate. Karena itu tidak ada
// information_schema, ALTER TABLE warisan, atau FULLTEXT di sini.
func migrateSQLite() error {
	DB.Logger = logger.Default.LogMode(logger.Error)

	log.Println("[MIGRASI] Menyiapkan skema SQLite...")
	if err := DB.AutoMigrate(allModels()...); err != nil {
		return err
	}

	// Tabel penghubung yang tidak punya model GORM (dipakai fitur pemusnahan).
	if err := DB.Exec(`CREATE TABLE IF NOT EXISTS pemusnahan_arsip_items (
		id TEXT NOT NULL PRIMARY KEY,
		pemusnahan_id TEXT NOT NULL,
		arsip_id TEXT NOT NULL,
		created_at DATETIME NULL
	)`).Error; err != nil {
		return err
	}

	// Indeks yang paling berpengaruh pada daftar/pencarian arsip. AutoMigrate
	// hanya membuat indeks dari tag model, sedangkan indeks performa ini
	// sebelumnya dibuat manual di MySQL.
	indexes := []struct {
		name  string
		table string
		cols  string
	}{
		{"idx_arsip_deleted_at", "arsip", "deleted_at"},
		{"idx_arsip_unit_kerja", "arsip", "unit_kerja_id"},
		{"idx_arsip_kode_klasifikasi", "arsip", "kode_klasifikasi_id"},
		{"idx_arsip_lokasi_arsip", "arsip", "lokasi_arsip_id"},
		{"idx_arsip_status", "arsip", "status_arsip"},
		{"idx_arsip_tanggal_dibuat", "arsip", "tanggal_dibuat"},
		{"idx_arsip_tanggal_retensi", "arsip", "tanggal_retensi_berakhir"},
		{"idx_arsip_pemberkasan", "arsip", "pemberkasan_id"},
		{"idx_arsip_created_at", "arsip", "created_at"},
		{"idx_arsip_uraian", "arsip", "uraian"},
		{"idx_login_logs_username", "login_logs", "username"},
		{"idx_login_logs_user", "login_logs", "user_id"},
		{"idx_activity_logs_user", "activity_logs", "user_id"},
		{"idx_activity_logs_created", "activity_logs", "created_at"},
		{"idx_pemusnahan_status", "pemusnahan_arsip", "status"},
		{"idx_pemusnahan_created", "pemusnahan_arsip", "created_at"},
		{"idx_arsip_versions_arsip", "arsip_versions", "arsip_id"},
		{"idx_peminjaman_status", "peminjaman_arsips", "status"},
		{"idx_peminjaman_arsip", "peminjaman_arsips", "arsip_id"},
		{"idx_peminjaman_user", "peminjaman_arsips", "user_id"},
		{"idx_blockchain_entity_type", "blockchain_audits", "entity_type"},
		{"idx_blockchain_entity_id", "blockchain_audits", "entity_id"},
		{"idx_pemusnahan_items_pemusnahan", "pemusnahan_arsip_items", "pemusnahan_id"},
		{"idx_pemusnahan_items_arsip", "pemusnahan_arsip_items", "arsip_id"},
	}
	for _, ix := range indexes {
		sql := fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s (%s)", ix.name, ix.table, ix.cols)
		if err := DB.Exec(sql).Error; err != nil {
			log.Printf("[MIGRASI] Lewati indeks %s: %v", ix.name, err)
		}
	}

	// Catat versi skema (dipakai jalur MySQL; di SQLite hanya informasi).
	DB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT NOT NULL PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	DB.Exec("INSERT OR IGNORE INTO schema_migrations (version) VALUES (?)", SchemaVersion)

	log.Println("[MIGRASI] Skema SQLite siap")
	return nil
}
