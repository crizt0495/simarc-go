package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// TestMigrateAutoIncToUUID proves migrateAutoIncToUUID converts the sync-bound
// tables from BIGINT AUTO_INCREMENT primary keys to CHAR(36) UUID keys,
// rewrites arsip.jenis_arsip_id to match, preserves row identity through
// deterministic UUIDs, recreates the dropped foreign key, and is idempotent.
//
// Skipped unless SIMARC_TEST_DSN_BASE is set, e.g.:
//
//	SIMARC_TEST_DSN_BASE='chris:pass@tcp(127.0.0.1:3306)/' go test ./internal/database/
func TestMigrateAutoIncToUUID(t *testing.T) {
	base := os.Getenv("SIMARC_TEST_DSN_BASE")
	if strings.TrimSpace(base) == "" {
		t.Skip("SIMARC_TEST_DSN_BASE not set; skipping live MySQL migration test")
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}

	ctx := context.Background()
	root, err := sql.Open("mysql", base)
	if err != nil {
		t.Fatalf("open server: %v", err)
	}
	defer root.Close()
	if err := root.PingContext(ctx); err != nil {
		t.Fatalf("ping server: %v", err)
	}

	dbname := fmt.Sprintf("mig_uuid_%d", time.Now().UnixNano())
	_, _ = root.ExecContext(ctx, "DROP DATABASE IF EXISTS "+dbname)
	if _, err := root.ExecContext(ctx, "CREATE DATABASE "+dbname+" CHARACTER SET utf8mb4"); err != nil {
		t.Fatalf("create scratch db: %v", err)
	}
	t.Cleanup(func() { _, _ = root.ExecContext(ctx, "DROP DATABASE IF EXISTS "+dbname) })

	gormDB, err := gorm.Open(mysql.Open(base+dbname+"?charset=utf8mb4&parseTime=true&loc=Local"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open scratch db: %v", err)
	}
	prev := DB
	DB = gormDB
	defer func() { DB = prev }()

	// Reproduce the pre-2.1 layout: BIGINT AUTO_INCREMENT keys plus the two
	// foreign keys on arsip.jenis_arsip_id that actually exist in production.
	ddls := []string{
		`CREATE TABLE jenis_arsip (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			kode_jenis VARCHAR(50) NULL,
			nama_jenis VARCHAR(255) NOT NULL,
			created_at DATETIME(3) NULL,
			updated_at DATETIME(3) NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE arsip (
			id VARCHAR(36) NOT NULL PRIMARY KEY,
			jenis_arsip_id BIGINT UNSIGNED NULL,
			nomor_arsip VARCHAR(100) NULL,
			created_at DATETIME(3) NULL,
			updated_at DATETIME(3) NULL,
			deleted_at DATETIME(3) NULL,
			CONSTRAINT arsip_jenis_arsip_id_foreign FOREIGN KEY (jenis_arsip_id) REFERENCES jenis_arsip(id),
			CONSTRAINT fk_arsip_jenis FOREIGN KEY (jenis_arsip_id) REFERENCES jenis_arsip(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE berita_acara_items (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			berita_acara_id VARCHAR(36) NOT NULL,
			arsip_id VARCHAR(36) NOT NULL,
			created_at DATETIME(3) NULL,
			INDEX idx_bi_ba (berita_acara_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE pemusnahan_arsip_items (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			pemusnahan_id VARCHAR(36) NOT NULL,
			arsip_id VARCHAR(36) NOT NULL,
			created_at DATETIME(3) NULL,
			INDEX idx_pemusnahan_items_pemusnahan (pemusnahan_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE blockchain_audits (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			uuid VARCHAR(36) NULL,
			previous_hash VARCHAR(64) NULL,
			current_hash VARCHAR(64) NULL,
			block_number BIGINT NULL,
			timestamp VARCHAR(50) NULL,
			user_id VARCHAR(36) NULL,
			action VARCHAR(100) NULL,
			entity_type VARCHAR(100) NULL,
			entity_id TEXT NULL,
			details TEXT NULL,
			ip_address VARCHAR(45) NULL,
			user_agent TEXT NULL,
			is_valid TINYINT(1) DEFAULT 1,
			created_at DATETIME(3) NULL,
			updated_at DATETIME(3) NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, ddl := range ddls {
		if err := gormDB.Exec(ddl).Error; err != nil {
			t.Fatalf("ddl: %v\n%s", err, ddl)
		}
	}

	now := time.Now().Truncate(time.Millisecond)
	mustInsert := func(q string, args ...any) {
		t.Helper()
		if err := gormDB.Exec(q, args...).Error; err != nil {
			t.Fatalf("seed insert: %v", err)
		}
	}
	mustInsert(`INSERT INTO jenis_arsip (kode_jenis,nama_jenis,created_at,updated_at) VALUES ('SPJ','Surat Perjalanan Dinas',?,?)`, now, now)
	mustInsert(`INSERT INTO jenis_arsip (kode_jenis,nama_jenis,created_at,updated_at) VALUES ('KT','Kontrak',?,?)`, now, now)
	mustInsert(`INSERT INTO jenis_arsip (kode_jenis,nama_jenis,created_at,updated_at) VALUES ('LL','Lain-lain',?,?)`, now, now)
	mustInsert(`INSERT INTO arsip (id,jenis_arsip_id,nomor_arsip,created_at,updated_at) VALUES ('aaaaaaaa-0000-0000-0000-000000000001',1,'SPM-001',?,?)`, now, now)
	mustInsert(`INSERT INTO arsip (id,jenis_arsip_id,nomor_arsip,created_at,updated_at) VALUES ('aaaaaaaa-0000-0000-0000-000000000002',NULL,'BEBAS',?,?)`, now, now)
	mustInsert(`INSERT INTO berita_acara_items (berita_acara_id,arsip_id,created_at) VALUES ('ba-1-jenis-test','aaaaaaaa-0000-0000-0000-000000000003',?)`, now)
	mustInsert(`INSERT INTO berita_acara_items (berita_acara_id,arsip_id,created_at) VALUES ('ba-1-jenis-test','aaaaaaaa-0000-0000-0000-000000000004',?)`, now)
	mustInsert(`INSERT INTO pemusnahan_arsip_items (pemusnahan_id,arsip_id,created_at) VALUES ('pm-1','aaaaaaaa-0000-0000-0000-000000000005',?)`, now)
	mustInsert(`INSERT INTO pemusnahan_arsip_items (pemusnahan_id,arsip_id,created_at) VALUES ('pm-1','aaaaaaaa-0000-0000-0000-000000000006',?)`, now)
	mustInsert(`INSERT INTO blockchain_audits (uuid,block_number,timestamp,action,entity_type,created_at,updated_at) VALUES ('bc-uuid-1',1,'2024-01-01','create','arsip',?,?)`, now, now)
	mustInsert(`INSERT INTO blockchain_audits (uuid,block_number,timestamp,action,entity_type,created_at,updated_at) VALUES ('bc-uuid-2',2,'2024-01-02','create','arsip',?,?)`, now, now)

	if err := migrateAutoIncToUUID(); err != nil {
		t.Fatalf("migrateAutoIncToUUID: %v", err)
	}

	// ── assertions ──
	want := func(oldID int64) string {
		return fmt.Sprintf("00000000-0000-0000-0000-%012x", oldID)
	}

	type colSpec struct {
		ColumnType string
		Extra      string
	}
	for _, tbl := range []string{"jenis_arsip", "berita_acara_items", "pemusnahan_arsip_items", "blockchain_audits"} {
		var cs colSpec
		if err := DB.Raw(`SELECT column_type, extra FROM information_schema.columns
			WHERE table_schema = DATABASE() AND table_name = ? AND column_name = 'id'`, tbl).Scan(&cs).Error; err != nil {
			t.Fatalf("inspect %s.id: %v", tbl, err)
		}
		if !strings.HasPrefix(cs.ColumnType, "char(36)") {
			t.Errorf("%s.id column_type = %s, want char(36)", tbl, cs.ColumnType)
		}
		if strings.Contains(cs.Extra, "auto_increment") {
			t.Errorf("%s.id still auto_increment", tbl)
		}
	}

	// Deterministic identity preservation on jenis_arsip (old ids 1,2,3).
	var ids []string
	if err := DB.Raw(`SELECT id FROM jenis_arsip ORDER BY nama_jenis`).Scan(&ids).Error; err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{want(1): true, want(2): true, want(3): true}
	for _, id := range ids {
		if !known[id] {
			t.Errorf("jenis_arsip id = %s, want one of %v", id, known)
		}
	}

	// arsip reference column rewritten (1→want(1)), NULL preserved.
	var a1, a2 sql.NullString
	if err := DB.Raw(`SELECT jenis_arsip_id FROM arsip WHERE nomor_arsip = 'SPM-001'`).Scan(&a1).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Raw(`SELECT jenis_arsip_id FROM arsip WHERE nomor_arsip = 'BEBAS'`).Scan(&a2).Error; err != nil {
		t.Fatal(err)
	}
	if a1.String != want(1) {
		t.Errorf("arsip SPM-001 jenis_arsip_id = %v, want %s", a1.String, want(1))
	}
	if a2.Valid {
		t.Errorf("arsip BEBAS jenis_arsip_id = %v, want NULL", a2.String)
	}

	// arsip FK recreated.
	var fks []string
	if err := DB.Raw(`SELECT constraint_name FROM information_schema.KEY_COLUMN_USAGE
		WHERE table_schema = DATABASE() AND table_name = 'arsip' AND column_name = 'jenis_arsip_id'
		AND referenced_table_name = 'jenis_arsip'`).Scan(&fks).Error; err != nil {
		t.Fatal(err)
	}
	if len(fks) == 0 {
		t.Error("arsip.jenis_arsip_id FK not recreated")
	}

	// Row counts preserved everywhere.
	for _, tbl := range []string{"jenis_arsip", "berita_acara_items", "pemusnahan_arsip_items", "blockchain_audits"} {
		var n int64
		if err := DB.Raw("SELECT COUNT(*) FROM " + tbl).Scan(&n).Error; err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Errorf("%s lost all rows during migration", tbl)
		}
	}

	// Idempotent: a second run must be a no-op.
	before := map[string]string{}
	for _, tbl := range []string{"jenis_arsip", "berita_acara_items", "pemusnahan_arsip_items", "blockchain_audits"} {
		var cs colSpec
		_ = DB.Raw(`SELECT column_type, extra FROM information_schema.columns
			WHERE table_schema = DATABASE() AND table_name = ? AND column_name = 'id'`, tbl).Scan(&cs).Error
		before[tbl] = cs.ColumnType + "|" + cs.Extra
	}
	if err := migrateAutoIncToUUID(); err != nil {
		t.Fatalf("second migrateAutoIncToUUID: %v", err)
	}
	for tbl, spec := range before {
		var cs colSpec
		_ = DB.Raw(`SELECT column_type, extra FROM information_schema.columns
			WHERE table_schema = DATABASE() AND table_name = ? AND column_name = 'id'`, tbl).Scan(&cs).Error
		if got := cs.ColumnType + "|" + cs.Extra; got != spec {
			t.Errorf("%s.id changed on second run: %s → %s", tbl, spec, got)
		}
	}
}
