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

// TestMigrateAutoIncToUUIDResumesInterrupted replicates the exact interrupted
// state the hub database was left in when an ALTER exceeded the application's
// socket timeout: some tables already converted (CHAR(36) keys, arsip
// reference rewritten) while one table is mid-conversion — its id is now plain
// BIGINT (auto_increment dropped), still the PRIMARY KEY, with a fully-filled
// new_id staging column. A re-run must finish the job and leave no debris.
func TestMigrateAutoIncToUUIDResumesInterrupted(t *testing.T) {
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

	dbname := fmt.Sprintf("mig_resume_%d", time.Now().UnixNano())
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

	// Tables already converted (as on the hub after the interrupted run).
	done := []string{
		`CREATE TABLE jenis_arsip (
			id CHAR(36) NOT NULL PRIMARY KEY,
			kode_jenis VARCHAR(50) NULL,
			created_at DATETIME(3) NULL,
			updated_at DATETIME(3) NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE arsip (
			id VARCHAR(36) NOT NULL PRIMARY KEY,
			jenis_arsip_id CHAR(36) NULL,
			created_at DATETIME(3) NULL,
			updated_at DATETIME(3) NULL,
			deleted_at DATETIME(3) NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE berita_acara_items (
			id CHAR(36) NOT NULL PRIMARY KEY,
			created_at DATETIME(3) NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE pemusnahan_arsip_items (
			id CHAR(36) NOT NULL PRIMARY KEY,
			created_at DATETIME(3) NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, ddl := range done {
		if err := gormDB.Exec(ddl).Error; err != nil {
			t.Fatalf("ddl: %v\n%s", err, ddl)
		}
	}

	// The interrupted table: AUTO_INCREMENT already dropped by a server-side
	// success the client never saw; id is plain BIGINT PRIMARY KEY and the
	// new_id staging column is fully populated.
	interrupted := `CREATE TABLE blockchain_audits (
		id BIGINT NOT NULL,
		new_id CHAR(36) NULL,
		block_number BIGINT NULL,
		timestamp VARCHAR(50) NULL,
		created_at DATETIME(3) NULL,
		updated_at DATETIME(3) NULL,
		PRIMARY KEY (id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`
	if err := gormDB.Exec(interrupted).Error; err != nil {
		t.Fatalf("ddl interrupted: %v", err)
	}
	for _, pair := range []struct {
		old  int64
		uuid string
	}{
		{1, "00000000-0000-0000-0000-000000000001"},
		{2, "00000000-0000-0000-0000-000000000002"},
	} {
		if err := gormDB.Exec(`INSERT INTO blockchain_audits (id,new_id,block_number,timestamp,created_at,updated_at) VALUES (?,?,?,?,?,?)`,
			pair.old, pair.uuid, pair.old, "t"+fmt.Sprint(pair.old), time.Now(), time.Now()).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	if err := migrateAutoIncToUUID(); err != nil {
		t.Fatalf("resume migrate: %v", err)
	}

	var spec struct {
		ColumnType string
		Extra      string
	}
	if err := DB.Raw(`SELECT column_type, extra FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = 'blockchain_audits' AND column_name = 'id'`).Scan(&spec).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(spec.ColumnType, "char(36)") || strings.Contains(spec.Extra, "auto_increment") {
		t.Errorf("blockchain_audits.id after resume = (%s, extra=%q), want char(36) non-auto", spec.ColumnType, spec.Extra)
	}
	if columnExists("blockchain_audits", "new_id") {
		t.Error("blockchain_audits.new_id left behind after resume")
	}

	var id1, id2 string
	if err := DB.Raw(`SELECT id FROM blockchain_audits WHERE block_number = 1`).Scan(&id1).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Raw(`SELECT id FROM blockchain_audits WHERE block_number = 2`).Scan(&id2).Error; err != nil {
		t.Fatal(err)
	}
	if id1 != "00000000-0000-0000-0000-000000000001" || id2 != "00000000-0000-0000-0000-000000000002" {
		t.Errorf("blockchain ids = (%q,%q), want preserved deterministic uuids", id1, id2)
	}
	var n int64
	if err := DB.Raw("SELECT COUNT(*) FROM blockchain_audits").Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("blockchain_audits rows = %d, want 2", n)
	}

	// The already-converted tables must be untouched and a further run a no-op.
	if err := migrateAutoIncToUUID(); err != nil {
		t.Fatalf("post-resume no-op run: %v", err)
	}
}
