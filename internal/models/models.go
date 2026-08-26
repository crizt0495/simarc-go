package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"time"

	"gorm.io/gorm"
)

// JSONStringArray is a helper for JSON-encoded string arrays
type JSONStringArray []string

func (j JSONStringArray) Value() (driver.Value, error) {
	return json.Marshal(j)
}

func (j *JSONStringArray) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, j)
}

// ── ROLE ──────────────────────────────────────────────────────────────────────

type Role struct {
	ID          string         `gorm:"type:char(36);primaryKey" json:"id"`
	Name        string         `gorm:"size:100;not null" json:"name"`
	NamaRole    string         `gorm:"size:100" json:"nama_role"`
	Keterangan  string         `gorm:"type:text" json:"keterangan"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at"`
	Users       []User         `gorm:"foreignKey:RoleID" json:"users,omitempty"`
	Permissions []Permission   `gorm:"many2many:permission_role;" json:"permissions,omitempty"`
}

// ── PERMISSION ────────────────────────────────────────────────────────────────

type Permission struct {
	ID        string         `gorm:"type:char(36);primaryKey" json:"id"`
	Name      string         `gorm:"size:100;not null;unique" json:"name"`
	Module    string         `gorm:"size:100" json:"module"`
	IsActive  bool           `gorm:"default:true" json:"is_active"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

// ── UNIT KERJA ────────────────────────────────────────────────────────────────

type UnitKerja struct {
	ID          string         `gorm:"column:id;type:char(36);primaryKey" json:"id"`
	NamaUnit    string         `gorm:"column:nama_unit;size:255;not null" json:"nama_unit"`
	TotalPoints int            `gorm:"column:total_points;default:0" json:"total_points"`
	Badge       string         `gorm:"column:badge;size:100;default:'Bronze Archival'" json:"badge"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (UnitKerja) TableName() string { return "unit_kerja" }

// ── USER ──────────────────────────────────────────────────────────────────────

type User struct {
	ID             string         `gorm:"type:char(36);primaryKey" json:"id"`
	Username       string         `gorm:"size:100;not null;unique" json:"username"`
	Name           string         `gorm:"size:255;not null" json:"name"`
	Password       string         `gorm:"size:255;not null" json:"-"`
	RoleID         string         `gorm:"type:char(36)" json:"role_id"`
	UnitKerjaID    *string        `gorm:"type:char(36)" json:"unit_kerja_id"`
	IsActive       bool           `gorm:"default:true" json:"is_active"`
	FailedAttempts int            `gorm:"default:0" json:"-"`
	LockedUntil    *time.Time     `json:"-"`
	LastLoginAt    *time.Time     `json:"last_login_at"`
	RememberToken  string         `gorm:"size:100" json:"-"`
	ThemeSettings  string         `gorm:"type:text" json:"theme_settings"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"deleted_at"`
	Role           *Role          `gorm:"foreignKey:RoleID" json:"role,omitempty"`
	UnitKerja      *UnitKerja     `gorm:"foreignKey:UnitKerjaID" json:"unit_kerja,omitempty"`
}

func (u *User) IsAdmin() bool {
	return u.Role != nil && u.Role.Name == "Admin"
}

// ── KODE KLASIFIKASI ──────────────────────────────────────────────────────────

type KodeKlasifikasi struct {
	ID                  string            `gorm:"type:char(36);primaryKey" json:"id"`
	KodeKlasifikasi     string            `gorm:"column:kode_klasifikasi;size:50;not null" json:"kode_klasifikasi"`
	NamaKlasifikasi     string            `gorm:"column:nama_klasifikasi;size:255;not null" json:"nama_klasifikasi"`
	RetensiAktif        int               `gorm:"default:0" json:"retensi_aktif"`
	RetensiInaktif      int               `gorm:"default:0" json:"retensi_inaktif"`
	PenyusutanArsip     string            `gorm:"size:50" json:"penyusutan_arsip"`
	HakAkses            JSONStringArray   `gorm:"type:longtext" json:"hak_akses"`
	KlasifikasiKeamanan string            `gorm:"size:50" json:"klasifikasi_keamanan"`
	DasarPertimbangan   string            `gorm:"type:text" json:"dasar_pertimbangan"`
	UnitPengolah        string            `gorm:"size:255" json:"unit_pengolah"`
	ParentID            *string           `gorm:"type:char(36)" json:"parent_id"`
	IsActive            bool              `gorm:"default:true" json:"is_active"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
	DeletedAt           gorm.DeletedAt    `gorm:"index" json:"deleted_at"`
	Parent              *KodeKlasifikasi  `gorm:"foreignKey:ParentID" json:"parent,omitempty"`
	Children            []KodeKlasifikasi `gorm:"foreignKey:ParentID" json:"children,omitempty"`
}

func (KodeKlasifikasi) TableName() string { return "kode_klasifikasi" }

// ── LOKASI ARSIP ──────────────────────────────────────────────────────────────

type LokasiArsip struct {
	ID          string         `gorm:"type:char(36);primaryKey" json:"id"`
	NamaLokasi  string         `gorm:"column:nama_lokasi;size:255;not null" json:"nama_lokasi"`
	Deskripsi   string         `gorm:"type:text" json:"deskripsi"`
	Kapasitas   *string        `gorm:"size:100" json:"kapasitas"`
	IsActive    bool           `gorm:"default:true" json:"is_active"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (LokasiArsip) TableName() string { return "lokasi_arsips" }

// ── JENIS ARSIP ───────────────────────────────────────────────────────────────

type JenisArsip struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	KodeJenis  string    `gorm:"size:50" json:"kode_jenis"`
	NamaJenis  string    `gorm:"size:255;not null" json:"nama_jenis"`
	Keterangan string    `gorm:"type:text" json:"keterangan"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (JenisArsip) TableName() string { return "jenis_arsip" }

// ── PEMBERKASAN ───────────────────────────────────────────────────────────────

type Pemberkasan struct {
	ID                string           `gorm:"type:char(36);primaryKey" json:"id"`
	KodeBerkas        string           `gorm:"size:100" json:"kode_berkas"`
	NamaPemberkasan   string           `gorm:"size:255" json:"nama_pemberkasan"`
	Tahun             int              `json:"tahun"`
	TanggalMulai      *time.Time       `json:"tanggal_mulai"`
	TanggalTutup      *time.Time       `json:"tanggal_tutup"`
	StatusBerkas      string           `gorm:"size:50;default:aktif" json:"status_berkas"`
	UnitKerjaID       *string          `gorm:"type:char(36)" json:"unit_kerja_id"`
	KodeKlasifikasiID *string          `gorm:"type:char(36)" json:"kode_klasifikasi_id"`
	CreatedBy         *string          `gorm:"type:char(36)" json:"created_by"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
	DeletedAt         gorm.DeletedAt   `gorm:"index" json:"deleted_at"`
	Creator           *User            `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
	UnitKerja         *UnitKerja       `gorm:"foreignKey:UnitKerjaID" json:"unit_kerja,omitempty"`
	KodeKlasifikasi   *KodeKlasifikasi `gorm:"foreignKey:KodeKlasifikasiID" json:"kode_klasifikasi,omitempty"`
	Arsip             []Arsip          `gorm:"foreignKey:PemberkasanID" json:"arsip,omitempty"`
}

func (Pemberkasan) TableName() string { return "pemberkasan" }

// ── ARSIP ─────────────────────────────────────────────────────────────────────

type Arsip struct {
	ID                  string           `gorm:"type:char(36);primaryKey" json:"id"`
	NomorArsip          string           `gorm:"size:100" json:"nomor_arsip"`
	NamaArsip           string           `gorm:"size:500;not null" json:"nama_arsip"`
	TingkatPerkembangan string           `gorm:"size:50" json:"tingkat_perkembangan"`
	JenisArsipID        *uint            `json:"jenis_arsip_id"`
	Uraian              string           `gorm:"type:text" json:"uraian"`
	KodeKlasifikasiID   string           `gorm:"type:char(36);not null" json:"kode_klasifikasi_id"`
	UnitKerjaID         string           `gorm:"type:char(36);not null" json:"unit_kerja_id"`
	PemberkasanID       *string          `gorm:"type:char(36)" json:"pemberkasan_id"`
	LokasiArsipID       *string          `gorm:"type:char(36)" json:"lokasi_arsip_id"`
	TanggalDibuat       *time.Time       `json:"tanggal_dibuat"`
	TanggalRetensiAkhir *time.Time       `gorm:"column:tanggal_retensi_berakhir" json:"tanggal_retensi_berakhir"`
	NilaiAnggaran       *float64         `json:"nilai_anggaran"`
	Nominal             *float64         `json:"nominal"`
	JenisArsip          string           `gorm:"column:jenis_arsip;size:50" json:"jenis_arsip"`
	NomorSPM            string           `gorm:"column:nomor_spm;size:100" json:"nomor_spm"`
	StatusArsip         string           `gorm:"size:50;default:aktif" json:"status_arsip"`
	Jumlah              int              `gorm:"default:1;not null" json:"jumlah"`
	Satuan              string           `gorm:"size:30;not null;default:Berkas" json:"satuan"`
	StatusReview        bool             `gorm:"default:false" json:"status_review"`
	FilePath            string           `gorm:"type:text" json:"file_path"`
	FileName            string           `gorm:"column:file_name;size:255" json:"file_name"`
	OcrText             string           `gorm:"column:ocr_text;type:text" json:"ocr_text"`
	OcrFullText         string           `gorm:"column:ocr_full_text;type:longtext" json:"ocr_full_text"`
	Tags                string           `gorm:"type:text" json:"tags"`
	OcrProcessed        bool             `gorm:"default:false" json:"ocr_processed"`
	OcrProcessedAt      *time.Time       `json:"ocr_processed_at"`
	GoogleDriveFileID   string           `gorm:"size:255" json:"google_drive_file_id"`
	GoogleDriveURL      string           `gorm:"type:text" json:"google_drive_url"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
	DeletedAt           gorm.DeletedAt   `gorm:"index" json:"deleted_at"`
	KodeKlasifikasi     *KodeKlasifikasi `gorm:"foreignKey:KodeKlasifikasiID" json:"kategori,omitempty"`
	UnitKerja           *UnitKerja       `gorm:"foreignKey:UnitKerjaID" json:"unit_kerja,omitempty"`
	Pemberkasan         *Pemberkasan     `gorm:"foreignKey:PemberkasanID" json:"pemberkasan,omitempty"`
	LokasiArsip         *LokasiArsip     `gorm:"foreignKey:LokasiArsipID" json:"lokasi_arsip,omitempty"`
	JenisArsipRel       *JenisArsip      `gorm:"foreignKey:JenisArsipID" json:"jenis_arsip_rel,omitempty"`
}

func (Arsip) TableName() string { return "arsip" }

func (a Arsip) TanggalRetensiBerakhir() string {
	if a.TanggalRetensiAkhir == nil {
		return ""
	}
	return a.TanggalRetensiAkhir.Format("2006-01-02")
}

func (a Arsip) RetensiExpired() bool {
	if a.TanggalRetensiAkhir == nil {
		return false
	}
	return a.TanggalRetensiAkhir.Before(time.Now())
}

// TanggalRetensiBerakhirExpired is alias for template compatibility
func (a Arsip) TanggalRetensiBerakhirExpired() bool {
	return a.RetensiExpired()
}

// BeforeCreate ensures the Arsip primary key is populated before insert.
// The id column is CHAR(36) PRIMARY KEY with no DEFAULT and no AUTO_INCREMENT,
// so MySQL rejects inserts that omit it with Error 1364 (HY000)
// "Field 'id' doesn't have a default value". Mirrors the pattern already
// used by LoginLog, AuditLog, QrScanLog, OcrLog, BackupLog, IntegrationLog,
// and EmailNotification.
func (a *Arsip) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}

