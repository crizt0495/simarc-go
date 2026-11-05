package database

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// LocalEngine adalah pojok akses database MySQL/MariaDB lokal.
// SELURUH akses data dari frontend berjalan di sini (backend Golang);
// antarmuka hanya memanggil binding Wails — tidak pernah menyentuh DB langsung.
type LocalEngine struct {
	db  *sql.DB
	dsn string
}

// NewLocalEngine membuka pool koneksi MySQL dan memverifikasinya.
func NewLocalEngine(dsn string) (*LocalEngine, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("buka database: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(3)
	db.SetConnMaxLifetime(4 * time.Hour)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("database ping: %w", err)
	}
	return &LocalEngine{db: db, dsn: dsn}, nil
}

// Retryable memeriksa apakah aplikasi masih bisa mencoba ulang koneksi
// (database belum siap saat komputer menyala, misalnya).
func Retryable(err error) bool { return err != nil }

// DSN koneksi aktif.
func (e *LocalEngine) DSN() string { return e.dsn }

// HealthCheck memverifikasi server database merespons.
func (e *LocalEngine) HealthCheck() error {
	if e == nil || e.db == nil {
		return errors.New("database belum aktif")
	}
	return e.db.Ping()
}

// DB memberikan akses dokol untuk paket lain (sync engine).
func (e *LocalEngine) DB() *sql.DB { return e.db }

// Close menutup pool koneksi.
func (e *LocalEngine) Close() error {
	if e == nil || e.db == nil {
		return nil
	}
	return e.db.Close()
}

// ── Migrasi ────────────────────────────────────────────────────────────────

// MigrateFS menerapkan seluruh file .sql pada fsys secara urut (aus path),
// hanya sekali per file. Pelacak: tabel schema_migrations.
// DSN wajib menyertakan multiStatements=true agar file multi-statement jalan.
func MigrateFS(db *sql.DB, fsys fs.FS) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations(
        name VARCHAR(255) PRIMARY KEY,
        applied_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3)
    ) ENGINE=InnoDB`); err != nil {
		return fmt.Errorf("buat schema_migrations: %w", err)
	}

	var files []string
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(p, ".sql") {
			return nil
		}
		files = append(files, p)
		return nil
	})
	if err != nil {
		return fmt.Errorf("baca daftar migrasi: %w", err)
	}
	sort.Strings(files) // migrations/central/001… sebelum local/002…

	for _, name := range files {
		var done int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name=?`, name).Scan(&done); err != nil {
			return fmt.Errorf("cek %s: %w", name, err)
		}
		if done > 0 {
			continue
		}
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("baca %s: %w", name, err)
		}
		if _, err := db.Exec(string(b)); err != nil {
			return fmt.Errorf("terapkan %s: %w", name, err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations(name) VALUES(?)`, name); err != nil {
			return fmt.Errorf("catat %s: %w", name, err)
		}
	}
	return nil
}

// Migrate menjalankan migrasi embet (fs dari //go:embed di package main).
func (e *LocalEngine) Migrate(fsys fs.FS) error {
	return MigrateFS(e.db, fsys)
}

// SchemaError mencari apa pun yang bersifat fatal pada struktur database.
func (e *LocalEngine) SchemaError() error {
	var tables int
	err := e.db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables
        WHERE table_schema=DATABASE() AND table_name IN ('arsip','pemberkasan','users','roles','sync_queue')`).Scan(&tables)
	if err != nil {
		return err
	}
	if tables < 5 {
		return fmt.Errorf("skema belum lengkap (%d/5 tabel)", tables)
	}
	return nil
}

