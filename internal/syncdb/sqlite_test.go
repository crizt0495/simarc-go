package syncdb

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"arsippro/internal/config"
)

// sqliteTimeFormat adalah format waktu yang dipakai driver SQLite untuk
// menyimpan timestamp. Semua nilai waktu pada tes ini memakai format yang
// sama supaya perbandingan leksikografis (dipakai mesin sinkron) konsisten.
const sqliteTimeFormat = "2006-01-02 15:04:05.999999999-07:00"

func openTestSQLite(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+
		"?_pragma=busy_timeout(15000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatalf("buka sqlite %s: %v", path, err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("ping sqlite %s: %v", path, err)
	}
	return db
}

func scalar(t *testing.T, db *sql.DB, query string, args ...any) string {
	t.Helper()
	var v sql.NullString
	if err := db.QueryRow(query, args...).Scan(&v); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return v.String
}

// newTestEngine menyalakan mesin sinkronisasi dengan kedua sisi SQLite.
func newTestEngine(t *testing.T, local, hub *sql.DB, hubPath string) *Engine {
	t.Helper()
	cfg := config.SyncConfig{
		Enabled:         true,
		NodeID:          "node-a",
		IntervalMinutes: 0,
		OverlapSeconds:  30,
		BatchSize:       100,
	}
	e, err := newEngine(context.Background(), local, driverSQLite, hub, driverSQLite, hubPath, cfg,
		func(format string, args ...any) { t.Logf(format, args...) })
	if err != nil {
		t.Fatalf("buka engine: %v", err)
	}
	t.Cleanup(e.Close)
	return e
}

// TestSQLiteConvergence membuktikan node SQLite dan hub SQLite converge:
// perubahan lokal naik ke hub, perubahan hub turun ke node, soft delete
// merambat dua arah, dan penghapusan permanen memakai tombstone.
func TestSQLiteConvergence(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	local := openTestSQLite(t, filepath.Join(dir, "node-a.db"))
	hub := openTestSQLite(t, filepath.Join(dir, "hub.db"))

	// Skema sama di kedua sisi: satu tabel soft-delete, satu tabel keyed.
	for _, db := range []*sql.DB{local, hub} {
		mustExec(t, db, `CREATE TABLE arsip (
			id text PRIMARY KEY,
			judul text,
			created_at datetime,
			updated_at datetime,
			deleted_at datetime
		)`)
		mustExec(t, db, `CREATE TABLE unit_kerja (
			id text PRIMARY KEY,
			nama text,
			created_at datetime,
			updated_at datetime
		)`)
	}

	e := newTestEngine(t, local, hub, filepath.Join(dir, "hub.db"))

	// 1. Insert lokal harus naik ke hub.
	t1 := time.Now().UTC().Format(sqliteTimeFormat)
	mustExec(t, local, `INSERT INTO arsip (id, judul, created_at, updated_at) VALUES (?,?,?,?)`,
		"arsip-1", "Draft lokal", t1, t1)
	mustExec(t, local, `INSERT INTO unit_kerja (id, nama, created_at, updated_at) VALUES (?,?,?,?)`,
		"uk-1", "Unit Satu", t1, t1)

	if _, err := e.Run(ctx); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	if got := scalar(t, hub, `SELECT judul FROM arsip WHERE id = 'arsip-1'`); got != "Draft lokal" {
		t.Fatalf("baris lokal belum naik ke hub, judul=%q", got)
	}
	if got := scalar(t, hub, `SELECT nama FROM unit_kerja WHERE id = 'uk-1'`); got != "Unit Satu" {
		t.Fatalf("unit_kerja lokal belum naik ke hub, nama=%q", got)
	}

	// 2. Perubahan di hub harus turun ke node (last-write-wins).
	t2 := time.Now().UTC().Add(time.Minute).Format(sqliteTimeFormat)
	mustExec(t, hub, `UPDATE arsip SET judul = 'Revisi dari hub', updated_at = ? WHERE id = 'arsip-1'`, t2)
	if _, err := e.Run(ctx); err != nil {
		t.Fatalf("run 2: %v", err)
	}
	if got := scalar(t, local, `SELECT judul FROM arsip WHERE id = 'arsip-1'`); got != "Revisi dari hub" {
		t.Fatalf("perubahan hub belum turun ke node, judul=%q", got)
	}

	// 3. Perubahan lokal yang lebih baru harus menang (LWW).
	t3 := time.Now().UTC().Add(2 * time.Minute).Format(sqliteTimeFormat)
	mustExec(t, local, `UPDATE arsip SET judul = 'Revisi dari node', updated_at = ? WHERE id = 'arsip-1'`, t3)
	if _, err := e.Run(ctx); err != nil {
		t.Fatalf("run 3: %v", err)
	}
	if got := scalar(t, hub, `SELECT judul FROM arsip WHERE id = 'arsip-1'`); got != "Revisi dari node" {
		t.Fatalf("LWW gagal, judul di hub=%q", got)
	}

	// 4. Soft delete di node harus merambat ke hub.
	t4 := time.Now().UTC().Add(3 * time.Minute).Format(sqliteTimeFormat)
	mustExec(t, local, `UPDATE arsip SET deleted_at = ?, updated_at = ? WHERE id = 'arsip-1'`, t4, t4)
	if _, err := e.Run(ctx); err != nil {
		t.Fatalf("run 4: %v", err)
	}
	if got := scalar(t, hub, `SELECT deleted_at FROM arsip WHERE id = 'arsip-1'`); got == "" {
		t.Fatal("soft delete di node belum merambat ke hub")
	}

	// 5. Penghapusan permanen (tabel tanpa deleted_at) memakai tombstone.
	mustExec(t, local, `DELETE FROM unit_kerja WHERE id = 'uk-1'`)
	if _, err := e.Run(ctx); err != nil {
		t.Fatalf("run 5: %v", err)
	}
	if got := scalar(t, hub, `SELECT COUNT(1) FROM unit_kerja WHERE id = 'uk-1'`); got != "0" {
		t.Fatalf("baris di hub belum terhapus oleh tombstone, count=%q", got)
	}
}

