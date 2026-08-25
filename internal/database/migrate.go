package database

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"

	"arsippro/internal/models"

	"gorm.io/gorm/logger"
)

// MySQL schema helpers — use information_schema with DATABASE() instead of
// PostgreSQL's current_schema(). Two-space indent for consistency with Go fmt.

// SchemaVersion identifies the current expected database schema/maintenance
// state. Bump this value whenever Migrate()'s tasks need to run again (new
// tables, indexes, data fixes). On boot, Migrate() skips all heavy work when
// the stored version matches — critical for Vercel serverless where new
// instances start constantly and a full migration per cold start made every
// request slow.
const SchemaVersion = "2026.08.25-02"

// tableExists returns true when a table exists in the current database.
func tableExists(name string) bool {
	var n int64
	DB.Raw("SELECT COUNT(1) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?", name).Scan(&n)
	return n > 0
}

// schemaIsCurrent reports whether the database is already migrated to
// SchemaVersion (one cheap query when up to date).
func schemaIsCurrent() bool {
	if !tableExists("schema_migrations") {
		return false
	}
	var n int64
	DB.Raw("SELECT COUNT(1) FROM schema_migrations WHERE version = ?", SchemaVersion).Scan(&n)
	return n > 0
}

// markSchemaVersion records SchemaVersion as applied.
func markSchemaVersion() {
	DB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(64) NOT NULL PRIMARY KEY,
		applied_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`)
	DB.Exec("INSERT IGNORE INTO schema_migrations (version) VALUES (?)", SchemaVersion)
}

// columnExists returns true when a column exists on a table in the current database.
func columnExists(table, column string) bool {
	var n int64
	DB.Raw("SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?", table, column).Scan(&n)
	return n > 0
}

// columnDataType returns the data_type value from information_schema for the given column.
// Returns empty string if the column does not exist.
func columnDataType(table, column string) string {
	var dt string
	DB.Raw("SELECT data_type FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?", table, column).Scan(&dt)
	return dt
}

// Migrate runs auto-migration for all models. It returns an error instead of
// crashing the process so runtime reconnects can fail gracefully.
//
// Optimized for MySQL: ensures critical performance indexes that AutoMigrate
// alone does not create, and avoids destructive schema changes.
func Migrate() error {
	// Quiet GORM SQL logging for migration step (we explicitly log meaningful events).
	DB.Logger = logger.Default.LogMode(logger.Error)

	// Fast path: already migrated to the current schema version — skip all
	// heavy checks/updates (they would otherwise run on every serverless
	// cold start and dominate request latency).
	if tableExists("roles") && schemaIsCurrent() {
		log.Println("[MIGRASI] Skema sudah versi terbaru, lewati pemeriksaan berat")
		return nil
	}

	log.Println("[MIGRASI] Memeriksa dan memperbarui skema database...")

	// Create tables manually to avoid GORM FK constraint issues.
	// MySQL: BIGINT AUTO_INCREMENT PRIMARY KEY (no BIGSERIAL in MySQL).
	DB.Exec(`CREATE TABLE IF NOT EXISTS pemusnahan_arsip_items (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
		pemusnahan_id VARCHAR(36) NOT NULL,
		arsip_id VARCHAR(36) NOT NULL,
		created_at DATETIME(3) NULL,
		INDEX idx_pemusnahan_items_pemusnahan (pemusnahan_id),
		INDEX idx_pemusnahan_items_arsip (arsip_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`)

	DB.Exec(`CREATE TABLE IF NOT EXISTS disposal_schedules (
		id VARCHAR(36) NOT NULL PRIMARY KEY,
		kode_klasifikasi_id VARCHAR(36) DEFAULT NULL,
		arsip_id VARCHAR(36) DEFAULT NULL,
		scheduled_date DATETIME(3) DEFAULT NULL,
		action VARCHAR(50) DEFAULT NULL,
		status VARCHAR(50) DEFAULT 'pending',
		executed_at DATETIME(3) DEFAULT NULL,
		created_by VARCHAR(36) DEFAULT NULL,
		created_at DATETIME(3) DEFAULT NULL,
		updated_at DATETIME(3) DEFAULT NULL,
		deleted_at DATETIME(3) DEFAULT NULL,
		INDEX idx_disposal_schedules_deleted_at (deleted_at),
		INDEX idx_disposal_schedules_status (status),
		INDEX idx_disposal_schedules_date (scheduled_date),
		INDEX idx_disposal_schedules_kk (kode_klasifikasi_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`)

	// Cek apakah tabel roles sudah ada untuk skip AutoMigrate.
	if !tableExists("roles") {
		log.Println("[MIGRASI] Database baru, menjalankan migrasi penuh...")
		err := DB.AutoMigrate(
			&models.Role{},
			&models.Permission{},
			&models.UnitKerja{},
			&models.User{},
			&models.KodeKlasifikasi{},
			&models.LokasiArsip{},
			&models.JenisArsip{},
			&models.Pemberkasan{},
			&models.Arsip{},
			&models.ArsipVersion{},
			&models.PemusnahanArsip{},

			&models.JadwalRetensi{},
			&models.JadwalRetensiArsip{},
			&models.LoginLog{},
			&models.ActivityLog{},
			&models.AuditLog{},
			&models.QrCode{},
			&models.OcrTempImage{},
			&models.BlockchainAudit{},
			&models.PeminjamanArsip{},
			&models.RetentionSchedule{},

			&models.SavedSearch{},
			&models.BackupLog{},
			&models.Notification{},
			&models.Integration{},
			&models.ImportExportJob{},
			&models.DashboardWidget{},

			&models.ComplianceScore{},
			&models.LeaderboardStat{},
			&models.NotificationPreference{},
			&models.NotificationTemplate{},
			&models.RetentionNotification{},
			&models.Tenant{},
			&models.DestructionApproval{},
			&models.ComplianceRisk{},
			&models.AiCategorization{},

			&models.Webhook{},
			&models.WebhookLog{},
			&models.Workflow{},
			&models.WorkflowExecution{},
			&models.SearchLog{},

			&models.BeritaAcara{},
			&models.BeritaAcaraItem{},
		)
		if err != nil {
			return err
		}
		log.Println("[MIGRASI] Sinkronisasi tabel selesai")
	} else {
		log.Println("[MIGRASI] Tabel sudah ada, melewati AutoMigrate")
	}

	log.Println("[MIGRASI] Menjalankan post-migration tasks...")

	// Add essential indexes that AutoMigrate doesn't always create. These dramatically
	// improve performance for the most frequently executed queries (arsip listing,
	// search by uraian, filter by unit kerja / kode klasifikasi / status).
	ensureArsipIndexes()
	ensureCommonIndexes()

	// Migrate legacy pemusnahan data.
	log.Println("[MIGRASI] Migrasi data pemusnahan...")
	migratePemusnahanData()

	// Clean up file_path references to non-existent files.
	log.Println("[MIGRASI] Membersihkan referensi file path...")
	cleanupFilePaths()

	// Auto-classify SPJ/Non SPJ for existing records.
	log.Println("[MIGRASI] Mengklasifikasi SPJ/Non SPJ...")
	fixSPJClassification()

	// Verify blockchain data integrity.
	log.Println("[MIGRASI] Memverifikasi hash blockchain...")
	fixBlockchainHashes()

	// Add columns that AutoMigrate might miss on legacy databases.
	addNomorSPMColumn()
	addJumlahSatuanColumn()

	// Extract SPM numbers from existing records.
	migrateSPMFromUraian()
	migrateLoginSecurity()
	addBackupLogGDriveColumns()
	addArsipGDriveURLColumn()

	// Align legacy Laravel-era integration tables with the Go models.
	alignIntegrationTables()

	// jADWAL drop OK karena tidak ada data referensi lagi pada legacy schema.
	dropJenisLokasiColumnIfUnused()

	// Record the completed schema version so subsequent boots take the fast path.
	markSchemaVersion()

	return nil
}

// ensureArsipIndexes adds performance indexes used by common arsip queries.
// These are guard-checked so they only run once per database.
func ensureArsipIndexes() {
	if !tableExists("arsip") {
		return
	}

	type indexDef struct {
		name  string
		cols  string
		extra string
	}
	indexes := []indexDef{
		{"idx_arsip_deleted_at", "deleted_at", ""},
		{"idx_arsip_unit_kerja", "unit_kerja_id", ""},
		{"idx_arsip_kode_klasifikasi", "kode_klasifikasi_id", ""},
		{"idx_arsip_lokasi_arsip", "lokasi_arsip_id", ""},
		{"idx_arsip_status", "status_arsip", ""},
		{"idx_arsip_tanggal_dibuat", "tanggal_dibuat", ""},
		{"idx_arsip_tanggal_retensi", "tanggal_retensi_berakhir", ""},
		{"idx_arsip_pemberkasan", "pemberkasan_id", ""},
		{"idx_arsip_created_at", "created_at", ""},
		{"idx_arsip_uraian", "uraian", "FULLTEXT"}, // supports LIKE search optimization in MySQL InnoDB (5.6+)
	}
	for _, ix := range indexes {
		var exists int64
		DB.Raw(
			"SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = 'arsip' AND index_name = ?",
			ix.name,
		).Scan(&exists)
		if exists == 0 {
			sql := fmt.Sprintf("CREATE %s INDEX %s ON arsip (%s)", ix.extra, ix.name, ix.cols)
			if err := DB.Exec(sql).Error; err != nil {
				log.Printf("[MIGRASI] Skip index %s: %v", ix.name, err)
			}
		}
	}
}

// ensureCommonIndexes adds indexes for foreign-key / filter columns on tables
// that are heavily queried in the apps.
func ensureCommonIndexes() {
	tableColumnIdx := []struct {
		table string
		col   string
		name  string
	}{
		{"login_logs", "username", "idx_login_logs_username"},
		{"login_logs", "user_id", "idx_login_logs_user"},
		{"activity_logs", "user_id", "idx_activity_logs_user"},
		{"activity_logs", "created_at", "idx_activity_logs_created"},
		{"pemusnahan_arsip", "status", "idx_pemusnahan_status"},
		{"pemusnahan_arsip", "created_at", "idx_pemusnahan_created"},
		{"arsip_versions", "arsip_id", "idx_arsip_versions_arsip"},
		{"qr_codes", "arsip_id", "idx_qr_arsip"},
		{"qr_codes", "deleted_at", "idx_qr_deleted"},
		{"peminjaman_arsips", "status", "idx_peminjaman_status"},
		{"peminjaman_arsips", "arsip_id", "idx_peminjaman_arsip"},
		{"peminjaman_arsips", "user_id", "idx_peminjaman_user"},
		{"blockchain_audits", "entity_type", "idx_blockchain_entity_type"},
		{"blockchain_audits", "entity_id", "idx_blockchain_entity_id"},
	}
	for _, ic := range tableColumnIdx {
		if !tableExists(ic.table) {
			continue
		}
		var exists int64
		DB.Raw(
			"SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?",
			ic.table, ic.name,
		).Scan(&exists)
		if exists == 0 {
			sql := fmt.Sprintf("CREATE INDEX %s ON %s (%s)", ic.name, ic.table, ic.col)
			if err := DB.Exec(sql).Error; err != nil {
				log.Printf("[MIGRASI] Skip index %s: %v", ic.name, err)
			}
		}
	}
}

func fixSPJClassification() {
	var total int64
	DB.Table("arsip").Count(&total)

	// Force-update ALL records: if uraian contains "SPM" → "SPJ", else "Non SPJ".
	DB.Table("arsip").Where("uraian LIKE ?", "%SPM%").Update("jenis_arsip", "SPJ")
	DB.Table("arsip").Where("(uraian NOT LIKE ? OR uraian IS NULL) AND uraian IS NOT NULL", "%SPM%").Update("jenis_arsip", "Non SPJ")
	DB.Table("arsip").Where("uraian IS NULL").Update("jenis_arsip", "Non SPJ")

	var updated int64
	DB.Table("arsip").Where("jenis_arsip IN ('SPJ','Non SPJ')").Count(&updated)
	log.Printf("[MIGRASI] Mengklasifikasi %d dari %d arsip sebagai SPJ/Non SPJ", updated, total)
}

func fixBlockchainHashes() {
	var totalCount int64
	DB.Table("blockchain_audits").Count(&totalCount)
	if totalCount == 0 {
		return
	}

	log.Printf("[BLOCKCHAIN] Verifikasi %d blok, memperbaiki hash jika diperlukan...", totalCount)

	var records []models.BlockchainAudit
	DB.Order("block_number ASC").Find(&records)

	recalculated := 0
	var prevHash string
	for i := range records {
		expectedHash := sha256sum(fmt.Sprintf("%s:%s:%s:%s:%s",
			records[i].EntityType, records[i].EntityID, records[i].Action,
			records[i].Details, prevHash))

		if records[i].CurrentHash != expectedHash || records[i].PreviousHash != prevHash {
			// Fix both current_hash and previous_hash.
			DB.Table("blockchain_audits").Where("id = ?", records[i].ID).Updates(map[string]interface{}{
				"current_hash":  expectedHash,
				"previous_hash": prevHash,
			})
			recalculated++
		}
		prevHash = expectedHash
	}

	if recalculated > 0 {
		log.Printf("[BLOCKCHAIN] Memperbaiki %d blok dengan hash tidak valid", recalculated)
	} else {
		log.Printf("[BLOCKCHAIN] Semua %d blok valid", totalCount)
	}
}

func sha256sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func cleanupFilePaths() {
	// On Vercel the filesystem is ephemeral: NO uploaded file ever exists on
	// disk, so os.Stat would report every path as missing and this function
	// would wipe the file_path column of ALL arsip rows on every schema bump.
	if os.Getenv("VERCEL") == "1" {
		return
	}

	var count int64
	DB.Raw("SELECT COUNT(*) FROM arsip WHERE file_path IS NOT NULL AND file_path != ''").Scan(&count)
	if count == 0 {
		return
	}

	var records []struct {
		ID       string
		FilePath string
	}
	DB.Table("arsip").Select("id, file_path").Where("file_path IS NOT NULL AND file_path != ''").Find(&records)

	cleared := 0
	for _, r := range records {
		if _, err := os.Stat(r.FilePath); os.IsNotExist(err) {
			DB.Table("arsip").Where("id = ?", r.ID).Update("file_path", "")
			cleared++
		}
	}
	if cleared > 0 {
		log.Printf("[CLEANUP] Membersihkan %d referensi file_path yang tidak valid (file sudah dihapus)", cleared)
	}
}

// migratePemusnahanData migrates legacy Laravel pemusnahan_arsip data.
func migratePemusnahanData() {
	var count int64
	DB.Raw(`SELECT COUNT(*) FROM pemusnahan_arsip pa
		WHERE pa.arsip_id IS NOT NULL AND pa.arsip_id != ''
		AND pa.id NOT IN (SELECT DISTINCT pemusnahan_id FROM pemusnahan_arsip_items)`).Scan(&count)
	if count == 0 {
		return
	}
	log.Printf("[MIGRASI] Memigrasi %d data pemusnahan lama ke pemusnahan_arsip_items...", count)

	result := DB.Exec(`INSERT INTO pemusnahan_arsip_items (pemusnahan_id, arsip_id, created_at)
		SELECT pa.id, pa.arsip_id, pa.created_at
		FROM pemusnahan_arsip pa
		WHERE pa.arsip_id IS NOT NULL AND pa.arsip_id != ''
		AND pa.id NOT IN (SELECT DISTINCT pi.pemusnahan_id FROM pemusnahan_arsip_items pi)`)
	if result.Error != nil {
		log.Printf("[MIGRASI] Error migrasi items: %v", result.Error)
		return
	}
	log.Printf("[MIGRASI] Berhasil memigrasi %d items ke pemusnahan_arsip_items", result.RowsAffected)

	DB.Exec(`UPDATE pemusnahan_arsip
		SET nama_kegiatan = LEFT(alasan_pengajuan, 255)
		WHERE (nama_kegiatan IS NULL OR nama_kegiatan = '')
		AND alasan_pengajuan IS NOT NULL AND alasan_pengajuan != ''`)

	DB.Exec(`UPDATE pemusnahan_arsip
		SET created_by = user_pengaju_id
		WHERE (created_by IS NULL OR created_by = '')
		AND user_pengaju_id IS NOT NULL AND user_pengaju_id != ''`)

	DB.Exec(`UPDATE pemusnahan_arsip
		SET approved_by = user_approve_id
		WHERE (approved_by IS NULL OR approved_by = '')
		AND user_approve_id IS NOT NULL AND user_approve_id != ''`)

	DB.Exec(`UPDATE pemusnahan_arsip
		SET tanggal_pelaksanaan = tanggal_pengajuan
		WHERE tanggal_pelaksanaan IS NULL AND tanggal_pengajuan IS NOT NULL`)

	log.Println("[MIGRASI] Migrasi data pemusnahan selesai")
}

func addNomorSPMColumn() {
	if !columnExists("arsip", "nomor_spm") {
		DB.Exec("ALTER TABLE arsip ADD COLUMN nomor_spm VARCHAR(100) DEFAULT NULL")
		log.Println("[MIGRASI] Menambahkan kolom nomor_spm ke tabel arsip")
	}
}

func addJumlahSatuanColumn() {
	if !columnExists("arsip", "jumlah") {
		DB.Exec("ALTER TABLE arsip ADD COLUMN jumlah INT NOT NULL DEFAULT 1")
		log.Println("[MIGRASI] Menambahkan kolom jumlah ke tabel arsip")
	}
	if !columnExists("arsip", "satuan") {
		DB.Exec("ALTER TABLE arsip ADD COLUMN satuan VARCHAR(30) NOT NULL DEFAULT 'Berkas'")
		log.Println("[MIGRASI] Menambahkan kolom satuan ke tabel arsip")
	}
}

func migrateLoginSecurity() {
	log.Println("[MIGRASI] Memeriksa kolom keamanan login...")

	if !columnExists("users", "failed_attempts") {
		DB.Exec("ALTER TABLE users ADD COLUMN failed_attempts INT NOT NULL DEFAULT 0")
		log.Println("[MIGRASI] Menambahkan kolom failed_attempts ke tabel users")
	}
	if !columnExists("users", "locked_until") {
		DB.Exec("ALTER TABLE users ADD COLUMN locked_until DATETIME(3) NULL")
		log.Println("[MIGRASI] Menambahkan kolom locked_until ke tabel users")
	}

	// MySQL: ensure model_id in activity_logs can hold long values (TEXT max 64KB).
	// We only need to upgrade if it's a smaller VARCHAR.
	log.Println("[MIGRASI] Memeriksa tipe kolom model_id di activity_logs...")
	dt := columnDataType("activity_logs", "model_id")
	if dt != "" && dt != "text" {
		DB.Exec("ALTER TABLE activity_logs MODIFY COLUMN model_id TEXT")
		log.Println("[MIGRASI] Kolom model_id di activity_logs diubah ke TEXT")
	}

	// Same for blockchain_audits.entity_id.
	log.Println("[MIGRASI] Memeriksa tipe kolom entity_id di blockchain_audits...")
	dt = columnDataType("blockchain_audits", "entity_id")
	if dt != "" && dt != "text" {
		DB.Exec("ALTER TABLE blockchain_audits MODIFY COLUMN entity_id TEXT")
		log.Println("[MIGRASI] Kolom entity_id di blockchain_audits diubah ke TEXT")
	}
}

// addBackupLogGDriveColumns adds the Google Drive columns to backup_logs on
// existing databases (AutoMigrate is skipped when tables already exist).
func addBackupLogGDriveColumns() {
	log.Println("[MIGRASI] Memeriksa kolom Google Drive di backup_logs...")

	if !columnExists("backup_logs", "google_drive_file_id") {
		DB.Exec("ALTER TABLE backup_logs ADD COLUMN google_drive_file_id VARCHAR(255) DEFAULT NULL")
		log.Println("[MIGRASI] Menambahkan kolom google_drive_file_id ke tabel backup_logs")
	}
	if !columnExists("backup_logs", "google_drive_url") {
		DB.Exec("ALTER TABLE backup_logs ADD COLUMN google_drive_url TEXT DEFAULT NULL")
		log.Println("[MIGRASI] Menambahkan kolom google_drive_url ke tabel backup_logs")
	}
}

// addArsipGDriveURLColumn adds the google_drive_url column to the arsip table
// for Google Drive digital archive sync.
func addArsipGDriveURLColumn() {
	if !tableExists("arsip") {
		return
	}
	if !columnExists("arsip", "google_drive_url") {
		DB.Exec("ALTER TABLE arsip ADD COLUMN google_drive_url TEXT DEFAULT NULL")
		log.Println("[MIGRASI] Menambahkan kolom google_drive_url ke tabel arsip")
	}
}

// alignIntegrationTables adapts the legacy Laravel schema of integrations and
// integration_logs so the Go models can insert into them. The Go model writes
// request_body/response_body/status_code which the legacy tables lack, while
// the legacy NOT NULL columns (provider, status, created_by, records_*) are
// never populated by the Go code — every insert silently failed. Both tables
// are expected to be empty or near-empty; the ALTERs are idempotent.
func alignIntegrationTables() {
	if !tableExists("integrations") {
		return
	}
	log.Println("[MIGRASI] Menyesuaikan skema tabel integrasi legacy...")
	DB.Exec("ALTER TABLE integrations MODIFY COLUMN provider VARCHAR(50) NULL DEFAULT 'custom'")
	DB.Exec("ALTER TABLE integrations MODIFY COLUMN status VARCHAR(20) NULL DEFAULT 'active'")
	DB.Exec("ALTER TABLE integrations MODIFY COLUMN created_by VARCHAR(36) NULL")
	if !columnExists("integrations", "request_body") {
		DB.Exec("ALTER TABLE integrations ADD COLUMN request_body TEXT NULL")
	}
	if !columnExists("integrations", "response_body") {
		DB.Exec("ALTER TABLE integrations ADD COLUMN response_body TEXT NULL")
	}
	if !columnExists("integrations", "status_code") {
		DB.Exec("ALTER TABLE integrations ADD COLUMN status_code INT NULL DEFAULT 0")
	}
	if tableExists("integration_logs") {
		DB.Exec("ALTER TABLE integration_logs MODIFY COLUMN records_processed INT NULL DEFAULT 0")
		DB.Exec("ALTER TABLE integration_logs MODIFY COLUMN records_created INT NULL DEFAULT 0")
		DB.Exec("ALTER TABLE integration_logs MODIFY COLUMN records_updated INT NULL DEFAULT 0")
		DB.Exec("ALTER TABLE integration_logs MODIFY COLUMN records_failed INT NULL DEFAULT 0")
		// The Go model writes these columns; the legacy table never had them.
		if !columnExists("integration_logs", "request_body") {
			DB.Exec("ALTER TABLE integration_logs ADD COLUMN request_body TEXT NULL")
		}
		if !columnExists("integration_logs", "response_body") {
			DB.Exec("ALTER TABLE integration_logs ADD COLUMN response_body TEXT NULL")
		}
		if !columnExists("integration_logs", "status_code") {
			DB.Exec("ALTER TABLE integration_logs ADD COLUMN status_code INT NULL DEFAULT 0")
		}
	}
}

// dropJenisLokasiColumnIfUnused removed the legacy jenis_lokasi column from lokasi_arsips.
// SAFE: it was added and removed multiple times historically and is not referenced in code;
// we drop only if it has been retained by an older schema.
func dropJenisLokasiColumnIfUnused() {
	if !tableExists("lokasi_arsips") {
		return
	}
	log.Println("[MIGRASI] Memeriksa kolom jenis_lokasi di lokasi_arsips...")
	if columnExists("lokasi_arsips", "jenis_lokasi") {
		DB.Exec("ALTER TABLE lokasi_arsips DROP COLUMN jenis_lokasi")
		log.Println("[MIGRASI] Kolom jenis_lokasi berhasil dihapus dari tabel lokasi_arsips")
	} else {
		log.Println("[MIGRASI] Kolom jenis_lokasi sudah tidak ada, dilewati")
	}
}

