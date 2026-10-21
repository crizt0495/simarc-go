package database

import (
	"database/sql"
	"errors"
	"log"
	"reflect"
	"sync/atomic"
	"time"

	"arsippro/internal/config"
	"arsippro/internal/models"

	"gorm.io/gorm"
)

// ── OUTBOX HOOKS ──────────────────────────────────────────────────────────────
//
// Callback GORM yang menuliskan satu baris sync_outbox untuk setiap
// create/update/delete pada tabel syncable, atomik dalam transaksi yang sama.
// Mesin sync memakai outbox ini (plus rescan updated_at sebagai jaring
// pengaman) untuk menentukan baris yang perlu dikirim ke pusat.
//
// Pengecualian yang didokumentasikan:
//   - update massal via Table()/Model() tanpa nilai baris tunggal tidak
//     dapat di-snapshot per baris → tetap tertangkap oleh rescan watermark
//     (rescan memakai updated_at > last_push_at);
//   - pendeteksian delete memakai struct dest dari statement (pola yang
//     dipakai seluruh handler: db.Delete(&model)).

// hooksPaused menandai dinonaktifkannya pencatatan outbox sementara
// (seed, apply hasil pull, materialisasi file — semua tulisan yang bukan
// hasil tindakan pengguna).
var hooksPaused atomic.Bool

func PauseSyncHooks()       { hooksPaused.Store(true) }
func ResumeSyncHooks()      { hooksPaused.Store(false) }
func SyncHooksPaused() bool { return hooksPaused.Load() }

// syncTableDef mendefinisikan satu tabel syncable.
type syncTableDef struct {
	table  string // nama tabel di database
	model  any    // contoh pointer struct (untuk refleksi & reload baris)
	keyCol string // kolom identitas baris: "id" (char36) atau "uuid"
}

// Registri tabel syncable — subset data inti lihat docs/offline-first.md.
var syncRegistry = map[string]syncTableDef{
	"roles":                  {table: "roles", model: &models.Role{}, keyCol: "id"},
	"permissions":            {table: "permissions", model: &models.Permission{}, keyCol: "id"},
	"unit_kerja":             {table: "unit_kerja", model: &models.UnitKerja{}, keyCol: "id"},
	"users":                  {table: "users", model: &models.User{}, keyCol: "id"},
	"kode_klasifikasi":       {table: "kode_klasifikasi", model: &models.KodeKlasifikasi{}, keyCol: "id"},
	"lokasi_arsips":          {table: "lokasi_arsips", model: &models.LokasiArsip{}, keyCol: "id"},
	"jenis_arsip":            {table: "jenis_arsip", model: &models.JenisArsip{}, keyCol: "uuid"},
	"pemberkasan":            {table: "pemberkasan", model: &models.Pemberkasan{}, keyCol: "id"},
	"arsip":                  {table: "arsip", model: &models.Arsip{}, keyCol: "id"},
	"arsip_versions":         {table: "arsip_versions", model: &models.ArsipVersion{}, keyCol: "id"},
	"pemusnahan_arsip":       {table: "pemusnahan_arsip", model: &models.PemusnahanArsip{}, keyCol: "id"},
	"pemusnahan_arsip_items": {table: "pemusnahan_arsip_items", model: &models.PemusnahanItem{}, keyCol: "uuid"},
	"jadwal_retensi":         {table: "jadwal_retensi", model: &models.JadwalRetensi{}, keyCol: "id"},
	"jadwal_retensi_arsip":   {table: "jadwal_retensi_arsip", model: &models.JadwalRetensiArsip{}, keyCol: "id"},
	"retention_schedules":    {table: "retention_schedules", model: &models.RetentionSchedule{}, keyCol: "id"},
	"destruction_approvals":  {table: "destruction_approvals", model: &models.DestructionApproval{}, keyCol: "id"},
	"disposal_schedules":     {table: "disposal_schedules", model: &models.DisposalSchedule{}, keyCol: "id"},
	"peminjaman_arsips":      {table: "peminjaman_arsips", model: &models.PeminjamanArsip{}, keyCol: "id"},
	"berita_acara":           {table: "berita_acara", model: &models.BeritaAcara{}, keyCol: "id"},
	"berita_acara_items":     {table: "berita_acara_items", model: &models.BeritaAcaraItem{}, keyCol: "uuid"},
	"qr_codes":               {table: "qr_codes", model: &models.QrCode{}, keyCol: "id"},
}

// RegisterSyncHooks mendaftarkan callback outbox pada koneksi (lokal).
func RegisterSyncHooks(db *gorm.DB) {
	db.Callback().Create().After("gorm:create").Register("desksync:outbox-create", afterCreateOutbox)
	db.Callback().Update().After("gorm:update").Register("desksync:outbox-update", afterUpdateOutbox)
	db.Callback().Delete().After("gorm:delete").Register("desksync:outbox-delete", afterDeleteOutbox)
}

// shouldSkip memeriksa pause, syncable, dan satu baris struct.
func shouldSkipOutbox(tx *gorm.DB) (def syncTableDef, ok bool) {
	if SyncHooksPaused() || tx == nil || tx.Statement == nil {
		return def, false
	}
	table := tx.Statement.Table
	if table == "" && tx.Statement.Model != nil {
		table = lookupTableName(tx.Statement.Model)
	}
	if table == "" {
		return def, false
	}
	d, found := syncRegistry[table]
	if !found {
		return def, false
	}
	// Hanya destination struct tunggal yang bisa di-snapshot; slice & map
	// dilewatkan (ditangkap rescan watermark).
	if !isSingleStructDest(tx.Statement.Dest) {
		return d, false
	}
	return d, true
}