// TestSQLiteHubTableCreation memastikan tabel yang belum ada di hub dibuat
// ulang dari DDL SQLite sisi lokal (jalur dialek sama).
func TestSQLiteHubTableCreation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	local := openTestSQLite(t, filepath.Join(dir, "node-a.db"))
	hub := openTestSQLite(t, filepath.Join(dir, "hub.db"))

	mustExec(t, local, `CREATE TABLE arsip (
		id text PRIMARY KEY,
		judul text,
		created_at datetime,
		updated_at datetime,
		deleted_at datetime
	)`)

	cfg := config.SyncConfig{Enabled: true, NodeID: "node-a", BatchSize: 100, OverlapSeconds: 30}
	e, err := newEngine(ctx, local, driverSQLite, hub, driverSQLite, filepath.Join(dir, "hub.db"), cfg, nil)
	if err != nil {
		t.Fatalf("buka engine: %v", err)
	}
	defer e.Close()

	// Tabel "arsip" tidak ada di hub; mesin harus membuatnya sendiri.
	if err := ensureHubTable(ctx, e, "arsip"); err != nil {
		t.Fatalf("buat tabel di hub: %v", err)
	}
	exists, err := tableExistsIn(ctx, hub, driverSQLite, "arsip")
	if err != nil {
		t.Fatalf("cek tabel hub: %v", err)
	}
	if !exists {
		t.Fatal("tabel arsip belum dibuat di hub")
	}
}

// TestSQLiteTimeFormatRoundTrip memastikan waktu SQLite disimpan dengan format
// yang bisa dibandingkan leksikografis (dipakai mesin sinkronisasi untuk
// memilih perubahan terbaru) dan dibaca kembali tanpa kehilangan presisi.
func TestSQLiteTimeFormatRoundTrip(t *testing.T) {
	dir := t.TempDir()
	db := openTestSQLite(t, filepath.Join(dir, "waktu.db"))
	mustExec(t, db, `CREATE TABLE waktu (id text PRIMARY KEY, created_at datetime)`)

	now := time.Now()
	mustExec(t, db, `INSERT INTO waktu (id, created_at) VALUES (?,?)`, "w-1", now)

	// Teks yang benar-benar tersimpan (CAST menghindari konversi driver).
	raw := scalar(t, db, `SELECT CAST(created_at AS TEXT) FROM waktu WHERE id = 'w-1'`)
	want := now.Format(sqliteTimeFormat)
	if raw != want {
		t.Fatalf("format waktu SQLite tidak sesuai: %q != %q", raw, want)
	}
	if _, err := time.Parse(sqliteTimeFormat, raw); err != nil {
		t.Fatalf("waktu tidak bisa dibaca kembali: %v", err)
	}

	// Urutan leksikografis harus sama dengan urutan waktu sebenarnya.
	mustExec(t, db, `INSERT INTO waktu (id, created_at) VALUES (?,?)`, "w-2", now.Add(time.Second))
	older := scalar(t, db, `SELECT id FROM waktu WHERE created_at < ? ORDER BY id LIMIT 1`, now.Add(time.Second))
	if older != "w-1" {
		t.Fatalf("urutan leksikografis salah, baris terkecil=%q", older)
	}
}