// HitungRetensiBerakhir calculates the retention end date from tanggalDibuat + (RetensiAktif + RetensiInaktif) years
// based on the associated KodeKlasifikasi.
func HitungRetensiBerakhir(tanggalDibuat time.Time, kk *KodeKlasifikasi) *time.Time {
	if kk == nil {
		return nil
	}
	totalRetensi := kk.RetensiAktif + kk.RetensiInaktif
	if totalRetensi <= 0 {
		return nil
	}
	end := tanggalDibuat.AddDate(totalRetensi, 0, 0)
	return &end
}

// ── ARSIP VERSION ─────────────────────────────────────────────────────────────

type ArsipVersion struct {
	ID         string         `gorm:"type:char(36);primaryKey" json:"id"`
	ArsipID    string         `gorm:"type:char(36);not null" json:"arsip_id"`
	Version    int            `gorm:"column:nomor_versi" json:"version"`
	FilePath   string         `gorm:"type:text" json:"file_path"`
	ChangedBy  string         `gorm:"column:diubah_oleh;type:char(36)" json:"changed_by"`
	ChangeNote string         `gorm:"column:catatan_perubahan;type:text" json:"change_note"`
	CreatedAt  time.Time      `json:"created_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (ArsipVersion) TableName() string { return "arsip_versions" }

// ── PEMUSNAHAN ARSIP ──────────────────────────────────────────────────────────

type PemusnahanArsip struct {
	ID                 string         `gorm:"type:char(36);primaryKey" json:"id"`
	ArsipID            *string        `gorm:"type:char(36)" json:"arsip_id"`
	NamaKegiatan       string         `gorm:"size:255" json:"nama_kegiatan"`
	TanggalPelaksanaan *time.Time     `json:"tanggal_pelaksanaan"`
	AlasanPengajuan    string         `gorm:"type:text" json:"alasan_pengajuan"`
	KeteranganApprove  string         `gorm:"type:text" json:"keterangan_approve"`
	Keterangan         string         `gorm:"type:text" json:"keterangan"`
	UserPengajuID      string         `gorm:"type:char(36)" json:"user_pengaju_id"`
	UserApproveID      *string        `gorm:"type:char(36)" json:"user_approve_id"`
	CreatedBy          *string        `gorm:"type:char(36)" json:"created_by"`
	ApprovedBy         *string        `gorm:"type:char(36)" json:"approved_by"`
	Status             string         `gorm:"size:50;default:diajukan" json:"status"`
	TanggalPengajuan   *time.Time     `json:"tanggal_pengajuan"`
	TanggalApprove     *time.Time     `json:"tanggal_approve"`
	IsAuto             bool           `gorm:"default:false" json:"is_auto"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"deleted_at"`
	Arsip              *Arsip         `gorm:"foreignKey:ArsipID" json:"arsip,omitempty"`
	Creator            *User          `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
	Approver           *User          `gorm:"foreignKey:ApprovedBy" json:"approver,omitempty"`
	UserPengaju        *User          `gorm:"foreignKey:UserPengajuID" json:"user_pengaju,omitempty"`
	UserApprove        *User          `gorm:"foreignKey:UserApproveID" json:"user_approve,omitempty"`
	Items              []PemusnahanItem `gorm:"foreignKey:PemusnahanID" json:"items,omitempty"`
}

