package desksync

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"arsippro/internal/database"
	"arsippro/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// connectTestDB membuka file SQLite baru dan membuatnya database global
// (database.DB) milik paket database. Setiap pemanggilan menggeser global —
// dipakai berurutan: first remote (pegang handle), then local.
func connectTestDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	if err := database.ConnectSQLite(path); err != nil {
		t.Fatalf("gagal buka sqlite %s: %v", path, err)
	}
	if err := database.MigrateLocal(); err != nil {
		t.Fatalf("migrasi lokal gagal: %v", err)
	}
	return database.DB
}

// TestEngineLocalRemoteRoundTrip — skenario penuh:
//  1. seed bootstrap lokal (tidak ikut push)
//  2. buat arsip + lampiran + jenis lokal → push mencapai pusat
//     termasuk file BLOB & remap FK jenis
//  3. edit pusat lebih baru → pull LWW diterapkan lokal
//  4. tombstone pusat → lokal ikut terhapus (soft)
//  5. siklus berikutnya tanpa perubahan → nol push/pull (idempoten)
func TestEngineLocalRemoteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	remote := connectTestDB(t, filepath.Join(dir, "remote.db"))
	_ = connectTestDB(t, filepath.Join(dir, "local.db"))
	local := database.DB

	if err := database.SeedLocalBootstrap(); err != nil {
		t.Fatalf("seed bootstrap: %v", err)
	}
	time.Sleep(5 * time.Millisecond) // pastikan ts baris > watermark bootstrap

	// ── jenis arsip lokal (UUID terisi BeforeCreate) ──
	jenis := models.JenisArsip{KodeJenis: "KU", NamaJenis: "Kwitansi Uji"}
	if err := local.Create(&jenis).Error; err != nil {
		t.Fatalf("create jenis: %v", err)
	}

	// ── arsip + lampiran di disk ──
	content := []byte("PDF-LAMPIRAN-UJI")
	upPath := filepath.Join(dir, "uploads", "arsip", "u1_abcd.pdf")
	if err := os.MkdirAll(filepath.Dir(upPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(upPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	arsipRow := models.Arsip{
		ID:            uuid.NewString(),
		NamaArsip:     "Laporan Uji",
		Uraian:        "versi awal",
		StatusArsip:   "Aktif",
		FilePath:      upPath,
		FileName:      "u1_abcd.pdf",
		JenisArsipID:  &jenis.ID,
		TanggalDibuat: funcPtr(xnow()),
	}
	if err := local.Create(&arsipRow).Error; err != nil {
		t.Fatalf("create arsip: %v", err)
	}

	// ── engine dengan pusat palsu (sqlite kedua) ──
	engine := New(Params{
		ClientID: "test-local", Interval: time.Hour,
		PullFileBlobs: true, FileMax: 10 << 20,
	})
	engine.openRemote = func() (*gorm.DB, func(), error) { return remote, func() {}, nil }

	if err := engine.runOnce(); err != nil {
		t.Fatalf("siklus 1: %v", err)
	}

	// arsampai di pusat?
	var pushed models.Arsip
	if err := remote.Unscoped().Where("id = ?", arsipRow.ID).First(&pushed).Error; err != nil {
		t.Fatalf("arsip tidak sampai pusat: %v", err)
	}
	if string(pushed.FileContent) != string(content) {
		t.Fatalf("lampiran tidak terbawa (len=%d)", len(pushed.FileContent))
	}
	if pushed.JenisArsipID == nil || *pushed.JenisArsipID == jenis.ID {
		t.Fatalf("FK jenis tidak diremap ke id pusat (get %v)", pushed.JenisArsipID)
	}
	if n := database.OutboxCountPending(); n != 0 {
		t.Fatalf("outbox belum kosong setelah push: %d", n)
	}

	// ── edit pusat lebih baru → pull diterapkan ──
	edited := time.Now().Add(time.Minute)
	if err := remote.Model(&models.Arsip{}).Where("id = ?", arsipRow.ID).
		Updates(map[string]any{"uraian": "Diedit di pusat", "updated_at": edited}).Error; err != nil {
		t.Fatalf("edit pusat: %v", err)
	}
	if err := engine.runOnce(); err != nil {
		t.Fatalf("siklus 2: %v", err)
	}
	var merged models.Arsip
	if err := local.Unscoped().Where("id = ?", arsipRow.ID).First(&merged).Error; err != nil {
		t.Fatalf("arsip lokal hilang: %v", err)
	}
	if merged.Uraian != "Diedit di pusat" {
		t.Fatalf("pull LWW tidak dijalankan; uraian=%q", merged.Uraian)
	}

	// ── tombstone di pusat → lokal ikut lembut terhapus ──
	// (delete BIASA = soft delete; Unscoped hanya untuk query pembacaan)
	if err := remote.Where("id = ?", arsipRow.ID).Delete(&models.Arsip{}).Error; err != nil {
		t.Fatalf("tombstone pusat: %v", err)
	}
	// Pastikan pusat benar2 menyimpan tombstone (deleted_at) — tidak hard.
	var del models.Arsip
	if err := remote.Unscoped().Where("id = ?", arsipRow.ID).First(&del).Error; err != nil || !del.DeletedAt.Valid {
		t.Fatalf("pusat tidak menyimpan tombstone: %v", err)
	}
	if err := engine.runOnce(); err != nil {
		t.Fatalf("siklus 3: %v", err)
	}
	var tomb models.Arsip
	if err := local.Unscoped().Where("id = ?", arsipRow.ID).First(&tomb).Error; err != nil {
		t.Fatalf("tombstone hilang dari lokal: %v", err)
	}
	if !tomb.DeletedAt.Valid {
		t.Fatal("lokal tidak menandai deleted_at")
	}

	// ── idempotensi: tanpa perubahan → tidak ada lalu lintas ──
	if err := engine.runOnce(); err != nil {
		t.Fatalf("siklus 4 (idempoten): %v", err)
	}
	st := engine.Snapshot()
	if st.LastPushCount != 0 || st.LastPullCount != 0 {
		t.Fatalf("siklus diam seharusnya tanpa lalu lintas: push=%d pull=%d", st.LastPushCount, st.LastPullCount)
	}
	if !st.Online {
		t.Fatal("status seharusnya online")
	}
	if st.LastError != "" {
		t.Fatalf("error tidak diduga: %v", st.LastError)
	}
}

func TestSyncHooksRecordOutbox(t *testing.T) {
	dir := t.TempDir()
	_ = connectTestDB(t, filepath.Join(dir, "hooks.db"))
	db := database.DB

	if err := database.SeedLocalBootstrap(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	time.Sleep(5 * time.Millisecond)

	// create → outbox
	arsip := models.Arsip{ID: uuid.NewString(), NamaArsip: "hook arsip", StatusArsip: "Aktif"}
	if err := db.Create(&arsip).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if n := database.OutboxCountPending(); n == 0 {
		t.Fatal("outbox kosong setelah create")
	}

	// update → coalesce (tetap satu entri untuk baris yang sama)
	arsip.NamaArsip = "hook arsip 2"
	if err := db.Save(&arsip).Error; err != nil {
		t.Fatalf("save: %v", err)
	}

	// delete (soft) → action terakhir menang
	if err := db.Delete(&arsip).Error; err != nil {
		t.Fatalf("delete: %v", err)
	}
	var ob models.SyncOutbox
	if err := db.Where("table_name = ? AND row_key = ?", "arsip", arsip.ID).Order("id DESC").First(&ob).Error; err != nil {
		t.Fatalf("entri outbox missing: %v", err)
	}
	if ob.Action != models.OutboxActionDelete {
		t.Fatalf("aksi seharusnya delete, dapat %q", ob.Action)
	}

	// tabel non-syncable → tidak terekam
	before := database.OutboxCountPending()
	db.Create(&models.LoginLog{ID: uuid.NewString(), LoginTime: time.Now(), Status: "success"})
	if after := database.OutboxCountPending(); after != before {
		t.Fatalf("login_logs tidak seharusnya terekam (%d→%d)", before, after)
	}
}

// TestSeedLocalBootstrapPortable memastikan seed portabel & tidak menduplikat.
func TestSeedLocalBootstrapPortable(t *testing.T) {
	dir := t.TempDir()
	_ = connectTestDB(t, filepath.Join(dir, "seed.db"))
	db := database.DB

	if err := database.SeedLocalBootstrap(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var jenis []models.JenisArsip
	db.Find(&jenis)
	if len(jenis) == 0 {
		t.Fatal("jenis arsip tidak terseed")
	}
	seen := map[string]bool{}
	for _, j := range jenis {
		if seen[j.KodeJenis] {
			t.Fatalf("duplikat kode jenis %s", j.KodeJenis)
		}
		seen[j.KodeJenis] = true
	}
	var users int64
	db.Model(&models.User{}).Count(&users)
	if users == 0 {
		t.Fatal("admin seed tidak dibuat")
	}
}

func xnow() time.Time { return time.Now() }

func funcPtr(t time.Time) *time.Time { return &t }
