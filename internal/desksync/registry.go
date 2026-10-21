// Package desksync mengimplementasikan mesin sinkronisasi offline-first.
//
// Desktop bekerja di atas database SQLite primer (internal/database). Ketika
// MySQL pusat terjangkau (LAN/Aiven), mesin ini mengejar dua arah:
//
//	PUSH  — baris lokal yang berubah (outbox + rescan updated_at) dikirim ke
//	        pusat dengan aturan last-write-wins (LWW) memakai updated_at.
//	PULL  — baris pusat yang berubah sejak watermark terakhir ditarik dan
//	        diterapkan di lokal dengan LWW yang sama.
//
// Identitas baris lintas node memakai UUID (kolom id/uuid), semua tabel
// syncable punya salah satunya. Satu-satunya FK auto-increment yang ikut
// sync adalah arsip.jenis_arsip_id — diterjemahkan lewat sync_id_maps.
//
// File lampiran berpindah lewat kolom arsip.file_content (BLOB) dan
// dimaterialisasi ke disk saat diterima; QR dibuat ulang bila PNG hilang.
package desksync

import (
	"arsippro/internal/models"
)

// TableSpec mendefinisikan satu tabel yang ikut sinkronisasi.
type TableSpec struct {
	Table    string
	KeyCol   string // kolom identitas di DB: "id" (char36) atau "uuid"
	KeyProp  string // nama field struct: "ID" atau "UUID"
	Model    any    // contoh pointer struct
	TSProp   string // nama field timestamp untuk LWW: "UpdatedAt"/"CreatedAt"
	TSCol    string // nama kolom timestamp
	Soft     bool   // punya kolom deleted_at (tombstone)
	StripPK  bool   // PK auto-increment remote — jangan kirim nilai id (jenis_arsip & tabel item uint)
	NoChange bool   // tulis hanya bila berubah (masih dipakai LWW umum)
}

// specs daftar tabel sesuai urutan ketergantungan (induk dulu).
// role/permission join permission_role ditangani khusus di pull.
func specs() []TableSpec {
	return []TableSpec{
		{Table: "roles", KeyCol: "id", KeyProp: "ID", Model: &models.Role{}, TSProp: "UpdatedAt", TSCol: "updated_at", Soft: true},
		{Table: "permissions", KeyCol: "id", KeyProp: "ID", Model: &models.Permission{}, TSProp: "UpdatedAt", TSCol: "updated_at", Soft: true},
		{Table: "unit_kerja", KeyCol: "id", KeyProp: "ID", Model: &models.UnitKerja{}, TSProp: "UpdatedAt", TSCol: "updated_at", Soft: true},
		{Table: "users", KeyCol: "id", KeyProp: "ID", Model: &models.User{}, TSProp: "UpdatedAt", TSCol: "updated_at", Soft: true},
		{Table: "kode_klasifikasi", KeyCol: "id", KeyProp: "ID", Model: &models.KodeKlasifikasi{}, TSProp: "UpdatedAt", TSCol: "updated_at", Soft: true},
		{Table: "lokasi_arsips", KeyCol: "id", KeyProp: "ID", Model: &models.LokasiArsip{}, TSProp: "UpdatedAt", TSCol: "updated_at", Soft: true},
		{Table: "jenis_arsip", KeyCol: "uuid", KeyProp: "UUID", Model: &models.JenisArsip{}, TSProp: "UpdatedAt", TSCol: "updated_at", StripPK: true},
		{Table: "pemberkasan", KeyCol: "id", KeyProp: "ID", Model: &models.Pemberkasan{}, TSProp: "UpdatedAt", TSCol: "updated_at", Soft: true},
		{Table: "arsip", KeyCol: "id", KeyProp: "ID", Model: &models.Arsip{}, TSProp: "UpdatedAt", TSCol: "updated_at", Soft: true},
		{Table: "arsip_versions", KeyCol: "id", KeyProp: "ID", Model: &models.ArsipVersion{}, TSProp: "CreatedAt", TSCol: "created_at", Soft: true},
		{Table: "pemusnahan_arsip", KeyCol: "id", KeyProp: "ID", Model: &models.PemusnahanArsip{}, TSProp: "UpdatedAt", TSCol: "updated_at", Soft: true},
		{Table: "pemusnahan_arsip_items", KeyCol: "uuid", KeyProp: "UUID", Model: &models.PemusnahanItem{}, TSProp: "CreatedAt", TSCol: "created_at", StripPK: true},
		{Table: "jadwal_retensi", KeyCol: "id", KeyProp: "ID", Model: &models.JadwalRetensi{}, TSProp: "UpdatedAt", TSCol: "updated_at", Soft: true},
		{Table: "jadwal_retensi_arsip", KeyCol: "id", KeyProp: "ID", Model: &models.JadwalRetensiArsip{}, TSProp: "UpdatedAt", TSCol: "updated_at", Soft: false},
		{Table: "retention_schedules", KeyCol: "id", KeyProp: "ID", Model: &models.RetentionSchedule{}, TSProp: "UpdatedAt", TSCol: "updated_at", Soft: false},
		{Table: "destruction_approvals", KeyCol: "id", KeyProp: "ID", Model: &models.DestructionApproval{}, TSProp: "UpdatedAt", TSCol: "updated_at", Soft: false},
		{Table: "disposal_schedules", KeyCol: "id", KeyProp: "ID", Model: &models.DisposalSchedule{}, TSProp: "UpdatedAt", TSCol: "updated_at", Soft: true},
		{Table: "peminjaman_arsips", KeyCol: "id", KeyProp: "ID", Model: &models.PeminjamanArsip{}, TSProp: "UpdatedAt", TSCol: "updated_at", Soft: true},
		{Table: "berita_acara", KeyCol: "id", KeyProp: "ID", Model: &models.BeritaAcara{}, TSProp: "UpdatedAt", TSCol: "updated_at", Soft: true},
		{Table: "berita_acara_items", KeyCol: "uuid", KeyProp: "UUID", Model: &models.BeritaAcaraItem{}, TSProp: "CreatedAt", TSCol: "created_at", StripPK: true},
		{Table: "qr_codes", KeyCol: "id", KeyProp: "ID", Model: &models.QrCode{}, TSProp: "UpdatedAt", TSCol: "updated_at", Soft: true},
	}
}

func specByTable(table string) (TableSpec, bool) {
	for _, spec := range specs() {
		if spec.Table == table {
			return spec, true
		}
	}
	return TableSpec{}, false
}
