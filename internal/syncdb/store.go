package syncdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// ensureLocalSyncTables creates the bookkeeping tables on the local database.
// They are created lazily (not part of the main schema migration) so enabling
// sync needs no schema-version bump.
func ensureLocalSyncTables(ctx context.Context, db *sql.DB, driver string) error {
	if driver == driverSQLite {
		stmts := []string{
			`CREATE TABLE IF NOT EXISTS sync_state (
				node_id TEXT NOT NULL,
				table_name TEXT NOT NULL,
				direction TEXT NOT NULL,
				watermark DATETIME NULL,
				updated_at DATETIME NULL,
				PRIMARY KEY (node_id, table_name, direction)
			)`,
			`CREATE TABLE IF NOT EXISTS sync_runs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				node_id TEXT NOT NULL,
				started_at DATETIME NOT NULL,
				finished_at DATETIME NOT NULL,
				status TEXT NOT NULL,
				pushed INTEGER NOT NULL DEFAULT 0,
				pulled INTEGER NOT NULL DEFAULT 0,
				tables_ok INTEGER NOT NULL DEFAULT 0,
				tables_failed INTEGER NOT NULL DEFAULT 0,
				detail TEXT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS idx_sync_runs_started ON sync_runs (started_at)`,
			`CREATE TABLE IF NOT EXISTS sync_keys (
				table_name TEXT NOT NULL,
				key_hash TEXT NOT NULL,
				key_json TEXT NOT NULL,
				pk_sig TEXT NOT NULL DEFAULT '',
				PRIMARY KEY (table_name, key_hash)
			)`,
			`CREATE TABLE IF NOT EXISTS sync_tombstones (
				table_name TEXT NOT NULL,
				key_hash TEXT NOT NULL,
				key_json TEXT NOT NULL,
				deleted_at DATETIME NOT NULL,
				updated_at DATETIME NOT NULL,
				PRIMARY KEY (table_name, key_hash)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_sync_tomb_updated ON sync_tombstones (updated_at)`,
		}
		for _, q := range stmts {
			if _, err := db.ExecContext(ctx, q); err != nil {
				return err
			}
		}
		// Basis data lama (sebelum pk_sig ada) perlu kolom ini; SQLite tidak
		// punya "ADD COLUMN IF NOT EXISTS", jadi galat "duplicate column"
		// diabaikan.
		if _, aerr := db.ExecContext(ctx,
			`ALTER TABLE sync_keys ADD COLUMN pk_sig TEXT NOT NULL DEFAULT ''`); aerr != nil &&
			!strings.Contains(strings.ToLower(aerr.Error()), "duplicate") {
			_ = aerr
		}
		return nil
	}

	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS sync_state (
		node_id VARCHAR(64) NOT NULL,
		table_name VARCHAR(64) NOT NULL,
		direction VARCHAR(8) NOT NULL,
		watermark DATETIME(3) NULL,
		updated_at DATETIME(3) NULL,
		PRIMARY KEY (node_id, table_name, direction)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS sync_runs (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
		node_id VARCHAR(64) NOT NULL,
		started_at DATETIME(3) NOT NULL,
		finished_at DATETIME(3) NOT NULL,
		status VARCHAR(20) NOT NULL,
		pushed INT NOT NULL DEFAULT 0,
		pulled INT NOT NULL DEFAULT 0,
		tables_ok INT NOT NULL DEFAULT 0,
		tables_failed INT NOT NULL DEFAULT 0,
		detail LONGTEXT NULL,
		INDEX idx_sync_runs_started (started_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`)
	if err != nil {
		return err
	}

	// sync_keys is the local-only snapshot of keyed-table primary keys from the
	// previous run. It is how the engine detects hard deletes (a key that was
	// present before and is now gone). It is never synchronised.
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS sync_keys (
		table_name VARCHAR(64) NOT NULL,
		key_hash CHAR(64) NOT NULL,
		key_json VARCHAR(1024) NOT NULL,
		pk_sig VARCHAR(255) NOT NULL DEFAULT '',
		PRIMARY KEY (table_name, key_hash)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`)
	if err != nil {
		return err
	}
	// Tabel lama (sebelum pk_sig ada) perlu kolom ini; MySQL tidak punya
	// "ADD COLUMN IF NOT EXISTS", jadi galat "duplicate column" diabaikan.
	if _, aerr := db.ExecContext(ctx,
		`ALTER TABLE sync_keys ADD COLUMN pk_sig VARCHAR(255) NOT NULL DEFAULT ''`); aerr != nil &&
		!strings.Contains(strings.ToLower(aerr.Error()), "duplicate") {
		// Bukan galat fatal: sinkron tetap jalan tanpa proteksi bentuk PK.
		// Tetap lanjut agar tidak memblokir aplikasi.
		_ = aerr
	}

	// sync_tombstones records hard deletions of keyed-table rows. Unlike
	// sync_keys it IS synchronised between nodes, so a revocation made on one
	// computer deletes the same row on the others.
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS sync_tombstones (
		table_name VARCHAR(64) NOT NULL,
		key_hash CHAR(64) NOT NULL,
		key_json VARCHAR(1024) NOT NULL,
		deleted_at DATETIME(3) NOT NULL,
		updated_at DATETIME(3) NOT NULL,
		PRIMARY KEY (table_name, key_hash),
		INDEX idx_sync_tomb_updated (updated_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`)
	return err
}

// getWatermark returns the stored watermark for (node, table, direction).
// A missing row yields the zero time, which means "copy everything".
func getWatermark(ctx context.Context, db *sql.DB, node, table, dir string) (time.Time, error) {
	var raw any
	err := db.QueryRowContext(ctx,
		`SELECT watermark FROM sync_state WHERE node_id=? AND table_name=? AND direction=?`,
		node, table, dir).Scan(&raw)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	if raw == nil {
		return time.Time{}, nil
	}
	if t, ok := asTime(raw); ok {
		return t, nil
	}
	return time.Time{}, nil
}

// setWatermark upserts the watermark for (node, table, direction).
func setWatermark(ctx context.Context, db *sql.DB, driver, node, table, dir string, wm time.Time) error {
	if wm.IsZero() {
		return nil
	}
	var err error
	if driver == driverSQLite {
		_, err = db.ExecContext(ctx, `
			INSERT INTO sync_state (node_id, table_name, direction, watermark, updated_at)
			VALUES (?,?,?,?,?)
			ON CONFLICT (node_id, table_name, direction)
			DO UPDATE SET watermark=excluded.watermark, updated_at=excluded.updated_at`,
			node, table, dir, wm, time.Now())
	} else {
		_, err = db.ExecContext(ctx, `
			INSERT INTO sync_state (node_id, table_name, direction, watermark, updated_at)
			VALUES (?,?,?,?,?)
			ON DUPLICATE KEY UPDATE watermark=VALUES(watermark), updated_at=VALUES(updated_at)`,
			node, table, dir, wm, time.Now())
	}
	return err
}

// recordRun stores a run summary and trims history to the newest 50 rows.
func recordRun(ctx context.Context, db *sql.DB, driver string, res Result) error {
	detail, _ := json.Marshal(res.Tables)
	ok, failed := 0, 0
	for _, t := range res.Tables {
		if t.Err == "" {
			ok++
		} else {
			failed++
		}
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO sync_runs (node_id, started_at, finished_at, status, pushed, pulled, tables_ok, tables_failed, detail)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		res.NodeID, res.StartedAt, res.FinishedAt, res.Status(),
		res.Pushed, res.Pulled, ok, failed, string(detail)); err != nil {
		return err
	}
	if driver == driverSQLite {
		_, _ = db.ExecContext(ctx, `DELETE FROM sync_runs WHERE id NOT IN (
			SELECT id FROM sync_runs ORDER BY id DESC LIMIT 50
		)`)
	} else {
		_, _ = db.ExecContext(ctx, `DELETE FROM sync_runs WHERE id NOT IN (
			SELECT id FROM (SELECT id FROM sync_runs ORDER BY id DESC LIMIT 50) keep
		)`)
	}
	return nil
}

// RunSummary is a lightweight view of a sync run for the status endpoint.
type RunSummary struct {
	NodeID     string        `json:"node_id"`
	StartedAt  time.Time     `json:"started_at"`
	FinishedAt time.Time     `json:"finished_at"`
	Status     string        `json:"status"`
	Pushed     int           `json:"pushed"`
	Pulled     int           `json:"pulled"`
	TablesOK   int           `json:"tables_ok"`
	TablesFail int           `json:"tables_failed"`
	Tables     []TableResult `json:"tables,omitempty"`
}

// RecentRuns returns the newest runs (most recent first) from the local database.
func RecentRuns(ctx context.Context, db *sql.DB, limit int) ([]RunSummary, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := db.QueryContext(ctx, `
		SELECT node_id, started_at, finished_at, status, pushed, pulled, tables_ok, tables_failed, detail
		FROM sync_runs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RunSummary
	for rows.Next() {
		var r RunSummary
		var detail sql.NullString
		var started, finished any
		if err := rows.Scan(&r.NodeID, &started, &finished, &r.Status,
			&r.Pushed, &r.Pulled, &r.TablesOK, &r.TablesFail, &detail); err != nil {
			return nil, err
		}
		if t, ok := asTime(started); ok {
			r.StartedAt = t
		}
		if t, ok := asTime(finished); ok {
			r.FinishedAt = t
		}
		if detail.Valid && detail.String != "" {
			_ = json.Unmarshal([]byte(detail.String), &r.Tables)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
