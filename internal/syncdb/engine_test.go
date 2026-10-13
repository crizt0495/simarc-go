package syncdb

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"arsippro/internal/config"

	_ "github.com/go-sql-driver/mysql"
)

// TestConvergence proves two independent databases converge after a sync round,
// including soft-delete propagation and append-only log merging.
//
// It is skipped unless SIMARC_TEST_DSN_BASE is set, e.g.:
//
//	SIMARC_TEST_DSN_BASE='chris:pass@tcp(127.0.0.1:3306)/' go test ./internal/syncdb/
//
// The base DSN must not include a database name (it connects to the server so
// the test can create and drop its own scratch databases).
func TestConvergence(t *testing.T) {
	base := os.Getenv("SIMARC_TEST_DSN_BASE")
	if strings.TrimSpace(base) == "" {
		t.Skip("SIMARC_TEST_DSN_BASE not set; skipping live MySQL convergence test")
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

	dbs := map[string]string{
		"arsipsync_a": "a",
		"arsipsync_b": "b",
	}
	for name := range dbs {
		_, _ = root.ExecContext(ctx, "DROP DATABASE IF EXISTS "+name)
		if _, err := root.ExecContext(ctx, "CREATE DATABASE "+name+" CHARACTER SET utf8mb4"); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		t.Cleanup(func() { _, _ = root.ExecContext(ctx, "DROP DATABASE IF EXISTS "+name) })
	}

	dsn := func(db string) string {
		return base + db + "?charset=utf8mb4&parseTime=true&loc=Local"
	}
	a, b := "arsipsync_a", "arsipsync_b"

	// Create the schema only in A; the engine must create the tables in B.
	connA, err := sql.Open("mysql", dsn(a))
	if err != nil {
		t.Fatal(err)
	}
	defer connA.Close()
	for _, ddl := range []string{
		`CREATE TABLE arsip (
			id VARCHAR(36) NOT NULL PRIMARY KEY,
			uraian VARCHAR(255) NULL,
			jumlah INT NULL,
			created_at DATETIME(3) NULL,
			updated_at DATETIME(3) NULL,
			deleted_at DATETIME(3) NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE activity_logs (
			id VARCHAR(36) NOT NULL PRIMARY KEY,
			action VARCHAR(80) NULL,
			created_at DATETIME(3) NULL,
			updated_at DATETIME(3) NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE permission_role (
			id VARCHAR(36) NOT NULL PRIMARY KEY,
			role_id VARCHAR(36) NULL,
			permission_id VARCHAR(36) NULL,
			created_at DATETIME(3) NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	} {
		if _, err := connA.ExecContext(ctx, ddl); err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}

	now := time.Now().Truncate(time.Millisecond)

	// Seed A.
	mustExec(t, connA, `INSERT INTO arsip (id,uraian,jumlah,created_at,updated_at) VALUES (?,?,?,?,?)`,
		"11111111-1111-1111-1111-111111111111", "Berkas A", 1, now, now)
	mustExec(t, connA, `INSERT INTO permission_role (id,role_id,permission_id,created_at) VALUES (?,?,?,?)`,
		"aaaaaaaa-0000-0000-0000-000000000001", "role-1", "perm-1", now)

	cfgA := config.SyncConfig{NodeID: "a", BatchSize: 200, OverlapSeconds: 5}
	cfgB := config.SyncConfig{NodeID: "b", BatchSize: 200, OverlapSeconds: 5}

	// A pushes to B (and creates B's tables).
	runEngine(t, ctx, dsn(a), dsn(b), cfgA)

	// Seed B-specific rows now that B has the tables.
	connB, err := sql.Open("mysql", dsn(b))
	if err != nil {
		t.Fatal(err)
	}
	defer connB.Close()
	now2 := now.Add(time.Second)
	mustExec(t, connB, `INSERT INTO arsip (id,uraian,jumlah,created_at,updated_at) VALUES (?,?,?,?,?)`,
		"22222222-2222-2222-2222-222222222222", "Berkas B", 2, now2, now2)
	mustExec(t, connB, `INSERT INTO activity_logs (id,action,created_at,updated_at) VALUES (?,?,?,?)`,
		"bbbbbbbb-0000-0000-0000-000000000001", "login", now2, now2)

	// B pushes to A; A pulls B's rows back; one more A round settles it.
	runEngine(t, ctx, dsn(b), dsn(a), cfgB)
	runEngine(t, ctx, dsn(a), dsn(b), cfgA)

	// Both databases must contain both rows.
	for label, db := range map[string]*sql.DB{"A": connA, "B": connB} {
		if got := countRows(t, ctx, db, "arsip", "deleted_at IS NULL"); got != 2 {
			t.Errorf("%s: arsip count = %d, want 2", label, got)
		}
		if got := countRows(t, ctx, db, "activity_logs", "1=1"); got != 1 {
			t.Errorf("%s: activity_logs count = %d, want 1", label, got)
		}
		if got := countRows(t, ctx, db, "permission_role", "1=1"); got != 1 {
			t.Errorf("%s: permission_role count = %d, want 1", label, got)
		}
	}

	// Update in B, soft-delete in A; both must propagate.
	upd := now2.Add(2 * time.Second)
	mustExec(t, connB, `UPDATE arsip SET uraian=?, updated_at=? WHERE id=?`,
		"Berkas B diubah", upd, "22222222-2222-2222-2222-222222222222")
	del := now2.Add(3 * time.Second)
	mustExec(t, connA, `UPDATE arsip SET deleted_at=?, updated_at=? WHERE id=?`,
		del, del, "11111111-1111-1111-1111-111111111111")

	runEngine(t, ctx, dsn(a), dsn(b), cfgA)
	runEngine(t, ctx, dsn(b), dsn(a), cfgB)
	runEngine(t, ctx, dsn(a), dsn(b), cfgA)

	for label, db := range map[string]*sql.DB{"A": connA, "B": connB} {
		if got := countRows(t, ctx, db, "arsip", "deleted_at IS NULL"); got != 1 {
			t.Errorf("%s: live arsip count = %d, want 1 (soft delete should propagate)", label, got)
		}
		var uraian string
		if err := db.QueryRowContext(ctx, `SELECT uraian FROM arsip WHERE id=?`, "22222222-2222-2222-2222-222222222222").Scan(&uraian); err != nil {
			t.Fatalf("%s: read updated row: %v", label, err)
		}
		if uraian != "Berkas B diubah" {
			t.Errorf("%s: uraian = %q, want %q (update should propagate)", label, uraian, "Berkas B diubah")
		}
	}

	// Rows written by legacy importers often have created_at but NULL
	// updated_at; those must still travel (COALESCE fallback in readDelta).
	mustExec(t, connA, `INSERT INTO activity_logs (id,action,created_at,updated_at) VALUES (?,?,?,NULL)`,
		"cccccccc-0000-0000-0000-000000000001", "legacy-a", now2)

	runEngine(t, ctx, dsn(a), dsn(b), cfgA)
	runEngine(t, ctx, dsn(b), dsn(a), cfgB)
	runEngine(t, ctx, dsn(a), dsn(b), cfgA)

	for label, db := range map[string]*sql.DB{"A": connA, "B": connB} {
		if got := countRows(t, ctx, db, "activity_logs", "id='cccccccc-0000-0000-0000-000000000001'"); got != 1 {
			t.Errorf("%s: NULL-updated_at row count = %d, want 1 (created_at fallback)", label, got)
		}
	}

	// ── Keyed table (hard delete): revoke the grant on A, must vanish on B ──
	mustExec(t, connA, `DELETE FROM permission_role WHERE id=?`,
		"aaaaaaaa-0000-0000-0000-000000000001")

	// A detects the deletion and tombstones it; B must apply it too. Run a few
	// rounds so tombstones flow A→hub→B and back.
	runEngine(t, ctx, dsn(a), dsn(b), cfgA)
	runEngine(t, ctx, dsn(b), dsn(a), cfgB)
	runEngine(t, ctx, dsn(a), dsn(b), cfgA)

	for label, db := range map[string]*sql.DB{"A": connA, "B": connB} {
		if got := countRows(t, ctx, db, "permission_role", "1=1"); got != 0 {
			t.Errorf("%s: permission_role count = %d, want 0 (revoked grant must propagate)", label, got)
		}
	}

	// Re-granting on A after the tombstone must survive on both nodes. The
	// tombstone is stamped with the wall clock at deletion-detection time, so a
	// small fixed offset becomes fragile under heavy parallel load (wall clock
	// may pass it before the re-insert). Use a generous future offset: survival
	// only requires the re-created row to be newer than the tombstone.
	grantTime := now.Add(2 * time.Hour)
	mustExec(t, connA, `INSERT INTO permission_role (id,role_id,permission_id,created_at) VALUES (?,?,?,?)`,
		"aaaaaaaa-0000-0000-0000-000000000001", "role-1", "perm-1", grantTime)

	runEngine(t, ctx, dsn(a), dsn(b), cfgA)
	runEngine(t, ctx, dsn(b), dsn(a), cfgB)
	runEngine(t, ctx, dsn(a), dsn(b), cfgA)

	for label, db := range map[string]*sql.DB{"A": connA, "B": connB} {
		if got := countRows(t, ctx, db, "permission_role", "1=1"); got != 1 {
			t.Errorf("%s: permission_role count = %d, want 1 (re-grant newer than tombstone survives)", label, got)
		}
	}
}

func runEngine(t *testing.T, ctx context.Context, localDSN, hubDSN string, cfg config.SyncConfig) {
	t.Helper()
	eng, err := OpenWithDSN(ctx, localDSN, hubDSN, cfg, func(string, ...any) {})
	if err != nil {
		t.Fatalf("open engine: %v", err)
	}
	defer eng.Close()
	res, err := eng.Run(ctx)
	if err != nil {
		t.Fatalf("run engine: %v", err)
	}
	for _, tr := range res.Tables {
		if tr.Err != "" {
			t.Fatalf("table %s failed: %s", tr.Name, tr.Err)
		}
	}
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), q, args...); err != nil {
		t.Fatalf("exec: %v", err)
	}
}

func countRows(t *testing.T, ctx context.Context, db *sql.DB, table, where string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"` WHERE "+where).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}
