package syncdb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Hard (permanent) deletes on keyed tables (no deleted_at column) cannot travel
// through updated_at deltas, so the engine tracks them explicitly:
//
//  1. After every sync of a keyed table, this node stores a snapshot of the
//     table's primary keys in sync_keys (local only).
//  2. On the next run, keys that existed before but are gone now were deleted
//     on this computer → a tombstone row is written to sync_tombstones with the
//     deletion time.
//  3. Tombstones are shared between nodes (like any synced table) and applied
//     with INSERT/UPDATE on the row's created/updated timestamp so a row that
//     was re-created later wins over an old tombstone.

// pkValueStrings converts scanned primary-key values into stable strings.
func pkValueStrings(vals []any) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		switch x := v.(type) {
		case nil:
			out[i] = ""
		case []byte:
			out[i] = string(x)
		case string:
			out[i] = x
		case time.Time:
			out[i] = x.Format("2006-01-02 15:04:05.999999")
		default:
			out[i] = fmt.Sprint(x)
		}
	}
	return out
}

// hashKey returns the stable key identifier for a (table, pk values) pair.
func hashKey(table string, values []string) string {
	h := sha256.New()
	h.Write([]byte(table))
	h.Write([]byte{0})
	for _, v := range values {
		h.Write([]byte(v))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func marshalKey(values []string) (string, error) {
	b, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalKey(s string) ([]string, error) {
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// currentKeys reads all primary keys of a table as key_hash → key_json.
func currentKeys(ctx context.Context, db *sql.DB, ti *tableInfo) (map[string]string, error) {
	cols := make([]string, len(ti.PK))
	for i, c := range ti.PK {
		cols[i] = "`" + c + "`"
	}
	q := "SELECT " + strings.Join(cols, ", ") + " FROM `" + ti.Name + "`"
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		vals := make([]any, len(ti.PK))
		ptrs := make([]any, len(ti.PK))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		strs := pkValueStrings(vals)
		j, err := marshalKey(strs)
		if err != nil {
			return nil, err
		}
		out[hashKey(ti.Name, strs)] = j
	}
	return out, rows.Err()
}

// loadShadowKeys returns the key snapshot stored for a table in sync_keys.
func loadShadowKeys(ctx context.Context, db *sql.DB, table string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT key_hash, key_json FROM sync_keys WHERE table_name = ?`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var h, j string
		if err := rows.Scan(&h, &j); err != nil {
			return nil, err
		}
		out[h] = j
	}
	return out, rows.Err()
}

// detectDeletes compares the current primary keys against the stored snapshot
// and writes a tombstone for every key that disappeared on this node. It then
// refreshes the snapshot. Returns the number of newly detected deletions.
func detectDeletes(ctx context.Context, db *sql.DB, ti *tableInfo) (int, error) {
	cur, err := currentKeys(ctx, db, ti)
	if err != nil {
		return 0, err
	}
	prev, err := loadShadowKeys(ctx, db, ti.Name)
	if err != nil {
		return 0, err
	}

	now := time.Now()
	deleted := 0
	for h, j := range prev {
		if _, ok := cur[h]; ok {
			continue
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO sync_tombstones (table_name, key_hash, key_json, deleted_at, updated_at)
			VALUES (?,?,?,?,?)
			ON DUPLICATE KEY UPDATE deleted_at=VALUES(deleted_at), updated_at=VALUES(updated_at)`,
			ti.Name, h, j, now, now); err != nil {
			return 0, err
		}
		deleted++
	}

	// Refresh the snapshot to the current key set.
	if _, err := db.ExecContext(ctx, `DELETE FROM sync_keys WHERE table_name = ?`, ti.Name); err != nil {
		return 0, err
	}
	if err := refreshShadow(ctx, db, ti.Name, cur); err != nil {
		return 0, err
	}
	return deleted, nil
}

// refreshShadow rewrites the key snapshot for one table using chunked multi-row
// INSERTs (a per-key insert over the pool's single connection is too slow).
func refreshShadow(ctx context.Context, db *sql.DB, table string, keys map[string]string) error {
	const perStmt = 500 // 3 placeholders each; stays far under MySQL's 65k limit
	cols := 3
	rows := make([][]any, 0, len(keys))
	for h, j := range keys {
		rows = append(rows, []any{table, h, j})
	}
	for start := 0; start < len(rows); start += perStmt {
		end := start + perStmt
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[start:end]
		var b strings.Builder
		b.WriteString("INSERT INTO sync_keys (table_name, key_hash, key_json) VALUES ")
		args := make([]any, 0, len(chunk)*cols)
		for i, r := range chunk {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString("(?,?,?)")
			args = append(args, r...)
		}
		if _, err := db.ExecContext(ctx, b.String(), args...); err != nil {
			return err
		}
	}
	return nil
}

// copyTombstones synchronises locally known tombstones for one table to a
// remote database (used to send this run's fresh deletions to the hub).
func copyTombstones(ctx context.Context, src, dst *sql.DB, table string) error {
	rows, err := src.QueryContext(ctx,
		`SELECT key_hash, key_json, deleted_at, updated_at
		 FROM sync_tombstones WHERE table_name = ?`, table)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var h, j string
		var deletedAt, updatedAt time.Time
		if err := rows.Scan(&h, &j, &deletedAt, &updatedAt); err != nil {
			return err
		}
		if _, err := dst.ExecContext(ctx, `
			INSERT INTO sync_tombstones (table_name, key_hash, key_json, deleted_at, updated_at)
			VALUES (?,?,?,?,?)
			ON DUPLICATE KEY UPDATE deleted_at=VALUES(deleted_at), updated_at=VALUES(updated_at)`,
			table, h, j, deletedAt, updatedAt); err != nil {
			return err
		}
	}
	return rows.Err()
}

