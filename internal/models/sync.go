package models

import (
	"time"

	"gorm.io/gorm"
)

// ── OFFLINE-FIRST SYNC INFRASTRUCTURE ─────────────────────────────────────────
//
// Tabel-tabel di file ini mendukung mode offline-first (DB_MODE=local):
//   sync_outbox  — jejak perubahan lokal yang belum terkirim ke pusat
//   sync_state   — checkpoint watermark push/pull per tabel
//   sync_id_maps — pemetaan PK auto-increment lokal ↔ pusat (tabel jenis_arsip)

// SyncOutbox mencatat satu perubahan lokal (upsert/delete) ke sebuah baris.
// Ditulis otomatis oleh callback GORM di internal/database/hooks.go pada
// koneksi SQLite (dalam transaksi yang sama dengan perubahan barisnya).
type SyncOutbox struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	Table     string         `gorm:"column:table_name;size:100;not null" json:"table_name"`
	RowKey    string         `gorm:"column:row_key;size:64;not null" json:"row_key"` // UUID baris
	Action    string         `gorm:"size:20;not null" json:"action"`                 // create | update | delete
	ClientID  string         `gorm:"column:client_id;size:36" json:"client_id"`
	ChangedAt time.Time      `gorm:"column:changed_at;not null" json:"changed_at"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (SyncOutbox) TableName() string { return "sync_outbox" }

// SyncState menyimpan watermark per tabel: kapan terakhir baris tabel itu
// di-push ke pusat dan di-pull dari pusat. Satu baris per tabel. Kolom Meta
// dipakai sebagai metadata kecil (bootstrap, dsb).
type SyncState struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Table      string    `gorm:"column:table_name;size:100;uniqueIndex" json:"table_name"`
	LastPushAt time.Time `json:"last_push_at"`
	LastPullAt time.Time `json:"last_pull_at"`
	Meta       string    `gorm:"type:text" json:"meta"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (SyncState) TableName() string { return "sync_state" }

// SyncIDMap memetakan PK auto-increment antar node (lokal ↔ pusat) untuk
// tabel jenis_arsip yang PK-nya uint. FK arsip.jenis_arsip_id diterjemahkan
// lewat map ini saat merge ke dua arah.
type SyncIDMap struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Table     string    `gorm:"column:table_name;size:100;not null" json:"table_name"`
	LocalID   uint      `gorm:"column:local_id;not null" json:"local_id"`
	RemoteID  uint      `gorm:"column:remote_id;not null" json:"remote_id"`
	UUID      string    `gorm:"size:36" json:"uuid"`
	CreatedAt time.Time `json:"created_at"`
}

func (SyncIDMap) TableName() string { return "sync_id_maps" }

// BeforeCreate ensures id terisi untuk tabel outbox (dipanggil saat insert manual).
func (o *SyncOutbox) BeforeCreate(tx *gorm.DB) error { return nil }

// helper konstanta aksi outbox
const (
	OutboxActionCreate string = "create"
	OutboxActionUpdate string = "update"
	OutboxActionDelete string = "delete"
)