func (PemusnahanArsip) TableName() string { return "pemusnahan_arsip" }

type PemusnahanItem struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	PemusnahanID string    `gorm:"type:char(36);not null" json:"pemusnahan_id"`
	ArsipID      string    `gorm:"type:char(36);not null" json:"arsip_id"`
	CreatedAt    time.Time `json:"created_at"`
	Arsip        *Arsip    `gorm:"foreignKey:ArsipID" json:"arsip,omitempty"`
}

func (PemusnahanItem) TableName() string { return "pemusnahan_arsip_items" }

// ── JADWAL RETENSI ────────────────────────────────────────────────────────────

type JadwalRetensi struct {
	ID                 string           `gorm:"type:char(36);primaryKey" json:"id"`
	NamaJadwal         string           `gorm:"size:255;not null" json:"nama_jadwal"`
	Deskripsi          string           `gorm:"type:text" json:"deskripsi"`
	TanggalJadwal      *time.Time       `json:"tanggal_jadwal"`
	JenisJadwal        string           `gorm:"size:50;default:draft" json:"jenis_jadwal"`
	Status             string           `gorm:"size:50;default:draft" json:"status"`
	CreatedBy          *string          `gorm:"type:char(36)" json:"created_by"`
	KodeKlasifikasiID  *string          `gorm:"type:char(36)" json:"kode_klasifikasi_id"`
	UnitKerjaID        *string          `gorm:"type:char(36)" json:"unit_kerja_id"`
	RetensiAktif       int              `json:"retensi_aktif"`
	RetensiInaktif     int              `json:"retensi_inaktif"`
	TotalArsip         int              `gorm:"default:0" json:"total_arsip"`
	ArsipDiproses      int              `gorm:"default:0" json:"arsip_diproses"`
	Nasib              string           `gorm:"size:50" json:"nasib"`
	Catatan            string           `gorm:"type:text" json:"catatan"`
	Keterangan         string           `gorm:"type:text" json:"keterangan"`
	TanggalPelaksanaan *time.Time       `json:"tanggal_pelaksanaan"`
	TanggalSelesai     *time.Time       `json:"tanggal_selesai"`
	CreatedAt          time.Time        `json:"created_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
	DeletedAt          gorm.DeletedAt   `gorm:"index" json:"deleted_at"`
	Creator            *User            `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
	KodeKlasifikasi    *KodeKlasifikasi `gorm:"foreignKey:KodeKlasifikasiID" json:"kode_klasifikasi,omitempty"`
	UnitKerja          *UnitKerja       `gorm:"foreignKey:UnitKerjaID" json:"unit_kerja,omitempty"`
}

func (JadwalRetensi) TableName() string { return "jadwal_retensi" }

