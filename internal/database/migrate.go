package database

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"

	"arsippro/internal/models"
)

// Migrate runs auto-migration for all models. It returns an error instead of
// crashing the process so runtime reconnects can fail gracefully.
func Migrate() error {
	log.Println("[MIGRASI] Memeriksa dan memperbarui skema database...")

	// Create tables manually to avoid GORM FK constraint issues
	DB.Exec(`CREATE TABLE IF NOT EXISTS pemusnahan_arsip_items (
		id BIGSERIAL PRIMARY KEY,
		pemusnahan_id VARCHAR(36) NOT NULL,
		arsip_id VARCHAR(36) NOT NULL,
		created_at TIMESTAMP(3) NULL
	)`)
	DB.Exec(`CREATE TABLE IF NOT EXISTS disposal_schedules (
		id VARCHAR(36) NOT NULL PRIMARY KEY,
		kode_klasifikasi_id VARCHAR(36) DEFAULT NULL,
		arsip_id VARCHAR(36) DEFAULT NULL,
		scheduled_date TIMESTAMP(3) DEFAULT NULL,
		action VARCHAR(50) DEFAULT NULL,
		status VARCHAR(50) DEFAULT 'pending',
		executed_at TIMESTAMP(3) DEFAULT NULL,
		created_by VARCHAR(36) DEFAULT NULL,
		created_at TIMESTAMP(3) DEFAULT NULL,
		updated_at TIMESTAMP(3) DEFAULT NULL,
		deleted_at TIMESTAMP(3) DEFAULT NULL
	)`)
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_disposal_schedules_deleted_at ON disposal_schedules (deleted_at)`)

	// Cek apakah tabel roles sudah ada untuk skip AutoMigrate
	var tableExists int64
	DB.Raw("SELECT COUNT(1) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'roles'").Scan(&tableExists)
	if tableExists == 0 {
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
		log.Println("[MIGRASI] Tabel sudah ada, melewati AutoMigrate (gunakan --rebuild untuk migrasi ulang)")
	}
	log.Println("[MIGRASI] Menjalankan post-migration tasks...")

	// Migrate legacy pemusnahan data
	log.Println("[MIGRASI] Migrasi data pemusnahan...")
	migratePemusnahanData()
	// Clean up file_path references to non-existent files
	log.Println("[MIGRASI] Membersihkan referensi file path...")
	cleanupFilePaths()
	// Auto-classify SPJ/Non SPJ for existing records
	log.Println("[MIGRASI] Mengklasifikasi SPJ/Non SPJ...")
	fixSPJClassification()
	// Verify blockchain data integrity
	log.Println("[MIGRASI] Memverifikasi hash blockchain...")
	fixBlockchainHashes()
	// Add nomor_spm column if missing
	addNomorSPMColumn()
	addJumlahSatuanColumn()

	// Extract SPM numbers from existing records
	migrateSPMFromUraian()
	migrateLoginSecurity()
	dropJenisLokasiColumn()
	addBackupLogGDriveColumns()

	return nil
}

func fixSPJClassification() {
	var total int64
	DB.Table("arsip").Count(&total)

	// Force-update ALL records: if uraian contains "SPM" → "SPJ", else "Non SPJ"
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
			// Fix both current_hash and previous_hash
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
	var count int64
	DB.Raw("SELECT COUNT(*) FROM arsip WHERE file_path IS NOT NULL AND file_path != ''").Scan(&count)
	if count == 0 {
		return
	}

	var records []struct{ ID string; FilePath string }
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

// migratePemusnahanData migrates legacy Laravel pemusnahan_arsip data
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
	var colExists int64
	DB.Raw("SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'arsip' AND column_name = 'nomor_spm'").Scan(&colExists)
	if colExists == 0 {
		DB.Exec("ALTER TABLE arsip ADD COLUMN nomor_spm VARCHAR(100) DEFAULT NULL")
		log.Println("[MIGRASI] Menambahkan kolom nomor_spm ke tabel arsip")
	}
}
func addJumlahSatuanColumn() {
	var colJumlah, colSatuan int64
	DB.Raw("SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'arsip' AND column_name = 'jumlah'").Scan(&colJumlah)
	DB.Raw("SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'arsip' AND column_name = 'satuan'").Scan(&colSatuan)
	if colJumlah == 0 {
		DB.Exec("ALTER TABLE arsip ADD COLUMN jumlah INTEGER NOT NULL DEFAULT 1")
		log.Println("[MIGRASI] Menambahkan kolom jumlah ke tabel arsip")
	}
	if colSatuan == 0 {
		DB.Exec("ALTER TABLE arsip ADD COLUMN satuan VARCHAR(30) NOT NULL DEFAULT 'Berkas'")
		log.Println("[MIGRASI] Menambahkan kolom satuan ke tabel arsip")
	}
}


func migrateLoginSecurity() {
	log.Println("[MIGRASI] Memeriksa kolom keamanan login...")

	var colExists int64
	DB.Raw("SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'users' AND column_name = 'failed_attempts'").Scan(&colExists)
	if colExists == 0 {
		DB.Exec("ALTER TABLE users ADD COLUMN failed_attempts INTEGER DEFAULT 0")
		log.Println("[MIGRASI] Menambahkan kolom failed_attempts ke tabel users")
	}

	DB.Raw("SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'users' AND column_name = 'locked_until'").Scan(&colExists)
	if colExists == 0 {
		DB.Exec("ALTER TABLE users ADD COLUMN locked_until TIMESTAMP(3) NULL")
		log.Println("[MIGRASI] Menambahkan kolom locked_until ke tabel users")
	}

	// Ubah tipe kolom model_id di activity_logs jadi TEXT (PostgreSQL TEXT is unlimited)
	log.Println("[MIGRASI] Memeriksa tipe kolom model_id di activity_logs...")
	var colType string
	DB.Raw("SELECT data_type FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'activity_logs' AND column_name = 'model_id'").Scan(&colType)
	if colType != "" && colType != "text" {
		DB.Exec("ALTER TABLE activity_logs ALTER COLUMN model_id TYPE text")
		log.Println("[MIGRASI] Kolom model_id di activity_logs diubah ke text")
	}

	// Juga entity_id di blockchain_audits
	log.Println("[MIGRASI] Memeriksa tipe kolom entity_id di blockchain_audits...")
	DB.Raw("SELECT data_type FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'blockchain_audits' AND column_name = 'entity_id'").Scan(&colType)
	if colType != "" && colType != "text" {
		DB.Exec("ALTER TABLE blockchain_audits ALTER COLUMN entity_id TYPE text")
		log.Println("[MIGRASI] Kolom entity_id di blockchain_audits diubah ke text")
	}
}

// addBackupLogGDriveColumns adds the Google Drive columns to backup_logs on
// existing databases (AutoMigrate is skipped when tables already exist).
func addBackupLogGDriveColumns() {
	log.Println("[MIGRASI] Memeriksa kolom Google Drive di backup_logs...")

	var colExists int64
	DB.Raw("SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'backup_logs' AND column_name = 'google_drive_file_id'").Scan(&colExists)
	if colExists == 0 {
		DB.Exec("ALTER TABLE backup_logs ADD COLUMN google_drive_file_id VARCHAR(255) DEFAULT NULL")
		log.Println("[MIGRASI] Menambahkan kolom google_drive_file_id ke tabel backup_logs")
	}

	DB.Raw("SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'backup_logs' AND column_name = 'google_drive_url'").Scan(&colExists)
	if colExists == 0 {
		DB.Exec("ALTER TABLE backup_logs ADD COLUMN google_drive_url TEXT DEFAULT NULL")
		log.Println("[MIGRASI] Menambahkan kolom google_drive_url ke tabel backup_logs")
	}
}

func dropJenisLokasiColumn() {
	log.Println("[MIGRASI] Memeriksa kolom jenis_lokasi di lokasi_arsips...")
	var colExists int64
	DB.Raw("SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'lokasi_arsips' AND column_name = 'jenis_lokasi'").Scan(&colExists)
	if colExists > 0 {
		DB.Exec("ALTER TABLE lokasi_arsips DROP COLUMN jenis_lokasi")
		log.Println("[MIGRASI] Kolom jenis_lokasi berhasil dihapus dari tabel lokasi_arsips")
	} else {
		log.Println("[MIGRASI] Kolom jenis_lokasi sudah tidak ada, dilewati")
	}
}
