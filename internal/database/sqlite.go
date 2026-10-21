package database

import (
	"fmt"
	"log"
	"time"

	"arsippro/internal/models"

	glesqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// IsSQLite true saat koneksi aktif adalah database lokal (desktop).
var IsSQLite bool

// sqlitePath menyimpan path file SQLite yang sedang aktif.
var sqlitePath string

// ConnectSQLite membuka database SQLite lokal (mode offline-first) dan
// mendaftarkan outbox hooks. Semua tulisan aplikasi setelah ini otomatis
// tercatat ke sync_outbox untuk dikirim pusat kelak.
func ConnectSQLite(path string) error {
	// Pragma lewat DSN (diglebek oleh glebarez):
	//   busy_timeout — tunggu kunci tulis (WAL: satu penulis pada satu waktu)
	//   journal_mode WAL — baca paralel dengan satu penulis
	//   synchronous NORMAL — balance kecepatan/keamanan
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(0)", path)

	gormDB, err := gorm.Open(glesqlite.Open(dsn), &gorm.Config{
		Logger:                 logger.Default.LogMode(logger.Error),
		PrepareStmt:            false,
		SkipDefaultTransaction: false,
	})
	if err != nil {
		return err
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return err
	}
	// SQLite menyelesaikan tulisan serial; kolam kecil sudah cukup.
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)
	sqlDB.SetConnMaxLifetime(0)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)

	RegisterSyncHooks(gormDB)

	DB = gormDB
	IsSQLite = true
	sqlitePath = path
	log.Printf("[LOKAL] Database SQLite siap di %s", path)
	return nil
}

// SQLitePath mengembalikan path file database lokal aktif.
func SQLitePath() string { return sqlitePath }

// MigrateLocal menjalankan sinkronisasi skema SQLite. Tidak memakai fast-path
// versioning MySQL karena biaya AutoMigrate pada SQLite sangat murah.
func MigrateLocal() error {
	DB.Logger = logger.Default.LogMode(logger.Error)

	if err := DB.AutoMigrate(
		// infrastruktur sync
		&models.SyncOutbox{},
		&models.SyncState{},
		&models.SyncIDMap{},
		// skema aplikasi penuh (kesejajaran perilaku desktop dengan web)
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
		&models.PemusnahanItem{},
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
		&models.DisposalSchedule{},
		&models.EmailNotification{},
		&models.ComplianceScore{},
		&models.LeaderboardStat{},
		&models.NotificationPreference{},
		&models.NotificationTemplate{},
		&models.RetentionNotification{},
		&models.DestructionApproval{},
		&models.ComplianceRisk{},
		&models.AiCategorization{},
		&models.ApiToken{},
		&models.DigitalSignature{},
		&models.Webhook{},
		&models.WebhookLog{},
		&models.Workflow{},
		&models.WorkflowExecution{},
		&models.SearchLog{},
		&models.BeritaAcara{},
		&models.BeritaAcaraItem{},
		&models.Tenant{},
	); err != nil {
		return err
	}

	// Indeks ringan untuk watermark rescan & filter umum (aman idempotent).
	ExecIfPossible := func(sql string) {
		if err := DB.Exec(sql).Error; err != nil {
			log.Printf("[LOKAL] Skip indeks: %v", err)
		}
	}
	if tableExistsExisting("arsip") {
		ExecIfPossible("CREATE INDEX IF NOT EXISTS idx_arsip_updated_at ON arsip (updated_at)")
		ExecIfPossible("CREATE INDEX IF NOT EXISTS idx_arsip_deleted_at ON arsip (deleted_at)")
	}
	if tableExistsExisting("sync_outbox") {
		ExecIfPossible("CREATE INDEX IF NOT EXISTS idx_sync_outbox_changed ON sync_outbox (table_name, row_key)")
	}
	return nil
}

// tableExistsExisting memeriksa keberadaan tabel via sqlite_master (portable).
func tableExistsExisting(name string) bool {
	var n int64
	DB.Raw("SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name = ?", name).Scan(&n)
	return n > 0
}

// SeedLocalBootstrap menyiapkan data awal database lokal:
//   - seed standar (roles, admin, lokasi, jenis arsip) dengan HOOKS DIPAUSE
//     agar data seed TIDAK tercatat di outbox (tidak ter-push ke pusat);
//   - watermark push bootstrap: hanya perubahan nyata pengguna yang di-push.
//
// Fungsi ini harus dipanggil sebelum mesin sync berjalan.
func SeedLocalBootstrap() error {
	PauseSyncHooks()
	defer ResumeSyncHooks()

	var userCount int64
	DB.Model(&models.User{}).Count(&userCount)
	if userCount == 0 {
		seedRoles()
		seedAdmin()
		seedLokasiRakA()
		seedJenisArsip()
		MarkLocalBootstrap(time.Now())
		// Tandai admin seed lokal untuk rekonsiliasi setelah sync pertama.
		var admin models.User
		if err := DB.Where("username = ?", "admin").First(&admin).Error; err == nil {
			SetSyncMeta("local_admin_id", admin.ID)
		}
		if GetSyncMeta("bootstrap_at") == "" {
			SetSyncMeta("bootstrap_at", time.Now().Format(time.RFC3339))
		}
	} else {
		seedLokasiRakA()
		seedJenisArsip()
	}
	return nil
}
