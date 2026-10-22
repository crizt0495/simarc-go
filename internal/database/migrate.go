package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"

	"arsippro/internal/config"
	"arsippro/internal/models"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
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
const SchemaVersion = "2026.09.22-01"

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

// isAutoIncrementPK reports whether a table's primary key still uses
// AUTO_INCREMENT. Such tables cannot be synchronised between computers because
// independently-inserted numeric ids collide.
func isAutoIncrementPK(table string) bool {
	var n int64
	DB.Raw(`SELECT COUNT(1) FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = ? AND extra LIKE '%auto_increment%'`, table).Scan(&n)
	return n > 0
}

// needsUUIDConversion reports whether a table is still on the legacy layout:
// an AUTO_INCREMENT primary key, or a partially-converted state from an
// interrupted run (numeric id, or a leftover new_id staging column).
func needsUUIDConversion(table string) bool {
	dt := columnDataType(table, "id")
	return dt == "bigint" || isAutoIncrementPK(table) || columnExists(table, "new_id")
}

// dropFksIfExist drops named foreign keys when present.
func dropFksIfExist(table string, names []string) {
	for _, name := range names {
		var n int64
		DB.Raw(`SELECT COUNT(1) FROM information_schema.KEY_COLUMN_USAGE
			WHERE table_schema = DATABASE() AND table_name = ? AND constraint_name = ?`, table, name).Scan(&n)
		if n > 0 {
			DB.Exec("ALTER TABLE `" + table + "` DROP FOREIGN KEY `" + name + "`")
		}
	}
}

