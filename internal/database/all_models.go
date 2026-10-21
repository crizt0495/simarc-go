package database

import "arsippro/internal/models"

// allModels mengembalikan seluruh model GORM yang perlu dibuatkan tabel.
// Dipakai jalur MySQL (AutoMigrate) maupun SQLite.
func allModels() []interface{} {
	return []interface{}{
		&models.Role{},
		&models.Permission{},
		&models.PermissionRole{},
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
		&models.OcrTempImage{},
		&models.BlockchainAudit{},
		&models.PeminjamanArsip{},
		&models.RetentionSchedule{},
		&models.SavedSearch{},
		&models.BackupLog{},
		&models.Notification{},
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
	}
}
