package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"gorm.io/gorm/logger"
)

// migrateSQLite menyiapkan skema database lokal SQLite (mode offline-first).
//
// Berbeda dengan jalur MySQL yang penuh perbaikan skema legacy, database
// SQLite selalu dibuat baru oleh AutoMigrate. Karena itu tidak ada
// information_schema, ALTER TABLE warisan, atau FULLTEXT di sini.
func migrateSQLite() error {
	DB.Logger = logger.Default.LogMode(logger.Error)

	log.Println("[MIGRASI] Menyiapkan skema SQLite...")
	if err := DB.AutoMigrate(allModels()...); err != nil {
		return err
	}

	// Relasi many2many Role.Permissions membuat GORM membuat ulang tabel
	// penghubung `permission_role` dengan primary key komposit
	// (role_id, permission_id). Bentuk itu berbeda dari server MySQL dan hub
	// yang memakai UUID `id`, dan perbedaan primary keyahl membuat sinkron
	// menghasilkan tombstone palsu yang menghapus hak akses yang sah.
	// Perbaiki ke bentuk kanonik setelah AutoMigrate.
	if err := repairPermissionRoleSQLite(); err != nil {
		log.Println("[MIGRASI] Gagal memperbaiki tabel permission_role:", err)
	}

	// Tabel penghubung yang tidak punya model GORM (dipakai fitur pemusnahan).
	if err := DB.Exec(`CREATE TABLE IF NOT EXISTS pemusnahan_arsip_items (
		id TEXT NOT NULL PRIMARY KEY,
		pemusnahan_id TEXT NOT NULL,
		arsip_id TEXT NOT NULL,
		created_at DATETIME NULL
	)`).Error; err != nil {
		return err
	}

	// Indeks yang paling berpengaruh pada daftar/pencarian arsip. AutoMigrate
	// hanya membuat indeks dari tag model, sedangkan indeks performa ini
	// sebelumnya dibuat manual di MySQL.
	indexes := []struct {
		name  string
		table string
		cols  string
	}{
		{"idx_arsip_deleted_at", "arsip", "deleted_at"},
		{"idx_arsip_unit_kerja", "arsip", "unit_kerja_id"},
		{"idx_arsip_kode_klasifikasi", "arsip", "kode_klasifikasi_id"},
		{"idx_arsip_lokasi_arsip", "arsip", "lokasi_arsip_id"},
		{"idx_arsip_status", "arsip", "status_arsip"},
		{"idx_arsip_tanggal_dibuat", "arsip", "tanggal_dibuat"},
		{"idx_arsip_tanggal_retensi", "arsip", "tanggal_retensi_berakhir"},
		{"idx_arsip_pemberkasan", "arsip", "pemberkasan_id"},
		{"idx_arsip_created_at", "arsip", "created_at"},
		{"idx_arsip_uraian", "arsip", "uraian"},
		{"idx_login_logs_username", "login_logs", "username"},
		{"idx_login_logs_user", "login_logs", "user_id"},
		{"idx_activity_logs_user", "activity_logs", "user_id"},
		{"idx_activity_logs_created", "activity_logs", "created_at"},
		{"idx_pemusnahan_status", "pemusnahan_arsip", "status"},
		{"idx_pemusnahan_created", "pemusnahan_arsip", "created_at"},
		{"idx_arsip_versions_arsip", "arsip_versions", "arsip_id"},
		{"idx_peminjaman_status", "peminjaman_arsips", "status"},
		{"idx_peminjaman_arsip", "peminjaman_arsips", "arsip_id"},
		{"idx_peminjaman_user", "peminjaman_arsips", "user_id"},
		{"idx_blockchain_entity_type", "blockchain_audits", "entity_type"},
		{"idx_blockchain_entity_id", "blockchain_audits", "entity_id"},
		{"idx_pemusnahan_items_pemusnahan", "pemusnahan_arsip_items", "pemusnahan_id"},
		{"idx_pemusnahan_items_arsip", "pemusnahan_arsip_items", "arsip_id"},
	}
	for _, ix := range indexes {
		sql := fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s (%s)", ix.name, ix.table, ix.cols)
		if err := DB.Exec(sql).Error; err != nil {
			log.Printf("[MIGRASI] Lewati indeks %s: %v", ix.name, err)
		}
	}

	// Catat versi skema (dipakai jalur MySQL; di SQLite hanya informasi).
	DB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT NOT NULL PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	DB.Exec("INSERT OR IGNORE INTO schema_migrations (version) VALUES (?)", SchemaVersion)

	log.Println("[MIGRASI] Skema SQLite siap")
	return nil
}

// sqlitePermissionRoleColumns describes one column of a SQLite table.
type sqlitePermissionRoleColumns struct {
	name    string
	typ     string
	notNull bool
	pk      int
}