// migrateAutoIncToUUID converts the tables that must join multi-computer sync
// from BIGINT AUTO_INCREMENT primary keys to CHAR(36) UUID keys.
//
// Two computers inserting independently would collide on numeric ids, so the
// sync engine ignores AUTO_INCREMENT tables entirely. Existing rows keep their
// identity through deterministic UUIDs (00000000-0000-0000-0000-<12 hex>), and
// referencing columns are rewritten to match.
//
// The conversion is self-gating and per-step idempotent: it returns as soon as
// no targeted table still uses an AUTO_INCREMENT primary key, and unfinished
// steps are retried on the next boot after a partial failure.
func migrateAutoIncToUUID() error {
	targets := []string{"jenis_arsip", "pemusnahan_arsip_items", "blockchain_audits"}
	if !tableExists("jenis_arsip") {
		return nil
	}
	needed := false
	for _, tbl := range targets {
		if needsUUIDConversion(tbl) {
			needed = true
			break
		}
	}
	// Also repair a partially-converted database where jenis_arsip was converted
	// but the referencing arsip column still holds numeric ids.
	if !needed && columnDataType("arsip", "jenis_arsip_id") == "bigint" {
		needed = true
	}
	if !needed {
		return nil
	}

	log.Println("[MIGRASI] Mengonversi tabel BIGINT AUTO_INCREMENT ke kunci UUID: jenis_arsip, pemusnahan_arsip_items, blockchain_audits")

	// Heavy ALTERs on a live database can exceed the application's 20s socket
	// read timeout. When booting against the configured database (production),
	// use a dedicated connection with generous timeouts. Test harnesses that
	// swap DB for a scratch database never match config.App.DBName, so they keep
	// using their own connection — DDL is never aimed at the real database.
	mx := DB
	if config.App.DBName != "" {
		var current string
		DB.Raw("SELECT DATABASE()").Scan(&current)
		if current == config.App.DBName {
			if ddl, err := openDDLConn(); err == nil {
				mx = ddl
				defer func() {
					if sqlDB, e := mx.DB(); e == nil {
						_ = sqlDB.Close()
					}
				}()
			} else {
				log.Printf("[MIGRASI] Gagal membuka koneksi DDL (memakai koneksi biasa): %v", err)
			}
		}
	}

	// The column type of arsip.jenis_arsip_id must change too, which requires
	// dropping its foreign keys first.
	if needsUUIDConversion("jenis_arsip") {
		dropFksIfExist("arsip", []string{"arsip_jenis_arsip_id_foreign", "fk_arsip_jenis", "fk_arsip_jenis_arsip_rel"})
	}

	// Deterministic id for every existing row preserves old identity.
	const uuidFromID = "CONCAT('00000000-0000-0000-0000-', LPAD(HEX(`id`), 12, '0'))"

	convertPK := func(tbl string) error {
		var err error
		// Self-heal a partially converted table (retry after a mid-migration
		// failure): the temporary column is only ever ours, drop it so the
		// conversion restarts cleanly for this table.
		if columnExists(tbl, "new_id") {
			if err = mx.Exec("ALTER TABLE `" + tbl + "` DROP COLUMN `new_id`").Error; err != nil {
				return err
			}
		}
		if err = mx.Exec("ALTER TABLE `" + tbl + "` ADD COLUMN `new_id` CHAR(36) NULL").Error; err != nil {
			return err
		}
		if err = mx.Exec("UPDATE `" + tbl + "` SET `new_id` = " + uuidFromID).Error; err != nil {
			return err
		}
		if err = mx.Exec("ALTER TABLE `" + tbl + "` MODIFY `id` BIGINT NOT NULL").Error; err != nil { // drop auto_increment
			return err
		}
		if err = mx.Exec("ALTER TABLE `" + tbl + "` DROP PRIMARY KEY").Error; err != nil {
			return err
		}
		if err = mx.Exec("ALTER TABLE `" + tbl + "` DROP COLUMN `id`").Error; err != nil {
			return err
		}
		if err = mx.Exec("ALTER TABLE `" + tbl + "` CHANGE COLUMN `new_id` `id` CHAR(36) NOT NULL").Error; err != nil {
			return err
		}
		if err = mx.Exec("ALTER TABLE `" + tbl + "` ADD PRIMARY KEY (`id`)").Error; err != nil {
			return err
		}
		return nil
	}

	// Keep the old→new mapping for jenis_arsip: arsip.jenis_arsip_id still holds
	// the numeric ids and must be rewritten after the primary keys are converted.
	if needsUUIDConversion("jenis_arsip") {
		if err := mx.Exec("CREATE TABLE IF NOT EXISTS `_mig_jenis_uuid` (`old_id` BIGINT NOT NULL PRIMARY KEY, `new_id` CHAR(36) NOT NULL)").Error; err != nil {
			return err
		}
		if err := mx.Exec("DELETE FROM `_mig_jenis_uuid`").Error; err != nil {
			return err
		}
		if err := mx.Exec("INSERT INTO `_mig_jenis_uuid` (`old_id`,`new_id`) SELECT `id`, " + uuidFromID + " FROM `jenis_arsip`").Error; err != nil {
			return err
		}
	}

	for _, tbl := range targets {
		if needsUUIDConversion(tbl) {
			if err := convertPK(tbl); err != nil {
				return fmt.Errorf("konversi UUID %s: %w", tbl, err)
			}
		}
	}

	// Rewrite arsip.jenis_arsip_id (bigint) to the new UUID mapping.
	if columnDataType("arsip", "jenis_arsip_id") == "bigint" {
		// Self-heal a partial state where the temp column was left behind.
		if columnExists("arsip", "new_jenis") {
			if err := mx.Exec("ALTER TABLE `arsip` DROP COLUMN `new_jenis`").Error; err != nil {
				return err
			}
		}
		if err := mx.Exec("ALTER TABLE `arsip` ADD COLUMN `new_jenis` CHAR(36) NULL").Error; err != nil {
			return err
		}
		if err := mx.Exec("UPDATE `arsip` a JOIN `_mig_jenis_uuid` m ON a.`jenis_arsip_id` = m.`old_id` SET a.`new_jenis` = m.`new_id`").Error; err != nil {
			return err
		}
		if err := mx.Exec("ALTER TABLE `arsip` DROP COLUMN `jenis_arsip_id`").Error; err != nil {
			return err
		}
		if err := mx.Exec("ALTER TABLE `arsip` CHANGE COLUMN `new_jenis` `jenis_arsip_id` CHAR(36) NULL").Error; err != nil {
			return err
		}
		if err := mx.Exec("ALTER TABLE `arsip` ADD CONSTRAINT `fk_arsip_jenis` FOREIGN KEY (`jenis_arsip_id`) REFERENCES `jenis_arsip`(`id`)").Error; err != nil {
			return err
		}
	}

	mx.Exec("DROP TABLE IF EXISTS `_mig_jenis_uuid`")
	log.Println("[MIGRASI] Konversi ke UUID selesai untuk 4 tabel + arsip.jenis_arsip_id")
	return nil
}

