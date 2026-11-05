package models

import (
	"sync"

	"gorm.io/gorm/schema"
)

// allModels adalah daftar tunggal seluruh model yang membutuhkan tabel.
// Dipakai database.AllModels (jalur migrasi MySQL maupun SQLite) dan
// syncdb (membuat tabel yang belum ada di hub).
func allModels() []interface{} {
	return []interface{}{
		&Role{},
		&Permission{},
		&PermissionRole{},
		&UnitKerja{},
		&User{},
		&KodeKlasifikasi{},
		&LokasiArsip{},
		&JenisArsip{},
		&Pemberkasan{},
		&Arsip{},
		&ArsipVersion{},
		&PemusnahanArsip{},
		&PemusnahanItem{},
		&JadwalRetensi{},
		&JadwalRetensiArsip{},
		&LoginLog{},
		&ActivityLog{},
		&AuditLog{},
		&OcrLog{},
		&OcrTempImage{},
		&BlockchainAudit{},
		&PeminjamanArsip{},
		&RetentionSchedule{},
		&SavedSearch{},
		&BackupLog{},
		&Notification{},
		&DisposalSchedule{},
		&EmailNotification{},
		&ComplianceScore{},
		&LeaderboardStat{},
		&NotificationPreference{},
		&NotificationTemplate{},
		&RetentionNotification{},
		&Tenant{},
		&DestructionApproval{},
		&ComplianceRisk{},
		&AiCategorization{},
		&ApiToken{},
		&DigitalSignature{},
		&Webhook{},
		&WebhookLog{},
		&Workflow{},
		&WorkflowExecution{},
		&SearchLog{},
	}
}

// AllModels mengembalikan seluruh model GORM yang perlu dibuatkan tabel.
func AllModels() []interface{} { return allModels() }

var (
	tableIndexOnce sync.Once
	tableIndex     map[string]interface{}
)

// TableNameOf mengembalikan nama tabel yang dipakai GORM untuk sebuah model.
func TableNameOf(model interface{}) string {
	s, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		return ""
	}
	return s.Table
}

// ForTable mengembalikan model yang memiliki tabel dengan nama tersebut, atau
// nil bila tabelnya bukan tabel aplikasi (mis. tabel hasil migrasi manual).
func ForTable(name string) interface{} {
	tableIndexOnce.Do(func() {
		tableIndex = make(map[string]interface{}, len(allModels()))
		for _, m := range allModels() {
			if t := TableNameOf(m); t != "" {
				tableIndex[t] = m
			}
		}
	})
	return tableIndex[name]
}