type JadwalRetensiArsip struct {
	ID              string     `gorm:"type:char(36);primaryKey" json:"id"`
	JadwalRetensiID string     `gorm:"type:char(36);not null" json:"jadwal_retensi_id"`
	ArsipID         string     `gorm:"type:char(36);not null" json:"arsip_id"`
	Status          string     `gorm:"size:50;default:pending" json:"status"`
	Catatan         string     `gorm:"type:text" json:"catatan"`
	ProcessedAt     *time.Time `json:"processed_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	Arsip           *Arsip     `gorm:"foreignKey:ArsipID" json:"arsip,omitempty"`
}

func (JadwalRetensiArsip) TableName() string { return "jadwal_retensi_arsip" }

// ── LOGIN LOG ─────────────────────────────────────────────────────────────────

type LoginLog struct {
	ID         string     `gorm:"type:char(36);primaryKey" json:"id"`
	UserID     *string    `gorm:"type:char(36)" json:"user_id"`
	Username   string     `gorm:"size:100" json:"username"`
	IPAddress  string     `gorm:"size:50" json:"ip_address"`
	UserAgent  string     `gorm:"type:text" json:"user_agent"`
	Status     string     `gorm:"size:20" json:"status"`
	LoginTime  time.Time  `json:"login_time"`
	LogoutTime *time.Time `json:"logout_time"`
}

func (LoginLog) TableName() string { return "login_logs" }

func (l *LoginLog) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	return nil
}

// ── ACTIVITY LOG ──────────────────────────────────────────────────────────────

type ActivityLog struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      *string   `gorm:"type:char(36)" json:"user_id"`
	Action      string    `gorm:"size:255" json:"action"`
	Description string    `gorm:"type:text" json:"description"`
	ModelType   string    `gorm:"size:100" json:"model_type"`
	ModelID     string    `gorm:"type:text" json:"model_id"`
	CreatedAt   time.Time `json:"created_at"`
	User        *User     `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

func (ActivityLog) TableName() string { return "activity_logs" }

func (a *ActivityLog) BeforeCreate(tx *gorm.DB) error {
	if a.ID == 0 {
		// let DB auto-increment; this hook ensures it's called
	}
	return nil
}

// ── AUDIT LOG ────────────────────────────────────────────────────────────────

type AuditLog struct {
	ID          string    `gorm:"type:char(36);primaryKey" json:"id"`
	UserID      *string   `gorm:"type:char(36)" json:"user_id"`
	Action      string    `gorm:"size:255" json:"action"`
	EntityTable string    `gorm:"column:table_name;size:100" json:"table_name"`
	RecordID    string    `gorm:"size:100" json:"record_id"`
	OldValues   string    `gorm:"type:json" json:"old_values"`
	NewValues   string    `gorm:"type:json" json:"new_values"`
	IPAddress   string    `gorm:"size:50" json:"ip_address"`
	CreatedAt   time.Time `json:"created_at"`
}

func (AuditLog) TableName() string { return "audit_logs" }

func (l *AuditLog) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	return nil
}

// ── QR CODE ───────────────────────────────────────────────────────────────────

type QrCode struct {
	ID            string         `gorm:"type:char(36);primaryKey" json:"id"`
	ArsipID       *string        `gorm:"type:char(36)" json:"arsip_id"`
	LokasiID      *string        `gorm:"type:char(36)" json:"lokasi_id"`
	QrType        string         `gorm:"size:50;default:arsip" json:"qr_type"`
	BoxNumber     *string        `gorm:"size:100" json:"box_number"`
	ShelfLocation *string        `gorm:"size:255" json:"shelf_location"`
	RoomLocation  *string        `gorm:"size:255" json:"room_location"`
	LocationData  *string        `gorm:"type:json" json:"location_data"`
	QrCodePath    string         `gorm:"type:text" json:"qr_code_path"`
	QrData        string         `gorm:"type:text" json:"qr_data"`
	IsActive      bool           `gorm:"default:true" json:"is_active"`
	LastScannedAt *time.Time     `json:"last_scanned_at"`
	LastScannedBy *string        `gorm:"type:char(36)" json:"last_scanned_by"`
	ScanCount     int            `gorm:"default:0" json:"scan_count"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"deleted_at"`
	Arsip         *Arsip         `gorm:"foreignKey:ArsipID" json:"arsip,omitempty"`
}

func (QrCode) TableName() string { return "qr_codes" }

type QrScanLog struct {
	ID        string    `gorm:"type:char(36);primaryKey" json:"id"`
	QrCodeID  string    `gorm:"type:char(36);not null" json:"qr_code_id"`
	UserID    *string   `gorm:"type:char(36)" json:"user_id"`
	Action    string    `gorm:"size:50" json:"action"`
	Notes     *string   `gorm:"type:text" json:"notes"`
	IPAddress string    `gorm:"size:50" json:"ip_address"`
	UserAgent string    `gorm:"type:text" json:"user_agent"`
	Metadata  *string   `gorm:"type:json" json:"metadata"`
	ScannedAt time.Time `json:"scanned_at"`
	CreatedAt time.Time `json:"created_at"`
}

func (QrScanLog) TableName() string { return "qr_scan_logs" }

func (l *QrScanLog) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	return nil
}

// ── OCR ───────────────────────────────────────────────────────────────────────