// openDDLConn opens a dedicated connection to the configured database with
// generous socket timeouts, used for the one-time ALTER-heavy UUID conversion.
func openDDLConn() (*gorm.DB, error) {
	// Normalized so UI/.env values like "required" or "disabled" never reach the
	// driver as an unknown TLS config name. DATABASE_URL is parsed in
	// config.Load() into the DB_* fields, so it is not re-read here.
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local&timeout=10s&readTimeout=300s&writeTimeout=300s&multiStatements=true&interpolateParams=true&tls=%s",
		config.App.DBUser, config.App.DBPass, config.App.DBHost, config.App.DBPort, config.App.DBName,
		config.NormalizeMySQLTLS(config.App.DBTLS),
	)
	return gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
}

// Migrate runs auto-migration for all models. It returns an error instead of
// crashing the process so runtime reconnects can fail gracefully.
//
// Optimized for MySQL: ensures critical performance indexes that AutoMigrate
// alone does not create, and avoids destructive schema changes.
func Migrate() error {
	// Database lokal SQLite (mode offline-first desktop) memakai jalur sendiri:
	// semua tabel dibuat dari model, tanpa perbaikan skema legacy MySQL.
	if IsSQLite() {
		return migrateSQLite()
	}

	// Quiet GORM SQL logging for migration step (we explicitly log meaningful events).
	DB.Logger = logger.Default.LogMode(logger.Error)

	// Post-2.0: any table that joins multi-computer sync must use UUID keys
	// instead of AUTO_INCREMENT (numeric ids collide across computers, so the
	// sync engine ignores such tables). Convert once, idempotently, before the
	// schema-version fast path.
	if err := migrateAutoIncToUUID(); err != nil {
		return err
	}

	// Always-verified invariants (cheap information_schema checks). These run
	// before the schema-version fast path so a database that was already
	// marked "current" can still be repaired.
	//
	// activity_logs.id must stay a VARCHAR(36) UUID column: legacy builds
	// wrongly forced it to BIGINT AUTO_INCREMENT, which breaks activity
	// logging. Repair it whenever an integer type is detected.
	ensureActivityLogIDColumn()
	ensurePermissionColumns()
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
		id VARCHAR(36) NOT NULL PRIMARY KEY,
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
		err := DB.AutoMigrate(allModels()...)
		if err != nil {
			return err
		}
		log.Println("[MIGRASI] Sinkronisasi tabel selesai")
	} else {
		log.Println("[MIGRASI] Tabel sudah ada, melewati AutoMigrate")
	}

	// Repair legacy permission_role tables (composite PK -> surrogate id) AFTER
	// AutoMigrate so the table exists. Idempotent: no-op when id already exists.
	ensurePermissionRoleIdColumn()

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
	addBackupLogContentColumn()
	addArsipGDriveURLColumn()

	// jADWAL drop OK karena tidak ada data referensi lagi pada legacy schema.
	dropJenisLokasiColumnIfUnused()

	// Ensure arsip.id has a DEFAULT so MySQL won't reject inserts that forget
	// to supply the primary key (Error 1364). Works on MySQL 8.0.13+.
	ensureArsipIDDefault()

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

	result := DB.Exec(`INSERT INTO pemusnahan_arsip_items (id, pemusnahan_id, arsip_id, created_at)
		SELECT UUID(), pa.id, pa.arsip_id, pa.created_at
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

// addBackupLogContentColumn adds the file_content LONGBLOB column to
// backup_logs so downloads stay available even when the on-disk file is gone
// (Vercel serverless ephemeral filesystem, deleted files, different cwd).
func addBackupLogContentColumn() {
	log.Println("[MIGRASI] Memeriksa kolom file_content di backup_logs...")
	if !columnExists("backup_logs", "file_content") {
		DB.Exec("ALTER TABLE backup_logs ADD COLUMN file_content LONGBLOB DEFAULT NULL")
		log.Println("[MIGRASI] Menambahkan kolom file_content ke tabel backup_logs")
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

// ensureArsipIDDefault adds DEFAULT (UUID()) to arsip.id so MySQL won't reject
// inserts that forget to supply the primary key (Error 1364 HY000). This is a
// safety net alongside the GORM BeforeCreate hook on the Arsip model. Requires
// MySQL 8.0.13+ (Aiven MySQL satisfies this).
func ensureArsipIDDefault() {
	if !tableExists("arsip") {
		return
	}
	// Check current column default — only ALTER if it's empty.
	var currentDefault *string
	DB.Raw("SELECT column_default FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'arsip' AND column_name = 'id'").Scan(&currentDefault)
	if currentDefault != nil && *currentDefault != "" {
		return
	}
	DB.Exec("ALTER TABLE arsip MODIFY COLUMN id CHAR(36) NOT NULL DEFAULT (UUID())")
	log.Println("[MIGRASI] Menambahkan DEFAULT (UUID()) ke kolom id tabel arsip")
}

// ensureActivityLogIDColumn guarantees activity_logs.id is a VARCHAR(36)
// column that stores the UUID generated by ActivityLog.BeforeCreate.
//
// Legacy builds wrongly forced this column to BIGINT AUTO_INCREMENT, which
// contradicts the string primary key in the model and breaks activity logging
// on fresh (empty) databases. If an integer type is detected, it is repaired
// back to VARCHAR(36).
func ensureActivityLogIDColumn() {
	if !tableExists("activity_logs") {
		return
	}
	var colType string
	DB.Raw("SELECT column_type FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'activity_logs' AND column_name = 'id'").Scan(&colType)
	lower := strings.ToLower(colType)
	if colType == "" || strings.HasPrefix(lower, "varchar") || strings.HasPrefix(lower, "char") {
		return
	}
	// Integer (or other) type → repair to VARCHAR(36) so UUID keys work.
	// MODIFY without AUTO_INCREMENT also drops any stale AUTO_INCREMENT.
	if err := DB.Exec("ALTER TABLE activity_logs MODIFY COLUMN id VARCHAR(36) NOT NULL").Error; err != nil {
		log.Println("[MIGRASI] Gagal memperbaiki kolom id activity_logs:", err)
		return
	}
	log.Println("[MIGRASI] Memperbaiki kolom id tabel activity_logs menjadi VARCHAR(36) UUID")
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

// ensurePermissionColumns adds columns that were introduced to the permissions
// table after the first release (label, description, menu_identifier) but are
// missing on databases created by older builds. Without them the sync engine
// cannot upsert permissions rows into a hub that requires them (NOT NULL
// without default), which surfaced as "Field 'label' doesn't have a default
// value" on nodes created from an older schema.
func ensurePermissionColumns() {
	if !tableExists("permissions") {
		return
	}
	cols := []struct {
		name string
		ddl  string
	}{
		{"label", "VARCHAR(255) NOT NULL DEFAULT ''"},
		{"description", "VARCHAR(255) NULL"},
		{"menu_identifier", "VARCHAR(255) NULL"},
	}
	for _, c := range cols {
		if columnExists("permissions", c.name) {
			continue
		}
		if err := DB.Exec("ALTER TABLE permissions ADD COLUMN " + c.name + " " + c.ddl).Error; err != nil {
			log.Println("[MIGRASI] Gagal menambah kolom", c.name, "ke permissions:", err)
		} else {
			log.Println("[MIGRASI] Menambah kolom", c.name, "ke tabel permissions")
		}
	}
}

// ensurePermissionRoleIdColumn adds the surrogate `id` column to permission_role
// on databases created by older builds, which used a composite (role_id,
// permission_id) primary key. The current schema uses a single UUID `id` so
// rows can be upserted consistently across nodes. Existing rows are backfilled
// with fresh UUIDs. The repair runs on a single connection with foreign key
// checks disabled, because dropping the old primary key can otherwise be
// blocked by foreign key constraints (and a pooled connection would not keep
// the SET FOREIGN_KEY_CHECKS session setting).
func ensurePermissionRoleIdColumn() {
	if !tableExists("permission_role") {
		return
	}
	conn, err := DB.DB()
	if err != nil {
		log.Println("[MIGRASI] Gagal membuka koneksi untuk permission_role:", err)
		return
	}
	// Use a single connection so session-level SETs persist across statements.
	sqlConn, err := conn.Conn(context.Background())
	if err != nil {
		log.Println("[MIGRASI] Gagal membuka koneksi untuk permission_role:", err)
		return
	}
	defer sqlConn.Close()

	run := func(q string, args ...any) error {
		_, err := sqlConn.ExecContext(context.Background(), q, args...)
		return err
	}
	query := func(q string, dest any) {
		row := sqlConn.QueryRowContext(context.Background(), q)
		_ = row.Scan(dest)
	}

	if !columnExists("permission_role", "id") {
		if err := run("ALTER TABLE permission_role ADD COLUMN id VARCHAR(36) NULL"); err != nil {
			log.Println("[MIGRASI] Gagal menambah kolom id ke permission_role:", err)
			return
		}
	}
	run("UPDATE permission_role SET id = UUID() WHERE id IS NULL")

	var pkCol string
	query("SELECT column_name FROM information_schema.key_column_usage WHERE table_schema = DATABASE() AND table_name = 'permission_role' AND constraint_name = 'PRIMARY' LIMIT 1", &pkCol)
	if pkCol == "id" {
		run("ALTER TABLE permission_role MODIFY COLUMN id VARCHAR(36) NOT NULL")
		return
	}

	run("SET FOREIGN_KEY_CHECKS=0")
	if err := run("ALTER TABLE permission_role DROP PRIMARY KEY"); err != nil {
		run("SET FOREIGN_KEY_CHECKS=1")
		log.Println("[MIGRASI] Gagal menghapus PK lama permission_role:", err)
		return
	}
	if err := run("ALTER TABLE permission_role ADD PRIMARY KEY (id)"); err != nil {
		run("SET FOREIGN_KEY_CHECKS=1")
		log.Println("[MIGRASI] Gagal menambah PK id ke permission_role:", err)
		return
	}
	run("SET FOREIGN_KEY_CHECKS=1")
	if err := run("ALTER TABLE permission_role MODIFY COLUMN id VARCHAR(36) NOT NULL"); err != nil {
		log.Println("[MIGRASI] Gagal mengubah id permission_role menjadi NOT NULL:", err)
		return
	}
	run("DELETE FROM sync_keys WHERE table_name = 'permission_role'")
	run("DELETE FROM sync_state WHERE table_name = 'permission_role'")
	// Tombstone dengan bentuk kunci lama (komposit) kini tidak bisa
	// ditafsirkan dan hanya mengkerjakan data yang sah — buang.
	run("DELETE FROM sync_tombstones WHERE table_name = 'permission_role'")
	log.Println("[MIGRASI] Menambah kolom id ke tabel permission_role")
}
