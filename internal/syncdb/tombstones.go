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

// pkSignature returns a stable signature of a table's primary-key columns.
// It changes when the PK shape changes (e.g. a composite key becoming a UUID
// `id`), which invalidates any previous key snapshot.
func pkSignature(ti *tableInfo) string {
	return strings.Join(ti.PK, ",")
}

// loadShadowKeys returns the key snapshot stored for a table in sync_keys
// along with the PK signature it was captured under ("" when absent).
func loadShadowKeys(ctx context.Context, db *sql.DB, table string) (map[string]string, string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT key_hash, key_json, COALESCE(pk_sig,'') FROM sync_keys WHERE table_name = ?`, table)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := make(map[string]string)
	sig := ""
	for rows.Next() {
		var h, j, s string
		if err := rows.Scan(&h, &j, &s); err != nil {
			return nil, "", err
		}
		out[h] = j
		sig = s
	}
	return out, sig, rows.Err()
}

// upsertTombstone writes one tombstone using the destination dialect.
func upsertTombstone(ctx context.Context, db *sql.DB, driver, table, hash, keyJSON string, deletedAt, updatedAt time.Time) error {
	if driver == driverSQLite {
		_, err := db.ExecContext(ctx, `
			INSERT INTO sync_tombstones (table_name, key_hash, key_json, deleted_at, updated_at)
			VALUES (?,?,?,?,?)
			ON CONFLICT (table_name, key_hash)
			DO UPDATE SET deleted_at=excluded.deleted_at, updated_at=excluded.updated_at`,
			table, hash, keyJSON, deletedAt, updatedAt)
		return err
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO sync_tombstones (table_name, key_hash, key_json, deleted_at, updated_at)
		VALUES (?,?,?,?,?)
		ON DUPLICATE KEY UPDATE deleted_at=VALUES(deleted_at), updated_at=VALUES(updated_at)`,
		table, hash, keyJSON, deletedAt, updatedAt)
	return err
}

// detectDeletes compares the current primary keys against the stored snapshot
// and writes a tombstone for every key that disappeared on this node. It then
// refreshes the snapshot. Returns the number of newly detected deletions.
func detectDeletes(ctx context.Context, db *sql.DB, driver string, ti *tableInfo) (int, error) {
	cur, err := currentKeys(ctx, db, ti)
	if err != nil {
		return 0, err
	}
	prev, prevSig, err := loadShadowKeys(ctx, db, ti.Name)
	if err != nil {
		return 0, err
	}

	// Bila bentuk primary key berubah (mis. PK komposit lama menjadi UUID
	// `id`), snapshot lama TIDAK BISA dibandingkan: setiap kunci lama akan
	// tampak "hilang" sehingga menghasilkan tombstone palsu yang lalu menyebar
	// ke node lain dan menghapus baris yang masih sah. Saat itu terjadi, buang
	// snapshot lama DAN tombstone lama untuk tabel ini, lalu mulai dari nol.
	sig := pkSignature(ti)
	if len(prev) > 0 && prevSig != "" && prevSig != sig {
		if _, err := db.ExecContext(ctx, `DELETE FROM sync_keys WHERE table_name = ?`, ti.Name); err != nil {
			return 0, err
		}
		if _, err := db.ExecContext(ctx, `DELETE FROM sync_tombstones WHERE table_name = ?`, ti.Name); err != nil {
			return 0, err
		}
		prev = map[string]string{}
	}

	// Snapshot entries whose key has a different number of values than the
	// current primary key come from a different PK shape (e.g. the old
	// composite role_id+permission_id key). They must NOT become tombstones:
	// the tombstone would travel to the hub and delete rows that are still
	// valid there. Drop them and start the snapshot from the current shape.
	for h, j := range prev {
		vals, err := unmarshalKey(j)
		if err != nil || len(vals) != len(ti.PK) {
			delete(prev, h)
		}
	}

	now := time.Now()
	deleted := 0
	for h, j := range prev {
		if _, ok := cur[h]; ok {
			continue
		}
		if err := upsertTombstone(ctx, db, driver, ti.Name, h, j, now, now); err != nil {
			return 0, err
		}
		deleted++
	}

	// Refresh the snapshot to the current key set.
	if _, err := db.ExecContext(ctx, `DELETE FROM sync_keys WHERE table_name = ?`, ti.Name); err != nil {
		return 0, err
	}
	if err := refreshShadow(ctx, db, ti.Name, sig, cur); err != nil {
		return 0, err
	}
	return deleted, nil
}

// refreshShadow rewrites the key snapshot for one table using chunked multi-row
// INSERTs (a per-key insert over the pool's single connection is too slow).
func refreshShadow(ctx context.Context, db *sql.DB, table, pkSig string, keys map[string]string) error {
	const perStmt = 500 // 4 placeholders each; stays far under MySQL's 65k limit
	cols := 4
	rows := make([][]any, 0, len(keys))
	for h, j := range keys {
		rows = append(rows, []any{table, h, j, pkSig})
	}
	for start := 0; start < len(rows); start += perStmt {
		end := start + perStmt
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[start:end]
		var b strings.Builder
		b.WriteString("INSERT INTO sync_keys (table_name, key_hash, key_json, pk_sig) VALUES ")
		args := make([]any, 0, len(chunk)*cols)
		for i, r := range chunk {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString("(?,?,?,?)")
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
//
// keyCount, when greater than zero, restricts the copy to tombstones whose key
// has exactly that many values. It protects the hub from a node whose local
// schema still uses an outdated primary-key shape: those tombstones refer to
// rows the hub does not even know about.
func copyTombstones(ctx context.Context, src, dst *sql.DB, table, srcDriver, dstDriver string, keyCount int) error {
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
		if keyCount > 0 {
			vals, verr := unmarshalKey(j)
			if verr != nil || len(vals) != keyCount {
				continue
			}
		}
		_ = srcDriver
		if err := upsertTombstone(ctx, dst, dstDriver, table, h, j, deletedAt, updatedAt); err != nil {
			return err
		}
	}
	return rows.Err()
}

// purgeStaleTombstones removes tombstone rows whose key no longer matches the
// current primary-key shape of the table they belong to.
//
// Such rows are leftovers from an older schema (for example permission_role
// used to be keyed by role_id+permission_id and now uses a surrogate UUID
// `id`). Keeping them is dangerous: as soon as a node with the old shape
// applies them, valid rows are deleted — and they spread to every other node
// through the hub.
//
// Returns the number of purged rows.
func purgeStaleTombstones(ctx context.Context, db *sql.DB, driver string, tables map[string]*tableInfo) (int, error) {
	if len(tables) == 0 {
		return 0, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT table_name, key_hash, key_json FROM sync_tombstones`)
	if err != nil {
		return 0, err
	}
	type stale struct {
		table string
		hash  string
	}
	var victims []stale
	for rows.Next() {
		var table, hash, keyJSON string
		if err := rows.Scan(&table, &hash, &keyJSON); err != nil {
			rows.Close()
			return 0, err
		}
		ti, ok := tables[table]
		if !ok || len(ti.PK) == 0 {
			continue // unknown table: leave it alone
		}
		vals, verr := unmarshalKey(keyJSON)
		if verr != nil || len(vals) != len(ti.PK) {
			victims = append(victims, stale{table: table, hash: hash})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	purged := 0
	for _, v := range victims {
		if _, err := db.ExecContext(ctx,
			`DELETE FROM sync_tombstones WHERE table_name = ? AND key_hash = ?`, v.table, v.hash); err != nil {
			return purged, err
		}
		purged++
	}
	return purged, nil
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