type OcrLog struct {
	ID            string     `gorm:"type:char(36);primaryKey" json:"id"`
	ArsipID       *string    `gorm:"type:char(36)" json:"arsip_id"`
	FileName      string     `gorm:"column:filename;size:255" json:"file_name"`
	Status        string     `gorm:"size:50" json:"status"`
	ExtractedText string     `gorm:"type:longtext" json:"extracted_text"`
	ProcessedAt   *time.Time `json:"processed_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

func (OcrLog) TableName() string { return "ocr_logs" }

func (l *OcrLog) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	return nil
}

type OcrTempImage struct {
	ID           string    `gorm:"type:char(36);primaryKey" json:"id"`
	UserID       string    `gorm:"type:char(36)" json:"user_id"`
	FilePath     string    `gorm:"type:text" json:"file_path"`
	OriginalName string    `gorm:"size:255" json:"original_name"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (OcrTempImage) TableName() string { return "ocr_temp_images" }

// ── BLOCKCHAIN AUDIT ──────────────────────────────────────────────────────────

type BlockchainAudit struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UUID         string    `gorm:"column:uuid;size:36" json:"uuid"`
	PreviousHash string    `gorm:"column:previous_hash;size:64" json:"previous_hash"`
	CurrentHash  string    `gorm:"column:current_hash;size:64" json:"current_hash"`
	BlockNumber  uint64    `gorm:"column:block_number" json:"block_number"`
	Timestamp    string    `gorm:"column:timestamp;size:50" json:"timestamp"`
	UserID       *string   `gorm:"column:user_id;type:char(36)" json:"user_id"`
	Action       string    `gorm:"column:action;size:100" json:"action"`
	EntityType   string    `gorm:"column:entity_type;size:100" json:"entity_type"`
	EntityID     string    `gorm:"column:entity_id;type:text" json:"entity_id"`
	Details      string    `gorm:"column:details;type:text" json:"details"`
	IPAddress    string    `gorm:"column:ip_address;size:45" json:"ip_address"`
	UserAgent    string    `gorm:"column:user_agent;type:text" json:"user_agent"`
	IsValid      bool      `gorm:"column:is_valid;default:true" json:"is_valid"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (BlockchainAudit) TableName() string { return "blockchain_audits" }

// ── PEMINJAMAN ARSIP ──────────────────────────────────────────────────────────

type PeminjamanArsip struct {
	ID                    string         `gorm:"type:char(36);primaryKey" json:"id"`
	ArsipID               string         `gorm:"type:char(36);not null" json:"arsip_id"`
	UserID                string         `gorm:"type:char(36);not null" json:"user_id"`
	TanggalPinjam         *time.Time     `json:"tanggal_pinjam"`
	TanggalKembaliRencana *time.Time     `gorm:"column:tanggal_kembali_rencana" json:"tanggal_kembali_rencana"`
	TanggalKembaliAktual  *time.Time     `gorm:"column:tanggal_kembali_aktual" json:"tanggal_kembali_aktual"`
	Keperluan             string         `gorm:"type:text" json:"keperluan"`
	Status                string         `gorm:"size:50;default:pending" json:"status"`
	ApprovedBy            *string        `gorm:"type:char(36)" json:"approved_by"`
	Catatan               string         `gorm:"type:text" json:"catatan"`
	BlockchainHash        string         `gorm:"column:blockchain_hash;size:64" json:"blockchain_hash"`
	KeteranganAdmin       string         `gorm:"column:keterangan_admin;type:text" json:"keterangan_admin"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	DeletedAt             gorm.DeletedAt `gorm:"index" json:"deleted_at"`
	Arsip                 *Arsip         `gorm:"foreignKey:ArsipID" json:"arsip,omitempty"`
	User                  *User          `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Approver              *User          `gorm:"foreignKey:ApprovedBy" json:"approver,omitempty"`
}

func (PeminjamanArsip) TableName() string { return "peminjaman_arsips" }

func (p PeminjamanArsip) TanggalDueDate() string {
	if p.TanggalKembaliRencana == nil {
		return ""
	}
	return p.TanggalKembaliRencana.Format("02-01-2006")
}

// ── RETENTION SCHEDULE ────────────────────────────────────────────────────────

type RetentionSchedule struct {
	ID               string    `gorm:"type:char(36);primaryKey" json:"id"`
	ArsipID          string    `gorm:"type:char(36);not null" json:"arsip_id"`
	ScheduledDate    time.Time `json:"scheduled_date"`
	Action           string    `gorm:"size:50" json:"action"`
	Status           string    `gorm:"size:50;default:pending" json:"status"`
	NotificationSent bool      `gorm:"default:false" json:"notification_sent"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (RetentionSchedule) TableName() string { return "retention_schedules" }

// ── SAVED SEARCH ─────────────────────────────────────────────────────────────

type SavedSearch struct {
	ID        string    `gorm:"type:char(36);primaryKey" json:"id"`
	UserID    string    `gorm:"type:char(36);not null" json:"user_id"`
	Name      string    `gorm:"size:255" json:"name"`
	Query     string    `gorm:"type:text" json:"query"`
	Filters   string    `gorm:"type:json" json:"filters"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (SavedSearch) TableName() string { return "saved_searches" }

// ── BACKUP LOG ────────────────────────────────────────────────────────────────

type BackupLog struct {
	ID                string     `gorm:"type:char(36);primaryKey" json:"id"`
	FileName          string     `gorm:"column:filename;size:255" json:"file_name"`
	FilePath          string     `gorm:"type:text" json:"file_path"`
	FileSize          int64      `json:"file_size"`
	BackupType        string     `gorm:"size:50" json:"backup_type"`
	Status            string     `gorm:"size:50;default:pending" json:"status"`
	CloudPath         string     `gorm:"type:text" json:"cloud_path"`
	CloudProvider     string     `gorm:"size:50" json:"cloud_provider"`
	GoogleDriveFileID string     `gorm:"size:255" json:"google_drive_file_id"`
	GoogleDriveURL    string     `gorm:"type:text" json:"google_drive_url"`
	Notes             string     `gorm:"type:text" json:"notes"`
	CreatedAt         time.Time  `json:"created_at"`
	CompletedAt       *time.Time `json:"completed_at"`
}

func (BackupLog) TableName() string { return "backup_logs" }

func (l *BackupLog) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	return nil
}

func (b BackupLog) Filename() string {
	return b.FileName
}

func (b BackupLog) SizeFormatted() string {
	const (
		B  = 1
		KB = 1024 * B
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case b.FileSize >= GB:
		return fmt.Sprintf("%.2f GB", float64(b.FileSize)/GB)
	case b.FileSize >= MB:
		return fmt.Sprintf("%.2f MB", float64(b.FileSize)/MB)
	case b.FileSize >= KB:
		return fmt.Sprintf("%.2f KB", float64(b.FileSize)/KB)
	default:
		return fmt.Sprintf("%d B", b.FileSize)
	}
}

// ── TENANT ────────────────────────────────────────────────────────────────────

type Tenant struct {
	ID        string    `gorm:"type:char(36);primaryKey" json:"id"`
	Name      string    `gorm:"size:255;not null" json:"name"`
	Domain    string    `gorm:"size:255;unique" json:"domain"`
	IsActive  bool      `gorm:"default:true" json:"is_active"`
	Settings  string    `gorm:"type:json" json:"settings"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ── NOTIFICATION ─────────────────────────────────────────────────────────────

type Notification struct {
	ID             string     `gorm:"type:char(36);primaryKey" json:"id"`
	Type           string     `gorm:"size:255" json:"type"`
	NotifiableType string     `gorm:"size:255" json:"notifiable_type"`
	NotifiableID   string     `gorm:"type:char(36)" json:"notifiable_id"`
	Data           string     `gorm:"type:text" json:"data"`
	ReadAt         *time.Time `json:"read_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (Notification) TableName() string { return "notifications" }

// ── INTEGRATION ──────────────────────────────────────────────────────────────

type Integration struct {
	ID         string         `gorm:"type:char(36);primaryKey" json:"id"`
	Name       string         `gorm:"size:255;not null" json:"name"`
	Type       string         `gorm:"size:50" json:"type"`
	BaseURL    string         `gorm:"type:text" json:"base_url"`
	ApiKey     string         `gorm:"type:text" json:"api_key"`
	Config     string         `gorm:"type:json" json:"config"`
	IsActive   bool           `gorm:"default:false" json:"is_active"`
	LastSyncAt *time.Time     `json:"last_sync_at"`
	LastStatus string         `gorm:"size:50" json:"last_status"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (Integration) TableName() string { return "integrations" }

type IntegrationLog struct {
	ID            string    `gorm:"type:char(36);primaryKey" json:"id"`
	IntegrationID string    `gorm:"type:char(36);not null" json:"integration_id"`
	Action        string    `gorm:"size:100" json:"action"`
	Status        string    `gorm:"size:50" json:"status"`
	RequestBody   string    `gorm:"type:text" json:"request_body"`
	ResponseBody  string    `gorm:"type:text" json:"response_body"`
	StatusCode    int       `json:"status_code"`
	DurationMs    int       `json:"duration_ms"`
	CreatedAt     time.Time `json:"created_at"`
}

func (IntegrationLog) TableName() string { return "integration_logs" }

func (l *IntegrationLog) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	return nil
}

// ── IMPORT EXPORT JOB ────────────────────────────────────────────────────────

type ImportExportJob struct {
	ID            string     `gorm:"type:char(36);primaryKey" json:"id"`
	UserID        string     `gorm:"type:char(36)" json:"user_id"`
	Type          string     `gorm:"size:50" json:"type"`
	EntityType    string     `gorm:"size:100" json:"entity_type"`
	Status        string     `gorm:"size:50;default:pending" json:"status"`
	TotalRows     int        `gorm:"default:0" json:"total_rows"`
	ProcessedRows int        `gorm:"default:0" json:"processed_rows"`
	ErrorRows     int        `gorm:"default:0" json:"error_rows"`
	InputFile     string     `gorm:"type:text" json:"input_file"`
	OutputFile    string     `gorm:"type:text" json:"output_file"`
	ErrorLog      string     `gorm:"type:text" json:"error_log"`
	CreatedAt     time.Time  `json:"created_at"`
	CompletedAt   *time.Time `json:"completed_at"`
}

func (ImportExportJob) TableName() string { return "import_export_jobs" }

// ── DASHBOARD WIDGET ─────────────────────────────────────────────────────────

type DashboardWidget struct {
	ID        string    `gorm:"type:char(36);primaryKey" json:"id"`
	UserID    string    `gorm:"type:char(36);not null" json:"user_id"`
	WidgetKey string    `gorm:"size:100" json:"widget_key"`
	Position  int       `gorm:"default:0" json:"position"`
	IsVisible bool      `gorm:"default:true" json:"is_visible"`
	Config    string    `gorm:"type:json" json:"config"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (DashboardWidget) TableName() string { return "dashboard_widgets" }

// ── DISPOSAL SCHEDULE ────────────────────────────────────────────────────────

type DisposalSchedule struct {
	ID                string           `gorm:"type:char(36);primaryKey" json:"id"`
	KodeKlasifikasiID string           `gorm:"type:char(36)" json:"kode_klasifikasi_id"`
	ArsipID           string           `gorm:"type:char(36)" json:"arsip_id"`
	ScheduledDate     time.Time        `json:"scheduled_date"`
	Action            string           `gorm:"size:50" json:"action"`
	Status            string           `gorm:"size:50;default:pending" json:"status"`
	ExecutedAt        *time.Time       `json:"executed_at"`
	CreatedBy         *string          `gorm:"type:char(36)" json:"created_by"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
	DeletedAt         gorm.DeletedAt   `gorm:"index" json:"deleted_at"`
	KodeKlasifikasi   *KodeKlasifikasi `gorm:"foreignKey:KodeKlasifikasiID" json:"kode_klasifikasi,omitempty"`
	Arsip             *Arsip           `gorm:"foreignKey:ArsipID" json:"arsip,omitempty"`
}

func (DisposalSchedule) TableName() string { return "disposal_schedules" }

// ── EMAIL NOTIFICATION ───────────────────────────────────────────────────────

type EmailNotification struct {
	ID        string     `gorm:"type:char(36);primaryKey" json:"id"`
	UserID    *string    `gorm:"type:char(36)" json:"user_id"`
	Subject   string     `gorm:"size:255" json:"subject"`
	Body      string     `gorm:"type:text" json:"body"`
	Status    string     `gorm:"size:50;default:pending" json:"status"`
	SentAt    *time.Time `json:"sent_at"`
	CreatedAt time.Time  `json:"created_at"`
}

func (EmailNotification) TableName() string { return "email_notifications" }

func (l *EmailNotification) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	return nil
}

// ── COMPLIANCE SCORE ─────────────────────────────────────────────────────────

type ComplianceScore struct {
	ID                     uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UnitKerjaID            string    `gorm:"type:char(36);not null" json:"unit_kerja_id"`
	OverallScore           float64   `json:"overall_score"`
	ClassificationAccuracy float64   `json:"classification_accuracy"`
	PemberkasanCompliance  float64   `json:"pemberkasan_compliance"`
	RetentionEfficiency    float64   `json:"retention_efficiency"`
	AuditDate              string    `gorm:"type:date" json:"audit_date"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

func (ComplianceScore) TableName() string { return "compliance_scores" }

// ── LEADERBOARD STAT ─────────────────────────────────────────────────────────

type LeaderboardStat struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UnitKerjaID string    `gorm:"type:char(36);not null" json:"unit_kerja_id"`
	TotalPoints int       `json:"total_points"`
	MonthlyRank *int      `json:"monthly_rank"`
	BadgeName   string    `gorm:"size:100;default:'Bronze'" json:"badge_name"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (LeaderboardStat) TableName() string { return "leaderboard_stats" }

// ── NOTIFICATION PREFERENCE ──────────────────────────────────────────────────

type NotificationPreference struct {
	ID               string    `gorm:"type:char(36);primaryKey" json:"id"`
	UserID           string    `gorm:"type:char(36);not null;uniqueIndex:idx_user_notif_type" json:"user_id"`
	NotificationType string    `gorm:"size:100;uniqueIndex:idx_user_notif_type" json:"notification_type"`
	EmailEnabled     bool      `gorm:"default:true" json:"email_enabled"`
	InAppEnabled     bool      `gorm:"default:true" json:"in_app_enabled"`
	WhatsappEnabled  bool      `gorm:"default:false" json:"whatsapp_enabled"`
	QuietStart       *string   `gorm:"size:5" json:"quiet_start"`
	QuietEnd         *string   `gorm:"size:5" json:"quiet_end"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (NotificationPreference) TableName() string { return "notification_preferences" }

// ── NOTIFICATION TEMPLATE ────────────────────────────────────────────────────

type NotificationTemplate struct {
	ID              string    `gorm:"type:char(36);primaryKey" json:"id"`
	Name            string    `gorm:"size:255;not null" json:"name"`
	Code            string    `gorm:"size:100;unique" json:"code"`
	Type            string    `gorm:"size:50" json:"type"`
	SubjectTemplate string    `gorm:"type:text" json:"subject_template"`
	BodyTemplate    string    `gorm:"type:text" json:"body_template"`
	Variables       string    `gorm:"type:json" json:"variables"`
	IsActive        bool      `gorm:"default:true" json:"is_active"`
	TenantID        *string   `gorm:"type:char(36)" json:"tenant_id"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (NotificationTemplate) TableName() string { return "notification_templates" }

// ── RETENTION NOTIFICATION ───────────────────────────────────────────────────

type RetentionNotification struct {
	ID                  string     `gorm:"type:char(36);primaryKey" json:"id"`
	RetentionScheduleID string     `gorm:"type:char(36)" json:"retention_schedule_id"`
	UserID              string     `gorm:"type:char(36);not null" json:"user_id"`
	Type                string     `gorm:"size:50" json:"type"`
	Channel             string     `gorm:"size:50" json:"channel"`
	Status              string     `gorm:"size:50;default:pending" json:"status"`
	Message             string     `gorm:"type:text" json:"message"`
	SentAt              *time.Time `json:"sent_at"`
	ReadAt              *time.Time `json:"read_at"`
	Metadata            string     `gorm:"type:json" json:"metadata"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

func (RetentionNotification) TableName() string { return "retention_notifications" }

// ── DESTRUCTION APPROVAL ─────────────────────────────────────────────────────

type DestructionApproval struct {
	ID                  string     `gorm:"type:char(36);primaryKey" json:"id"`
	RetentionScheduleID string     `gorm:"type:char(36);not null;uniqueIndex:idx_ret_sched_approver" json:"retention_schedule_id"`
	ApproverID          string     `gorm:"type:char(36);not null;uniqueIndex:idx_ret_sched_approver" json:"approver_id"`
	Status              string     `gorm:"size:20;default:pending" json:"status"`
	Notes               string     `gorm:"type:text" json:"notes"`
	SignatureHash       string     `gorm:"size:255" json:"signature_hash"`
	DecidedAt           *time.Time `json:"decided_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

func (DestructionApproval) TableName() string { return "destruction_approvals" }

// ── COMPLIANCE RISK ──────────────────────────────────────────────────────────

type ComplianceRisk struct {
	ID          uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	UUID        string         `gorm:"type:char(36);unique" json:"uuid"`
	Title       string         `gorm:"size:255;not null" json:"title"`
	Description string         `gorm:"type:text" json:"description"`
	RiskLevel   string         `gorm:"size:50" json:"risk_level"`
	Category    string         `gorm:"size:100" json:"category"`
	Status      string         `gorm:"size:20;default:open" json:"status"`
	AssignedTo  *string        `gorm:"type:char(36)" json:"assigned_to"`
	DueDate     *time.Time     `json:"due_date"`
	Resolution  string         `gorm:"type:text" json:"resolution"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (ComplianceRisk) TableName() string { return "compliance_risks" }

// ── AI CATEGORIZATION ─────────────────────────────────────────────────────────

type AiCategorization struct {
	ID                  string    `gorm:"type:char(36);primaryKey" json:"id"`
	ArsipID             string    `gorm:"type:char(36);not null;index" json:"arsip_id"`
	PredictedCategory   *string   `gorm:"size:255;index" json:"predicted_category"`
	PredictedJenisArsip *string   `gorm:"size:255;index" json:"predicted_jenis_arsip"`
	PredictedUnitKerja  *string   `gorm:"size:255;index" json:"predicted_unit_kerja"`
	Predictions         *string   `gorm:"type:json" json:"predictions"`
	ConfidenceScore     *int      `json:"confidence_score"`
	WasAccepted         bool      `gorm:"default:false;index" json:"was_accepted"`
	ProcessedBy         *string   `gorm:"type:char(36);index" json:"processed_by"`
	ProcessingTimeMs    *int      `json:"processing_time_ms"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (AiCategorization) TableName() string { return "ai_categorizations" }

// ── API TOKEN ─────────────────────────────────────────────────────────────────

type ApiToken struct {
	ID         uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	TokenID    string     `gorm:"type:char(36);unique" json:"token_id"`
	UserID     string     `gorm:"type:char(36);not null;index" json:"user_id"`
	Name       string     `gorm:"size:255;not null" json:"name"`
	Token      string     `gorm:"size:64;not null;unique" json:"-"`
	Abilities  *string    `gorm:"type:text" json:"abilities"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (ApiToken) TableName() string { return "api_tokens" }

// ── DIGITAL SIGNATURE ─────────────────────────────────────────────────────────

type DigitalSignature struct {
	ID            string     `gorm:"type:char(36);primaryKey" json:"id"`
	UUID          string     `gorm:"type:char(36);unique" json:"uuid"`
	UserID        string     `gorm:"type:char(36);not null;index" json:"user_id"`
	EntityType    string     `gorm:"size:255;not null" json:"entity_type"`
	EntityID      string     `gorm:"type:char(36);not null;index" json:"entity_id"`
	CertificateID *string    `gorm:"size:255" json:"certificate_id"`
	SignatureData *string    `gorm:"type:text" json:"signature_data"`
	Status        string     `gorm:"size:20;default:pending;index" json:"status"`
	SignedAt      *time.Time `json:"signed_at"`
	Reason        *string    `gorm:"type:text" json:"reason"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (DigitalSignature) TableName() string { return "digital_signatures" }

// ── WEBHOOK ───────────────────────────────────────────────────────────────────

type Webhook struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UUID      string    `gorm:"type:char(36);unique" json:"uuid"`
	Name      string    `gorm:"size:255;not null" json:"name"`
	URL       string    `gorm:"size:255;not null" json:"url"`
	Secret    *string   `gorm:"size:255" json:"-"`
	Events    string    `gorm:"type:json;not null" json:"events"`
	Active    bool      `gorm:"default:true;index" json:"active"`
	VerifySSL bool      `gorm:"default:true" json:"verify_ssl"`
	CreatedBy string    `gorm:"type:char(36);not null" json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Webhook) TableName() string { return "webhooks" }

// ── WEBHOOK LOG ───────────────────────────────────────────────────────────────

type WebhookLog struct {
	ID             uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	WebhookID      uint      `gorm:"not null;index" json:"webhook_id"`
	Event          string    `gorm:"size:255;not null" json:"event"`
	Payload        string    `gorm:"type:json;not null" json:"payload"`
	ResponseStatus *int      `json:"response_status"`
	ResponseBody   *string   `gorm:"type:text" json:"response_body"`
	Success        bool      `gorm:"default:false;index" json:"success"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (WebhookLog) TableName() string { return "webhook_logs" }

// ── WORKFLOW ──────────────────────────────────────────────────────────────────

type Workflow struct {
	ID          uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	UUID        string         `gorm:"type:char(36);unique" json:"uuid"`
	Name        string         `gorm:"size:255;not null" json:"name"`
	Description *string        `gorm:"type:text" json:"description"`
	EntityType  string         `gorm:"size:255;not null;index" json:"entity_type"`
	Steps       string         `gorm:"type:json;not null" json:"steps"`
	Active      bool           `gorm:"default:true;index" json:"active"`
	CreatedBy   string         `gorm:"type:char(36);not null" json:"created_by"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (Workflow) TableName() string { return "workflows" }

// ── WORKFLOW EXECUTION ────────────────────────────────────────────────────────

type WorkflowExecution struct {
	ID          string    `gorm:"type:char(36);primaryKey" json:"id"`
	UUID        string    `gorm:"type:char(36);unique" json:"uuid"`
	WorkflowID  uint      `gorm:"not null;index" json:"workflow_id"`
	EntityID    string    `gorm:"type:char(36);not null;index" json:"entity_id"`
	EntityType  string    `gorm:"size:255;not null" json:"entity_type"`
	CurrentStep *string   `gorm:"size:255" json:"current_step"`
	Status      string    `gorm:"size:20;default:pending;index" json:"status"`
	StepResults *string   `gorm:"type:json" json:"step_results"`
	ExecutedBy  string    `gorm:"type:char(36);not null" json:"executed_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (WorkflowExecution) TableName() string { return "workflow_executions" }

// ── BERITA ACARA PEMINDAHAN ────────────────────────────────────────────────────

type BeritaAcara struct {
	ID                 string             `gorm:"type:char(36);primaryKey" json:"id"`
	NomorBA            string             `gorm:"column:nomor_ba;size:100;not null" json:"nomor_ba"`
	Tanggal            time.Time          `gorm:"column:tanggal" json:"tanggal"`
	Tempat             string             `gorm:"column:tempat;size:255" json:"tempat"`
	PihakPertamaNama   string             `gorm:"column:pihak_pertama_nama;size:255;not null" json:"pihak_pertama_nama"`
	PihakPertamaNIP    string             `gorm:"column:pihak_pertama_nip;size:100" json:"pihak_pertama_nip"`
	PihakPertamaJabatan string            `gorm:"column:pihak_pertama_jabatan;size:255" json:"pihak_pertama_jabatan"`
	PihakPertamaUnit   string             `gorm:"column:pihak_pertama_unit;size:255" json:"pihak_pertama_unit"`
	PihakKeduaNama     string             `gorm:"column:pihak_kedua_nama;size:255;not null" json:"pihak_kedua_nama"`
	PihakKeduaNIP      string             `gorm:"column:pihak_kedua_nip;size:100" json:"pihak_kedua_nip"`
	PihakKeduaJabatan  string             `gorm:"column:pihak_kedua_jabatan;size:255" json:"pihak_kedua_jabatan"`
	PihakKeduaUnit     string             `gorm:"column:pihak_kedua_unit;size:255" json:"pihak_kedua_unit"`
	LokasiAsalID       *string            `gorm:"column:lokasi_asal_id;type:char(36)" json:"lokasi_asal_id"`
	LokasiTujuanID     *string            `gorm:"column:lokasi_tujuan_id;type:char(36)" json:"lokasi_tujuan_id"`
	Catatan            string             `gorm:"type:text" json:"catatan"`
	CreatedBy          string             `gorm:"column:created_by;type:char(36)" json:"created_by"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
	DeletedAt          gorm.DeletedAt     `gorm:"index" json:"deleted_at"`
	Items              []BeritaAcaraItem  `gorm:"foreignKey:BeritaAcaraID" json:"items,omitempty"`
	LokasiAsal         *LokasiArsip       `gorm:"foreignKey:LokasiAsalID" json:"lokasi_asal,omitempty"`
	LokasiTujuan       *LokasiArsip       `gorm:"foreignKey:LokasiTujuanID" json:"lokasi_tujuan,omitempty"`
	Creator            *User              `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
}

func (BeritaAcara) TableName() string { return "berita_acara" }

type BeritaAcaraItem struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	BeritaAcaraID string    `gorm:"column:berita_acara_id;type:char(36);not null;index" json:"berita_acara_id"`
	ArsipID       string    `gorm:"column:arsip_id;type:char(36);not null" json:"arsip_id"`
	CreatedAt     time.Time `json:"created_at"`
	BeritaAcara   *BeritaAcara `gorm:"foreignKey:BeritaAcaraID" json:"berita_acara,omitempty"`
	Arsip         *Arsip       `gorm:"foreignKey:ArsipID" json:"arsip,omitempty"`
}

func (BeritaAcaraItem) TableName() string { return "berita_acara_items" }

// ── SEARCH LOG ────────────────────────────────────────────────────────────────

type SearchLog struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID       *string   `gorm:"type:char(36);index" json:"user_id"`
	SearchTerm   string    `gorm:"size:255;not null;index" json:"search_term"`
	ResultsCount int       `gorm:"default:0" json:"results_count"`
	Filters      *string   `gorm:"type:json" json:"filters"`
	IPAddress    *string   `gorm:"size:45" json:"ip_address"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (SearchLog) TableName() string { return "search_logs" }
