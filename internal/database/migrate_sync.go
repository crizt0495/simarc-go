package database

import (
	"log"
)

// ── MIGRASI KOLOM SINKRONISASI OFFLINE-FIRST (pusat MySQL) ────────────────────
//
// Kolom tambahan yang dipakai mesin sync dari desktop (DB_MODE=local):
//   arsip.file_content            — isi lampiran ikut sinkron (BLOB)
//   jenis_arsip.uuid              — identitas lintas-node untuk tabel uint-PK
//   pemusnahan_arsip_items.uuid   — idem
//   berita_acara_items.uuid       — idem
//   index arsip(updated_at)       — rescan watermark push/pull yang murah
//
// Idempotent: aman dijalankan tiap kali versi schema berubah.

// ensureSyncColumns dipanggil dari Migrate() (hanya jalur MySQL).
func ensureSyncColumns() {
	log.Println("[MIGRASI] Memeriksa kolom sinkronisasi offline-first...")

	// 1. Isi lampiran sebagai BLOB pada arsip (pusat menerima file klien).
	if tableExists("arsip") && !columnExists("arsip", "file_content") {
		if err := DB.Exec("ALTER TABLE arsip ADD COLUMN file_content LONGBLOB DEFAULT NULL").Error; err == nil {
			log.Println("[MIGRASI] Kolom arsip.file_content ditambahkan")
		} else {
			log.Printf("[MIGRASI] Gagal menambah file_content: %v", err)
		}
	}

	// 2. Identitas UUID utk tabel ber-PK auto-increment + backfill.
	ensureSyncUUIDColumn("jenis_arsip")
	ensureSyncUUIDColumn("pemusnahan_arsip_items")
	ensureSyncUUIDColumn("berita_acara_items")

	// 3. Index updated_at arsip agar rescan sync tidak table-scan.
	if tableExists("arsip") {
		var n int64
		DB.Raw("SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = 'arsip' AND index_name = 'idx_arsip_updated_at'").Scan(&n)
		if n == 0 {
			if err := DB.Exec("CREATE INDEX idx_arsip_updated_at ON arsip (updated_at)").Error; err == nil {
				log.Println("[MIGRASI] Index arsip(updated_at) ditambahkan")
			}
		}
	}
}

// ensureSyncUUIDColumn menambah kolom uuid CHAR(36) (bila belum ada) dan
// mengisi baris lama dengan UUID() MySQL.
func ensureSyncUUIDColumn(table string) {
	if !tableExists(table) {
		return
	}
	if !columnExists(table, "uuid") {
		if err := DB.Exec("ALTER TABLE " + table + " ADD COLUMN uuid CHAR(36) DEFAULT NULL").Error; err != nil {
			log.Printf("[MIGRASI] Gagal menambah kolom uuid di %s: %v", table, err)
			return
		}
		log.Printf("[MIGRASI] Kolom %s.uuid ditambahkan", table)
	}
	// Backfill baris lama: isi bila masih NULL (IIFE per-batch aman MySQL).
	var nulls int64
	DB.Raw("SELECT COUNT(1) FROM " + table + " WHERE uuid IS NULL").Scan(&nulls)
	if nulls > 0 {
		if err := DB.Exec("UPDATE " + table + " SET uuid = UUID() WHERE uuid IS NULL").Error; err == nil {
			log.Printf("[MIGRASI] %s: %d baris lama diisi UUID", table, nulls)
		} else {
			log.Printf("[MIGRASI] Backfill uuid %s gagal: %v", table, err)
		}
	}
}
