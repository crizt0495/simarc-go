package syncdb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// driverMySQL / driverSQLite menandai dialek mesin database. Database lokal
// bisa MySQL (instalasi lama / server) atau SQLite (desktop offline-first),
// sedangkan hub selalu MySQL.
const (
	driverMySQL  = "mysql"
	driverSQLite = "sqlite"
)

// strategy is how a table is merged between nodes.
type strategy int

const (
	// stratSoft: table has deleted_at (soft delete) and updated_at. Changes are
	// read with updated_at/deleted_at timestamps and merged with
	// last-write-wins. Deletions propagate because the tombstone row itself
	// carries a fresh timestamp.
	stratSoft strategy = iota
	// stratKeyed: table has no deleted_at (hard deletes, configuration, pivot).
	// Rows are upserted by primary key without deletion propagation (v1
	// limitation — see docs/OFFLINE-SYNC.md).
	stratKeyed
	// stratAppend: immutable log table. Rows are inserted once and never
	// overwritten or deleted.
	stratAppend
)

// skipTables are never synchronised: infrastructure, caches, job queues, debug
// tooling and the legacy duplicate plural tables. These either hold node-local
// state or would collide/duplicate across nodes.
var skipTables = map[string]bool{
	"migrations":         true,
	"schema_migrations":  true,
	"jobs":               true,
	"failed_jobs":        true,
	"import_export_jobs": true,

	// Sync bookkeeping: state/runs/keys are local-only. sync_tombstones is NOT
	// skipped — it is deliberately shared so deletes propagate.
	"sync_state": true,
	"sync_runs":  true,
	"sync_keys":  true,

	"telescope_entries":      true,
	"telescope_entries_tags": true,
	"telescope_monitoring":   true,

	"ai_classification_cache":  true,
	"ai_classification_caches": true,
	"analytics_cache":          true,
	"ocr_temp_images":          true,

	// Legacy duplicate (plural) tables kept for backward compatibility only.
	"arsips":                true,
	"kode_klasifikasis":     true,
	"unit_kerjas":           true,
	"jenis_arsips":          true,
	"jadwal_retensis":       true,
	"jadwal_retensi_arsips": true,
	"pemberkasans":          true,
	"pemusnahan_arsips":     true,
	"permission_roles":      true,
}

// appendTables are immutable, append-only logs. They are inserted once and
// never overwritten or deleted, so audit history from every node is preserved.
var appendTables = map[string]bool{
	"activity_logs":    true,
	"audit_logs":       true,
	"login_logs":       true,
	"integration_logs": true,
	"ocr_logs":         true,
	"qr_scan_logs":     true,
	"backup_logs":      true,
}

// tableInfo describes one table's shape for the generic merge engine.
type tableInfo struct {
	Name          string
	Columns       []string // insertable columns, in order
	PK            []string
	HasCreatedAt  bool
	HasUpdatedAt  bool
	HasDeletedAt  bool
	AutoIncrement bool
}

// timestampColumn returns the column used for change detection. updated_at is
// preferred (it moves on soft deletes and edits); created_at is used for
// append-only tables without updated_at.
func (t *tableInfo) timestampColumn() string {
	if t.HasUpdatedAt {
		return "updated_at"
	}
	if t.HasCreatedAt {
		return "created_at"
	}
	return ""
}

// deltaExpr returns the SQL pieces used by readDelta: an expression that
// resolves a row's change time (falling back to created_at when updated_at is
// NULL, which legacy data often has), and a test that matches rows where every
// timestamp column is NULL. The latter have no usable change time, so they are
// always included (the upsert is idempotent).
func deltaExpr(t *tableInfo) (coalesce, allNull string) {
	switch {
	case t.HasUpdatedAt && t.HasCreatedAt:
		return "COALESCE(`updated_at`, `created_at`)",
			"(`updated_at` IS NULL AND `created_at` IS NULL)"
	case t.HasUpdatedAt:
		return "`updated_at`", "(`updated_at` IS NULL)"
	case t.HasCreatedAt:
		return "`created_at`", "(`created_at` IS NULL)"
	default:
		return "1=1", ""
	}
}

// strategyFor decides how (and whether) a table participates in sync.
func strategyFor(t *tableInfo) (strategy, bool) {
	if skipTables[t.Name] {
		return 0, false
	}
	if len(t.PK) == 0 {
		return 0, false // cannot merge reliably without a primary key
	}
	if t.AutoIncrement {
		// Auto-increment keys collide across independent computers, which would
		// silently overwrite unrelated rows. Excluded until those tables use
		// UUID keys (documented in docs/OFFLINE-SYNC.md).
		return 0, false
	}
	if appendTables[t.Name] {
		return stratAppend, true
	}
	if t.HasDeletedAt {
		return stratSoft, true
	}
	return stratKeyed, true
}