// applyTombstones deletes rows of a keyed table that have a tombstone, unless
// the row was (re)created after the tombstone. Returns rows deleted.
func applyTombstones(ctx context.Context, db *sql.DB, ti *tableInfo) (int, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT key_json, deleted_at FROM sync_tombstones WHERE table_name = ?`, ti.Name)
	if err != nil {
		return 0, err
	}
	// Read everything first: the pool has a single connection, so executing the
	// DELETEs below while this cursor is still open would deadlock.
	type tomb struct {
		vals      []string
		deletedAt time.Time
		hasTime   bool
	}
	var tombs []tomb
	for rows.Next() {
		var j string
		var deletedAt any
		if err := rows.Scan(&j, &deletedAt); err != nil {
			rows.Close()
			return 0, err
		}
		vals, err := unmarshalKey(j)
		if err != nil || len(vals) != len(ti.PK) {
			continue
		}
		hasTime := false
		var t time.Time
		if tsTmp, ok := asTime(deletedAt); ok {
			t, hasTime = tsTmp, true
		}
		tombs = append(tombs, tomb{vals: vals, deletedAt: t, hasTime: hasTime})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	ts := ti.timestampColumn() // updated_at, then created_at; "" if none
	deleted := 0
	for _, tb := range tombs {
		var b strings.Builder
		b.WriteString("DELETE FROM `" + ti.Name + "` WHERE ")
		args := make([]any, 0, len(ti.PK)+1)
		for i, c := range ti.PK {
			b.WriteString("`" + c + "` = ? AND ")
			args = append(args, tb.vals[i])
		}
		if ts != "" && tb.hasTime {
			b.WriteString("(`" + ts + "` IS NULL OR `" + ts + "` < ?)")
			args = append(args, tb.deletedAt)
		} else {
			b.WriteString("1=1")
		}
		res, err := db.ExecContext(ctx, b.String(), args...)
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			deleted++
		}
	}
	return deleted, nil
}