func lookupTableName(model any) string {
	if m, ok := model.(interface{ TableName() string }); ok {
		return m.TableName()
	}
	return ""
}

func isSingleStructDest(dest any) bool {
	if dest == nil {
		return false
	}
	t := reflect.TypeOf(dest)
	if t.Kind() != reflect.Pointer {
		return false
	}
	return t.Elem().Kind() == reflect.Struct
}

func afterCreateOutbox(tx *gorm.DB) {
	def, ok := shouldSkipOutbox(tx)
	if !ok {
		return
	}
	key := identityKeyFromDest(tx.Statement.Dest, def.keyCol)
	if key == "" {
		log.Printf("[OUTX] Lewati catat create %s: kunci tidak terbaca", def.table)
		return
	}
	recordOutbox(tx, def.table, key, models.OutboxActionCreate)
}

func afterUpdateOutbox(tx *gorm.DB) {
	def, ok := shouldSkipOutbox(tx)
	if !ok {
		return
	}
	key := identityKeyFromDest(tx.Statement.Dest, def.keyCol)
	if key == "" {
		// Update via Where() tanpa struct → ditangkap rescan watermark.
		return
	}
	recordOutbox(tx, def.table, key, models.OutboxActionUpdate)
}

func afterDeleteOutbox(tx *gorm.DB) {
	def, ok := shouldSkipOutbox(tx)
	if !ok {
		return
	}
	key := identityKeyFromDest(tx.Statement.Dest, def.keyCol)
	if key == "" {
		return
	}
	recordOutbox(tx, def.table, key, models.OutboxActionDelete)
}

// identityKeyFromDest mengambil nilai identitas baris (kolom uuid/id) dari
// struct dest. Semua tombol identitas syncable bertipe string (char36).
func identityKeyFromDest(dest any, keyCol string) string {
	rv := reflect.ValueOf(dest)
	for rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return ""
	}
	fieldName := propNameForColumn(keyCol)
	if fieldName == "" {
		return ""
	}
	f := rv.FieldByName(fieldName)
	if !f.IsValid() || f.Kind() != reflect.String {
		return ""
	}
	return f.String()
}

// propNameForColumn memetakan kolom identitas ke nama field Go.
func propNameForColumn(keyCol string) string {
	switch keyCol {
	case "uuid":
		return "UUID"
	case "id":
		return "ID"
	default:
		return ""
	}
}

// recordOutbox menulis/mengganti entri outbox untuk (table,row) — coalesce.
//
// Ditetapkan lewat ConnPool statement (koneksi/transaksi yang SAMA dengan
// tulisan barisnya => atomik) memakai SQL mentah: memanggil query GORM lain
// pada instance callback bakal mengotori statement yang sedang berjalan.
func recordOutbox(tx *gorm.DB, table, rowKey, action string) {
	if tx == nil || tx.Statement == nil || tx.Statement.ConnPool == nil {
		return
	}
	ctx := tx.Statement.Context
	pool := tx.Statement.ConnPool
	now := time.Now().UTC()
	clientID := config.App.SyncClientID

	var existingID int64
	var existingAction string
	err := pool.QueryRowContext(ctx,
		"SELECT id, action FROM sync_outbox WHERE table_name = ? AND row_key = ? AND deleted_at IS NULL ORDER BY id DESC LIMIT 1",
		table, rowKey,
	).Scan(&existingID, &existingAction)
	switch {
	case err == nil:
		if existingAction == action {
			return // suduh tercatat untuk aksi sama
		}
		if _, err := pool.ExecContext(ctx,
			"UPDATE sync_outbox SET action = ?, changed_at = ? WHERE id = ?",
			action, now, existingID,
		); err != nil {
			log.Printf("[OUTX] Gagal memperbarui outbox %s/%s: %v", table, rowKey, err)
		}
		return
	case errors.Is(err, sql.ErrNoRows):
		// tulis entri baru di bawah
	default:
		log.Printf("[OUTX] Baca outbox %s/%s gagal: %v", table, rowKey, err)
		return
	}

	if _, err := pool.ExecContext(ctx,
		"INSERT INTO sync_outbox (table_name, row_key, action, client_id, changed_at, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		table, rowKey, action, clientID, now, now,
	); err != nil {
		log.Printf("[OUTX] Gagal menulis outbox %s/%s: %v", table, rowKey, err)
	}
}

// OutboxCountPending menjumlah antrean terbuka (untuk badge UI).
func OutboxCountPending() int64 {
	var n int64
	DB.Model(&models.SyncOutbox{}).Where("deleted_at IS NULL").Count(&n)
	return n
}

// OutboxClearNow menandai seluruh antrean terbuka sebagai terkirim.
func OutboxClearNow() {
	DB.Where("deleted_at IS NULL").Delete(&models.SyncOutbox{})
}

// OutboxClearFor menandai antrean SATU tabel sebagai terkirim, kecuali kunci
// yang gagal dikirim (tetap menunggu siklus berikutnya).
func OutboxClearFor(table string, exclude map[string]bool) {
	keys := make([]string, 0, len(exclude))
	for k := range exclude {
		keys = append(keys, k)
	}
	q := DB.Model(&models.SyncOutbox{}).Where("table_name = ?", table)
	if len(keys) > 0 {
		q = q.Where("row_key NOT IN ?", keys)
	}
	if err := q.Update("deleted_at", time.Now()).Error; err != nil {
		log.Printf("[OUTX] Gagal membersihkan outbox %s: %v", table, err)
	}
}