// loadTables introspects every base table in the given database.
func loadTables(ctx context.Context, db *sql.DB, driver string) (map[string]*tableInfo, error) {
	if driver == driverSQLite {
		return loadTablesSQLite(ctx, db)
	}

	var schema string
	if err := db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&schema); err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT table_name FROM information_schema.tables
		WHERE table_schema = ? AND table_type = 'BASE TABLE'
		ORDER BY table_name`, schema)
	if err != nil {
		return nil, err
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return nil, err
		}
		names = append(names, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make(map[string]*tableInfo, len(names))
	for _, name := range names {
		ti, err := loadTable(ctx, db, schema, name)
		if err != nil {
			return nil, err
		}
		out[name] = ti
	}
	return out, nil
}

// loadTablesSQLite membaca daftar tabel dan kolom dari katalog SQLite.
func loadTablesSQLite(ctx context.Context, db *sql.DB) (map[string]*tableInfo, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return nil, err
		}
		names = append(names, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make(map[string]*tableInfo, len(names))
	for _, name := range names {
		ti, err := loadTableSQLite(ctx, db, name)
		if err != nil {
			return nil, fmt.Errorf("tabel %s: %w", name, err)
		}
		out[name] = ti
	}
	return out, nil
}

// loadTableSQLite membaca satu tabel lewat PRAGMA table_info.
func loadTableSQLite(ctx context.Context, db *sql.DB, name string) (*tableInfo, error) {
	ti := &tableInfo{Name: name}

	rows, err := db.QueryContext(ctx, "PRAGMA table_info(`"+name+"`)")
	if err != nil {
		return nil, err
	}
	type pkCol struct {
		pos int
		col string
	}
	var pks []pkCol
	for rows.Next() {
		var cid, notNull, pk int
		var col, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &col, &typ, &notNull, &dflt, &pk); err != nil {
			rows.Close()
			return nil, err
		}
		ti.Columns = append(ti.Columns, col)
		if pk > 0 {
			pks = append(pks, pkCol{pos: pk, col: col})
		}
		switch col {
		case "created_at":
			ti.HasCreatedAt = true
		case "updated_at":
			ti.HasUpdatedAt = true
		case "deleted_at":
			ti.HasDeletedAt = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Urutan kolom primer mengikuti posisi pk pada PRAGMA.
	for i := 1; i <= len(pks); i++ {
		for _, p := range pks {
			if p.pos == i {
				ti.PK = append(ti.PK, p.col)
			}
		}
	}

	var ddl string
	if err := db.QueryRowContext(ctx,
		`SELECT COALESCE(sql,'') FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&ddl); err == nil {
		if strings.Contains(strings.ToUpper(ddl), "AUTOINCREMENT") {
			ti.AutoIncrement = true
		}
	}

	if len(ti.Columns) == 0 {
		return nil, fmt.Errorf("tabel %s tidak punya kolom yang bisa disalin", name)
	}
	return ti, nil
}

// loadTable introspects a single table (MySQL).
func loadTable(ctx context.Context, db *sql.DB, schema, name string) (*tableInfo, error) {
	ti := &tableInfo{Name: name}

	rows, err := db.QueryContext(ctx, `
		SELECT column_name, extra FROM information_schema.columns
		WHERE table_schema = ? AND table_name = ?
		ORDER BY ordinal_position`, schema, name)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var col, extra string
		if err := rows.Scan(&col, &extra); err != nil {
			rows.Close()
			return nil, err
		}
		lower := strings.ToLower(extra)
		if strings.Contains(lower, "generated") {
			continue // generated columns cannot be inserted explicitly
		}
		ti.Columns = append(ti.Columns, col)
		if strings.Contains(lower, "auto_increment") {
			ti.AutoIncrement = true
		}
		switch col {
		case "created_at":
			ti.HasCreatedAt = true
		case "updated_at":
			ti.HasUpdatedAt = true
		case "deleted_at":
			ti.HasDeletedAt = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	pkRows, err := db.QueryContext(ctx, `
		SELECT column_name FROM information_schema.statistics
		WHERE table_schema = ? AND table_name = ? AND index_name = 'PRIMARY'
		ORDER BY seq_in_index`, schema, name)
	if err != nil {
		return nil, err
	}
	for pkRows.Next() {
		var col string
		if err := pkRows.Scan(&col); err != nil {
			pkRows.Close()
			return nil, err
		}
		ti.PK = append(ti.PK, col)
	}
	pkRows.Close()
	if err := pkRows.Err(); err != nil {
		return nil, err
	}

	if len(ti.Columns) == 0 {
		return nil, fmt.Errorf("tabel %s tidak punya kolom yang bisa disalin", name)
	}
	return ti, nil
}

// tableExistsIn reports whether a table exists in the given database.
func tableExistsIn(ctx context.Context, db *sql.DB, driver, table string) (bool, error) {
	var n int64
	if driver == driverSQLite {
		err := db.QueryRowContext(ctx,
			`SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n)
		if err != nil {
			return false, err
		}
		return n > 0, nil
	}
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = ?`, table).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// createTableLike copies a table's DDL from src to dst when dst lacks it.
// Hanya dipakai untuk hub MySQL; database lokal SQLite tidak bisa menjadi
// sumber DDL MySQL.
func createTableLike(ctx context.Context, src, dst *sql.DB, table string) error {
	var name, ddl string
	if err := src.QueryRowContext(ctx, "SHOW CREATE TABLE `"+table+"`").Scan(&name, &ddl); err != nil {
		return err
	}
	ddl = strings.Replace(ddl, "CREATE TABLE", "CREATE TABLE IF NOT EXISTS", 1)
	_, err := dst.ExecContext(ctx, ddl)
	return err
}