// repairPermissionRoleSQLite brings a SQLite `permission_role` table to the
// canonical shape used by the MySQL server and the hub:
//
//	id TEXT NOT NULL PRIMARY KEY, permission_id, role_id, created_at, updated_at
//
// GORM's many2many association (Role.Permissions) recreates the join table
// with a composite primary key (role_id, permission_id). That difference is
// not cosmetic: the sync engine compares the primary-key shape of a table
// across nodes, so a client with the old composite key produces tombstones in
// the old format — and those tombstones delete valid rows on the hub and on
// every other node.
//
// SQLite cannot drop a primary key in place, so the table is rebuilt and its
// rows are copied over. The rows are dropped rather than carried over: their
// ids cannot match the hub's (the hub keys the same pairs with its own
// UUIDs), and keeping them would duplicate every role/permission pair in the
// hub. The hub re-supplies them on the next sync.
func repairPermissionRoleSQLite() error {
	ctx := context.Background()
	conn, err := DB.DB()
	if err != nil {
		return err
	}
	sqlConn, err := conn.Conn(ctx)
	if err != nil {
		return err
	}
	defer sqlConn.Close()

	rows, err := sqlConn.QueryContext(ctx, `PRAGMA table_info('permission_role')`)
	if err != nil {
		return err
	}
	var cols []sqlitePermissionRoleColumns
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		cols = append(cols, sqlitePermissionRoleColumns{name: name, typ: typ, notNull: notNull > 0, pk: pk})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil // tabel tidak ada
	}

	has := func(name string) bool {
		for _, c := range cols {
			if c.name == name {
				return true
			}
		}
		return false
	}
	pkCols := []string{}
	for _, c := range cols {
		if c.pk > 0 {
			pkCols = append(pkCols, c.name)
		}
	}
	canonical := len(pkCols) == 1 && pkCols[0] == "id" &&
		has("permission_id") && has("role_id") &&
		has("created_at") && has("updated_at")
	if canonical {
		return nil
	}

	// Foreign key check dimatikan agar tabel lama boleh dihapus.
	if _, err := sqlConn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}
	defer func() { _, _ = sqlConn.ExecContext(ctx, `PRAGMA foreign_keys=ON`) }()

	if _, err := sqlConn.ExecContext(ctx, `DROP TABLE IF EXISTS permission_role_new`); err != nil {
		return err
	}
	create := `CREATE TABLE permission_role_new (
		id TEXT NOT NULL PRIMARY KEY,
		permission_id TEXT NOT NULL,
		role_id TEXT NOT NULL,
		created_at DATETIME NULL,
		updated_at DATETIME NULL,
		CONSTRAINT fk_permission_role_permission FOREIGN KEY (permission_id) REFERENCES permissions(id),
		CONSTRAINT fk_permission_role_role FOREIGN KEY (role_id) REFERENCES roles(id)
	)`
	if _, err := sqlConn.ExecContext(ctx, create); err != nil {
		return err
	}

	// Baris lama TIDAK disalin. Id baris pivot harus UUID dan harus sama
	// dengan id yang dipakai hub untuk pasangan role/permission yang sama;
	// id acak baru justru akan membuat setiap pasangan terduplikasi di hub.
	// Tabel `permission_role` adalah cermin data server, jadi isinya diambil
	// ulang dari hub pada sinkron berikutnya.
	oldCount := 0
	if err := sqlConn.QueryRowContext(ctx, `SELECT COUNT(1) FROM permission_role`).Scan(&oldCount); err != nil {
		oldCount = 0
	}

	if _, err := sqlConn.ExecContext(ctx, `DROP TABLE permission_role`); err != nil {
		return err
	}
	if _, err := sqlConn.ExecContext(ctx, `ALTER TABLE permission_role_new RENAME TO permission_role`); err != nil {
		return err
	}

	// Bookmark sinkron memakai bentuk primary key lama; harus dibuang agar
	// sinkron berikutnya memperlakukan tabel ini sebagai tabel baru.
	for _, t := range []string{"sync_keys", "sync_tombstones", "sync_state"} {
		hasTbl := false
		if err := sqlConn.QueryRowContext(ctx,
			`SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name=?`, t).Scan(&hasTbl); err != nil {
			continue
		}
		if hasTbl {
			_, _ = sqlConn.ExecContext(ctx, `DELETE FROM `+t+` WHERE table_name = 'permission_role'`)
		}
	}

	log.Printf("[MIGRASI] permission_role diperbaiki ke primary key UUID `id` (%d baris lama dilepas, data diambil ulang dari hub)", oldCount)
	return nil
}

// quoteCols wraps column names in backticks for SQL building.
func quoteCols(cols []string) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = "`" + c + "`"
	}
	return out
}