// AppliedMigrations mengembalikan daftar migrasi ter-terapkan (untuk UI).
func (e *LocalEngine) AppliedMigrations() ([]string, error) {
	rows, err := e.db.Query(`SELECT name FROM schema_migrations ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err == nil {
			out = append(out, n)
		}
	}
	return out, rows.Err()
}

// ── Arsip CRUD ────────────────────────────────────────────────────────────

// Arsip minimal untuk desktop (pilih kolom eksplisit).
type Arsip struct {
	ID       string `json:"id"` // hex UUID
	Nama     string `json:"nama"`
	Kategori string `json:"kategori"`
	Status   string `json:"status"`
	FileSize int64  `json:"file_size"`
	Created  string `json:"created"`
	Updated  string `json:"updated"`
}

func NewUUID() (binary [16]byte, hex string) {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	copy(binary[:], b)
	return binary, toHex(b)
}

func toHex(b []byte) string {
	const hexDigits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, c := range b {
		out[i*2] = hexDigits[c>>4]
		out[i*2+1] = hexDigits[c&0xf]
	}
	return string(out)
}

// ListArsip mengambil arsip terbaru (soft-delete disaring).
func (e *LocalEngine) ListArsip(limit int) ([]Arsip, error) {
	rows, err := e.db.Query(`SELECT HEX(id), nama, IFNULL(kategori,''), status,
        IFNULL(file_size,0), created_at, updated_at
        FROM arsip WHERE deleted_at IS NULL
        ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query arsip: %w", err)
	}
	defer rows.Close()

	out := make([]Arsip, 0, limit)
	for rows.Next() {
		var a Arsip
		var created, updated time.Time
		if err := rows.Scan(&a.ID, &a.Nama, &a.Kategori, &a.Status, &a.FileSize, &created, &updated); err != nil {
			return nil, err
		}
		a.Created = created.Format(time.DateTime)
		a.Updated = updated.Format(time.DateTime)
		out = append(out, a)
	}
	return out, rows.Err()
}

// CountArsip menjumlahkan arsip aktif.
func (e *LocalEngine) CountArsip() (int64, error) {
	var n int64
	err := e.db.QueryRow(`SELECT COUNT(*) FROM arsip WHERE deleted_at IS NULL`).Scan(&n)
	return n, err
}

// CreateArsip menyimpan arsip baru lalu mengembalikan hasilnya.
func (e *LocalEngine) CreateArsip(nama, kategori string) (Arsip, error) {
	if strings.TrimSpace(nama) == "" {
		return Arsip{}, errors.New("nama arsip wajib diisi")
	}
	if kategori == "" {
		kategori = "umum"
	}
	_, idHex := NewUUID()
	rid, err := hexDecode(idHex)
	if err != nil {
		return Arsip{}, err
	}
	if _, err := e.db.Exec(`INSERT INTO arsip(id, nama, kategori, status) VALUES(?,?,?,'draft')`,
		rid, nama, kategori); err != nil {
		return Arsip{}, fmt.Errorf("insert arsip: %w", err)
	}
	return Arsip{ID: idHex, Nama: nama, Kategori: kategori, Status: "draft"}, nil
}

// DeleteArsip melakukan soft-delete berdasar hex UUID.
func (e *LocalEngine) DeleteArsip(idHex string) (bool, error) {
	rid, err := hexDecode(idHex)
	if err != nil {
		return false, err
	}
	rs, err := e.db.Exec(`UPDATE arsip SET deleted_at=NOW(3), status='deleted' WHERE id=? AND deleted_at IS NULL`, rid)
	if err != nil {
		return false, err
	}
	n, _ := rs.RowsAffected()
	return n > 0, nil
}

func hexDecode(h string) ([]byte, error) {
	clean := strings.Trim(strings.TrimSpace(h), "{}-")
	if len(clean) == 32 {
		build := make([]byte, 0, 32)
		for i, part := range []string{clean[0:8], clean[8:12], clean[12:16], clean[16:20], clean[20:32]} {
			_ = i
			build = append(build, []byte(part)...)
		}
		clean = string(build)
	}
	if len(clean) != 32 {
		return nil, fmt.Errorf("uuid tidak valid")
	}
	out := make([]byte, 16)
	for i := 0; i < 16; i++ {
		hi := hexVal(clean[i*2])
		lo := hexVal(clean[i*2+1])
		if hi < 0 || lo < 0 {
			return nil, fmt.Errorf("uuid tidak valid")
		}
		out[i] = byte(hi<<4 | lo)
	}
	return out, nil
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}
