package syncdb

import (
	"context"
	"path/filepath"
	"testing"
)

// TestDetectDeletesResetsOnPKChange memastikan bahwa ketika bentuk primary key
// sebuah tabel berubah (mis. PK komposit `role_id + permission_id` menjadi UUID
// `id`), snapshot kunci lama TIDAK dianggap sebagai penghapusan. Tanpa proteksi
// ini, seluruh kunci lama tampak "hilang" lalu menghasilkan tombstone palsu yang
// menyebar ke node lain dan menghapus baris yang masih sah (regresi izin).
func TestDetectDeletesResetsOnPKChange(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db := openTestSQLite(t, filepath.Join(dir, "node.db"))
	if err := ensureLocalSyncTables(ctx, db, driverSQLite); err != nil {
		t.Fatalf("ensureLocalSyncTables: %v", err)
	}

	mustExec(t, db, `CREATE TABLE izin (
		id text PRIMARY KEY,
		role_id text,
		permission_id text
	)`)

	// Snapshot lama dengan bentuk PK yang BERBEDA (komposit) + tombstone palsu,
	// meniru sisa dari versi lama.
	mustExec(t, db, `INSERT INTO sync_keys (table_name, key_hash, key_json, pk_sig)
		VALUES ('izin','hashlama','["role-1","perm-1"]','role_id,permission_id')`)
	mustExec(t, db, `INSERT INTO sync_tombstones (table_name, key_hash, key_json, deleted_at, updated_at)
		VALUES ('izin','hashlama','["role-1","perm-1"]','2026-01-01 00:00:00','2026-01-01 00:00:00')`)

	// Data saat ini memakai PK baru `id`.
	mustExec(t, db, `INSERT INTO izin (id, role_id, permission_id) VALUES ('uuid-1','role-1','perm-1')`)

	ti := &tableInfo{Name: "izin", PK: []string{"id"}}

	deleted, err := detectDeletes(ctx, db, driverSQLite, ti)
	if err != nil {
		t.Fatalf("detectDeletes: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("harus 0 penghapusan saat bentuk PK berubah, dapat %d", deleted)
	}

	// Tombstone palsu lama harus ikut dibersihkan.
	if n := scalar(t, db, `SELECT COUNT(1) FROM sync_tombstones WHERE table_name='izin'`); n != "0" {
		t.Fatalf("tombstone palsu belum dibersihkan, count=%s", n)
	}
	// Snapshot harus di-refresh memakai tanda tangan PK baru.
	if n := scalar(t, db, `SELECT COUNT(DISTINCT pk_sig) FROM sync_keys WHERE table_name='izin'`); n != "1" {
		t.Fatalf("pk_sig tidak diperbarui, distinct=%s", n)
	}
	if n := scalar(t, db, `SELECT COUNT(1) FROM sync_keys WHERE table_name='izin' AND pk_sig='id'`); n != "1" {
		t.Fatalf("pk_sig bukan 'id', count=%s", n)
	}
}
