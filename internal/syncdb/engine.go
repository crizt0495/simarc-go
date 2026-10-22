package syncdb

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"arsippro/internal/config"
	"arsippro/internal/database"

	_ "github.com/glebarez/go-sqlite"
	_ "github.com/go-sql-driver/mysql"
)

// epoch is the fallback watermark (no row stored yet), which makes the first
// sync of a table a full copy.
var epoch = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)

// Engine synchronises the local database with a hub database.
//
// Every computer keeps a complete local database and can read/write it with no
// network at all. When the hub is reachable, Run() merges changes in both
// directions: local edits are pushed to the hub, and changes made on other
// computers (already merged into the hub) are pulled down.
type Engine struct {
	hubTools
	cfg         config.SyncConfig
	local       *sql.DB
	hub         *sql.DB
	localDriver string
	hubDriver   string
	logf        func(string, ...any)
	tables      map[string]*tableInfo
}

// Open builds an Engine using the application's local database and the hub
// configured in cfg. Database lokal bisa MySQL (instalasi lama) atau SQLite
// (desktop offline-first). Hub juga bisa SQLite (SYNC_HUB_DRIVER=sqlite)
// bila titik temu memakai berkas lokal, selain MySQL (Aiven/kantor).
func Open(ctx context.Context, cfg config.SyncConfig, logf func(string, ...any)) (*Engine, error) {
	if !cfg.HubReady() {
		return nil, fmt.Errorf("hub database belum dikonfigurasi (SYNC_HUB_DB_HOST/NAME/USER)")
	}
	hub, hubDriver, hubDSN, err := openHub(cfg)
	if err != nil {
		return nil, err
	}

	if database.IsSQLite() {
		local, err := openSQLitePool(config.App.DBPath)
		if err != nil {
			hub.Close()
			return nil, fmt.Errorf("buka database lokal SQLite: %w", err)
		}
		return newEngine(ctx, local, driverSQLite, hub, hubDriver, hubDSN, cfg, logf)
	}
	local, err := openPool(dsnFromApp())
	if err != nil {
		hub.Close()
		return nil, fmt.Errorf("buka database lokal: %w", err)
	}
	return newEngine(ctx, local, driverMySQL, hub, hubDriver, hubDSN, cfg, logf)
}

// openHub membuka koneksi ke hub sesuai dialek yang dikonfigurasi.
func openHub(cfg config.SyncConfig) (*sql.DB, string, string, error) {
	if cfg.HubIsSQLite() {
		db, err := openSQLitePool(cfg.HubDBPath)
		if err != nil {
			return nil, driverSQLite, "", fmt.Errorf("buka database hub SQLite: %w", err)
		}
		return db, driverSQLite, cfg.HubDBPath, nil
	}
	hubDSN := dsn(cfg.HubDBHost, cfg.HubDBPort, cfg.HubDBName, cfg.HubDBUser, cfg.HubDBPass, cfg.HubDBTLS)
	db, err := openPool(hubDSN)
	if err != nil {
		return nil, driverMySQL, "", fmt.Errorf("buka database hub: %w", err)
	}
	return db, driverMySQL, hubDSN, nil
}

// OpenWithDSN builds an Engine from explicit MySQL DSNs (CLI/tests).
func OpenWithDSN(ctx context.Context, localDSN, hubDSN string, cfg config.SyncConfig, logf func(string, ...any)) (*Engine, error) {
	local, err := openPool(localDSN)
	if err != nil {
		return nil, fmt.Errorf("buka database lokal: %w", err)
	}
	hub, err := openPool(hubDSN)
	if err != nil {
		local.Close()
		return nil, fmt.Errorf("buka database hub: %w", err)
	}
	return newEngine(ctx, local, driverMySQL, hub, driverMySQL, hubDSN, cfg, logf)
}

// newEngine wires an opened local connection to the hub and prepares the sync
// bookkeeping/schema metadata.
func newEngine(ctx context.Context, local *sql.DB, localDriver string, hub *sql.DB, hubDriver string, hubDSN string, cfg config.SyncConfig, logf func(string, ...any)) (*Engine, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}

	// A single connection per side keeps session settings in effect and avoids
	// SQLite write contention.
	local.SetMaxOpenConns(1)
	hub.SetMaxOpenConns(1)

	if err := ping(ctx, local, "lokal"); err != nil {
		local.Close()
		hub.Close()
		return nil, err
	}
	if err := ping(ctx, hub, "hub"); err != nil {
		local.Close()
		hub.Close()
		return nil, err
	}
	// Kedua sisi harus mematikannya. Baris tiba dengan urutan induk/anak yang
	// bisa tertukar dalam satu delta, sehingga node baru yang menarik data hub
	// bisa kena galat foreign key pada tabel anak. Sesi ini terpisah dari
	// koneksi aplikasi, jadi aman.
	if err := disableForeignKeys(ctx, hub, hubDriver); err != nil {
		local.Close()
		hub.Close()
		return nil, fmt.Errorf("matikan foreign key check hub: %w", err)
	}
	if err := disableForeignKeys(ctx, local, localDriver); err != nil {
		local.Close()
		hub.Close()
		return nil, fmt.Errorf("matikan foreign key check lokal: %w", err)
	}

	if err := ensureLocalSyncTables(ctx, local, localDriver); err != nil {
		local.Close()
		hub.Close()
		return nil, err
	}
	// Tabel bookkeeping sync (terutama sync_tombstones yang dibagi antar-node)
	// juga harus ada di hub.
	if err := ensureLocalSyncTables(ctx, hub, hubDriver); err != nil {
		local.Close()
		hub.Close()
		return nil, fmt.Errorf("siapkan tabel sync hub: %w", err)
	}

	tables, err := loadTables(ctx, local, localDriver)
	if err != nil {
		local.Close()
		hub.Close()
		return nil, err
	}
	if localDriver == driverSQLite {
		// Pastikan tabel yang hilang di hub dibuat dulu (dari model GORM bila
		// dialek beda) supaya tidak semuanya "dilewati" pada hub baru.
		if err := ensureMissingHubTables(ctx, local, localDriver, hub, hubDriver, hubDSN, tables, logf); err != nil {
			local.Close()
			hub.Close()
			return nil, err
		}
		if err := restrictTablesToHub(ctx, hub, hubDriver, tables, logf); err != nil {
			local.Close()
			hub.Close()
			return nil, err
		}
	}

	return &Engine{
		hubTools:    hubTools{dsn: hubDSN, driver: hubDriver},
		cfg:         cfg,
		local:       local,
		hub:         hub,
		localDriver: localDriver,
		hubDriver:   hubDriver,
		logf:        logf,
		tables:      tables,
	}, nil
}

// disableForeignKeys mematikan pemeriksaan foreign key pada satu koneksi.
func disableForeignKeys(ctx context.Context, db *sql.DB, driver string) error {
	if driver == driverSQLite {
		_, err := db.ExecContext(ctx, "PRAGMA foreign_keys=OFF")
		return err
	}
	_, err := db.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=0")
	return err
}

// Close releases both connections.
func (e *Engine) Close() {
	if e.local != nil {
		e.local.Close()
	}
	if e.hub != nil {
		e.hub.Close()
	}
	e.closeHubGorm()
}

// TableResult is the per-table outcome of a run.
type TableResult struct {
	Name     string `json:"name"`
	Strategy string `json:"strategy"`
	Pushed   int    `json:"pushed"`
	Pulled   int    `json:"pulled"`
	Err      string `json:"error,omitempty"`
}

// Result summarises a synchronisation run.
type Result struct {
	NodeID     string        `json:"node_id"`
	StartedAt  time.Time     `json:"started_at"`
	FinishedAt time.Time     `json:"finished_at"`
	Pushed     int           `json:"pushed"`
	Pulled     int           `json:"pulled"`
	Tables     []TableResult `json:"tables"`
	Errors     int           `json:"errors"`

	// Archive-file transfer counts (filled in by the caller after SyncFiles;
	// the database engine itself does not move files).
	FilesPushed  int   `json:"files_pushed,omitempty"`
	FilesPulled  int   `json:"files_pulled,omitempty"`
	FileBytesOut int64 `json:"file_bytes_out,omitempty"`
	FileBytesIn  int64 `json:"file_bytes_in,omitempty"`
}

// Status returns "ok" when the run completed without per-table errors.
func (r Result) Status() string {
	if r.Errors > 0 {
		return "partial"
	}
	return "ok"
}

// Run performs one full synchronisation pass (push + pull) for every eligible
// table. It never aborts the whole run for a single table error: each failure is
// recorded and the remaining tables still sync.
func (e *Engine) Run(ctx context.Context) (Result, error) {
	res := Result{NodeID: e.cfg.NodeID, StartedAt: time.Now()}

	names := make([]string, 0, len(e.tables))
	for name := range e.tables {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return orderKey(names[i]) < orderKey(names[j]) ||
			(orderKey(names[i]) == orderKey(names[j]) && names[i] < names[j])
	})

	// Tombstone yang bentuk primary key-nya sudah tidak cocok dengan tabelnya
	// (skema lama) harus dibuang SEBELUM tabel apa pun disinkronkan. Kalau
	// tidak, node yang masih memakai skema lama akan meneruskannya ke hub dan
	// menghapus baris yang masih sah di sana.
	if n, err := purgeStaleTombstones(ctx, e.local, e.localDriver, e.tables); err != nil {
		e.logf("[sync] gagal membersihkan tombstone usang di lokal: %v", err)
	} else if n > 0 {
		e.logf("[sync] %d tombstone usang dibuang dari lokal", n)
	}
	// Di sisi hub, patokan bentuk primary key diambil dari skema hub sendiri,
	// bukan dari node ini: node dengan skema lama tidak boleh menentukan
	// mana tombstone yang "usang" di hub.
	if hubTables, err := loadTables(ctx, e.hub, e.hubDriver); err != nil {
		e.logf("[sync] gagal membaca skema hub untuk pembersihan tombstone: %v", err)
	} else if n, err := purgeStaleTombstones(ctx, e.hub, e.hubDriver, hubTables); err != nil {
		e.logf("[sync] gagal membersihkan tombstone usang di hub: %v", err)
	} else if n > 0 {
		e.logf("[sync] %d tombstone usang dibuang dari hub", n)
	}

	for _, name := range names {
		ti := e.tables[name]
		strat, ok := strategyFor(ti)
		if !ok {
			continue
		}
		tr := TableResult{Name: name, Strategy: strategyName(strat)}

		var err error
		if strat == stratKeyed {
			err = e.syncKeyedTable(ctx, ti, &tr)
		} else {
			err = e.syncTable(ctx, ti, strat, &tr)
		}
		if err != nil {
			tr.Err = err.Error()
			res.Errors++
			e.logf("[sync] tabel %s gagal: %v", name, err)
		} else {
			res.Pushed += tr.Pushed
			res.Pulled += tr.Pulled
			e.logf("[sync] %s (%s): dorong %d, tarik %d", name, tr.Strategy, tr.Pushed, tr.Pulled)
		}
		res.Tables = append(res.Tables, tr)
	}

	res.FinishedAt = time.Now()
	if err := recordRun(ctx, e.local, e.localDriver, res); err != nil {
		e.logf("[sync] gagal mencatat riwayat: %v", err)
	}
	return res, nil
}

// syncTable pushes local changes to the hub and pulls remote changes back.
func (e *Engine) syncTable(ctx context.Context, ti *tableInfo, strat strategy, tr *TableResult) error {
	exists, err := tableExistsIn(ctx, e.hub, e.hubDriver, ti.Name)
	if err != nil {
		return err
	}
	if !exists {
		if err := ensureHubTable(ctx, e, ti.Name); err != nil {
			return fmt.Errorf("buat tabel %s di hub: %w", ti.Name, err)
		}
	}

	// Push: local -> hub.
	pushSince, err := getWatermark(ctx, e.local, e.cfg.NodeID, ti.Name, "push")
	if err != nil {
		return err
	}
	pushedRows, pushMax, err := readDelta(ctx, e.local, ti, readFrom(pushSince, e.cfg.Overlap()))
	if err != nil {
		return fmt.Errorf("baca perubahan lokal: %w", err)
	}
	if len(pushedRows) > 0 {
		if err := upsertRows(ctx, e.hub, e.hubDriver, ti, strat, pushedRows, e.cfg.BatchSize); err != nil {
			return fmt.Errorf("tulis ke hub: %w", err)
		}
		tr.Pushed = len(pushedRows)
	}
	if err := setWatermark(ctx, e.local, e.localDriver, e.cfg.NodeID, ti.Name, "push", advance(pushSince, pushMax)); err != nil {
		return err
	}

	// Pull: hub -> local.
	pullSince, err := getWatermark(ctx, e.local, e.cfg.NodeID, ti.Name, "pull")
	if err != nil {
		return err
	}
	pulledRows, pullMax, err := readDelta(ctx, e.hub, ti, readFrom(pullSince, e.cfg.Overlap()))
	if err != nil {
		return fmt.Errorf("baca perubahan hub: %w", err)
	}
	if len(pulledRows) > 0 {
		if err := upsertRows(ctx, e.local, e.localDriver, ti, strat, pulledRows, e.cfg.BatchSize); err != nil {
			return fmt.Errorf("tulis ke lokal: %w", err)
		}
		tr.Pulled = len(pulledRows)
	}
	if err := setWatermark(ctx, e.local, e.localDriver, e.cfg.NodeID, ti.Name, "pull", advance(pullSince, pullMax)); err != nil {
		return err
	}
	// Advance the push watermark so rows just pulled are not re-pushed on the
	// next run (avoids a full re-push after the initial baseline pull).
	pushWm, _ := getWatermark(ctx, e.local, e.cfg.NodeID, ti.Name, "push")
	if err := setWatermark(ctx, e.local, e.localDriver, e.cfg.NodeID, ti.Name, "push", advance(pushWm, pullMax)); err != nil {
		e.logf("[sync] gagal memajukan watermark push %s: %v", ti.Name, err)
	}
	return nil
}

// syncKeyedTable merges a hard-delete table (no deleted_at). Rows travel as
// usual, but permanent deletions cannot, so the run also:
//
//  1. detects rows deleted locally since the previous run (shadow diff) and
//     writes tombstones,
//  2. copies those tombstones to the hub and deletes the referenced rows there,
//  3. pulls hub rows back and applies local tombstones (a row re-created after
//     its tombstone survives).
func (e *Engine) syncKeyedTable(ctx context.Context, ti *tableInfo, tr *TableResult) error {
	exists, err := tableExistsIn(ctx, e.hub, e.hubDriver, ti.Name)
	if err != nil {
		return err
	}
	if !exists {
		if err := ensureHubTable(ctx, e, ti.Name); err != nil {
			return fmt.Errorf("buat tabel %s di hub: %w", ti.Name, err)
		}
	}

	// 1. Destroyed rows this side become tombstones.
	if n, err := detectDeletes(ctx, e.local, e.localDriver, ti); err != nil {
		return fmt.Errorf("deteksi penghapusan: %w", err)
	} else if n > 0 {
		e.logf("[sync] %s: %d penghapusan permanen terdeteksi", ti.Name, n)
	}

	// Bentuk primary key di hub bisa berbeda dari lokal bila node ini masih
	// memakai skema lama (mis. permission_role: PK komposit lama vs UUID `id`).
	// Tombstone harus mengikuti bentuk PK masing-masing sisi, kalau tidak
	// tombstone lama bisa menghapus baris yang masih sah di hub.
	hubTI, err := loadTableFor(ctx, e.hub, e.hubDriver, ti.Name)
	if err != nil {
		return fmt.Errorf("bentuk tabel %s di hub: %w", ti.Name, err)
	}
	if len(hubTI.PK) != len(ti.PK) {
		e.logf("[sync] %s: bentuk PK lokal (%s) berbeda dari hub (%s) — propagasi tombstone dilewati",
			ti.Name, strings.Join(ti.PK, "+"), strings.Join(hubTI.PK, "+"))
	}

	// 2. Send tombstones to the hub and apply them there.
	if len(hubTI.PK) == len(ti.PK) {
		if err := copyTombstones(ctx, e.local, e.hub, ti.Name, e.localDriver, e.hubDriver, len(hubTI.PK)); err != nil {
			return fmt.Errorf("kirim tombstone ke hub: %w", err)
		}
	}
	if n, err := applyTombstones(ctx, e.hub, hubTI); err != nil {
		return fmt.Errorf("terapkan tombstone di hub: %w", err)
	} else if n > 0 {
		e.logf("[sync] %s: %d baris dihapus di hub oleh tombstone", ti.Name, n)
	}

	// Push rows (delta) to the hub.
	pushSince, err := getWatermark(ctx, e.local, e.cfg.NodeID, ti.Name, "push")
	if err != nil {
		return err
	}
	pushedRows, pushMax, err := readDelta(ctx, e.local, ti, readFrom(pushSince, e.cfg.Overlap()))
	if err != nil {
		return fmt.Errorf("baca perubahan lokal: %w", err)
	}
	if len(pushedRows) > 0 {
		if err := upsertRows(ctx, e.hub, e.hubDriver, ti, stratKeyed, pushedRows, e.cfg.BatchSize); err != nil {
			return fmt.Errorf("tulis ke hub: %w", err)
		}
		tr.Pushed = len(pushedRows)
	}
	if err := setWatermark(ctx, e.local, e.localDriver, e.cfg.NodeID, ti.Name, "push", advance(pushSince, pushMax)); err != nil {
		return err
	}

	// Pull rows (delta) from the hub, then apply local tombstones.
	pullSince, err := getWatermark(ctx, e.local, e.cfg.NodeID, ti.Name, "pull")
	if err != nil {
		return err
	}
	pulledRows, pullMax, err := readDelta(ctx, e.hub, ti, readFrom(pullSince, e.cfg.Overlap()))
	if err != nil {
		return fmt.Errorf("baca perubahan hub: %w", err)
	}
	if len(pulledRows) > 0 {
		if err := upsertRows(ctx, e.local, e.localDriver, ti, stratKeyed, pulledRows, e.cfg.BatchSize); err != nil {
			return fmt.Errorf("tulis ke lokal: %w", err)
		}
		tr.Pulled = len(pulledRows)
	}
	if err := setWatermark(ctx, e.local, e.localDriver, e.cfg.NodeID, ti.Name, "pull", advance(pullSince, pullMax)); err != nil {
		return err
	}
	// Advance the push watermark so pulled rows are not re-pushed on the next run.
	pushWm, _ := getWatermark(ctx, e.local, e.cfg.NodeID, ti.Name, "push")
	if err := setWatermark(ctx, e.local, e.localDriver, e.cfg.NodeID, ti.Name, "push", advance(pushWm, pullMax)); err != nil {
		e.logf("[sync] gagal memajukan watermark push %s: %v", ti.Name, err)
	}

	if n, err := applyTombstones(ctx, e.local, ti); err != nil {
		return fmt.Errorf("terapkan tombstone lokal: %w", err)
	} else if n > 0 {
		e.logf("[sync] %s: %d baris dihapus lokal oleh tombstone", ti.Name, n)
	}
	return nil
}

// advance returns the next watermark: the newest timestamp seen. When nothing
// newer was seen the previous watermark is kept. re-reads are made safe by the
// overlap window in readFrom plus last-write-wins.
func advance(prev, maxSeen time.Time) time.Time {
	if maxSeen.IsZero() {
		return prev
	}
	if maxSeen.Before(prev) {
		return prev
	}
	return maxSeen
}

// readFrom applies the overlap window to a stored watermark so a boundary row
// whose timestamp equals the watermark is not skipped.
func readFrom(watermark time.Time, overlap time.Duration) time.Time {
	if watermark.IsZero() {
		return epoch
	}
	from := watermark.Add(-overlap)
	if from.Before(epoch) {
		return epoch
	}
	return from
}

// readDelta reads rows changed since the watermark (minus the overlap window)
// and reports the newest timestamp among them.
func readDelta(ctx context.Context, db *sql.DB, ti *tableInfo, since time.Time) ([][]any, time.Time, error) {
	tsCol := ti.timestampColumn()
	if since.IsZero() {
		since = epoch
	}

	cols := quotedColumns(ti)
	q := "SELECT " + cols + " FROM `" + ti.Name + "`"
	var args []any
	if tsCol != "" {
		from := since
		// COALESCE lets rows whose updated_at is NULL fall back to created_at
		// (legacy rows were often written with only created_at). Rows where
		// every timestamp column is NULL are always included: they carry no
		// change time so the only safe option is to resend them (idempotent
		// upsert) until they pick up a timestamp.
		coalesce, allNull := deltaExpr(ti)
		if ti.HasDeletedAt {
			// A soft-deleted row carries deleted_at; updated_at may or may not
			// move on delete, so test both to never miss a tombstone.
			q += " WHERE (" + coalesce + ") >= ? OR (`deleted_at` IS NOT NULL AND `deleted_at` >= ?) OR (" + allNull + ")"
			args = []any{from, from}
		} else {
			q += " WHERE (" + coalesce + ") >= ? OR (" + allNull + ")"
			args = []any{from}
		}
	}

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer rows.Close()

	colNames, err := rows.Columns()
	if err != nil {
		return nil, time.Time{}, err
	}
	tsIdx := -1
	createdIdx := -1
	for i, c := range colNames {
		if tsCol != "" && strings.EqualFold(c, tsCol) {
			tsIdx = i
		}
		if strings.EqualFold(c, "created_at") {
			createdIdx = i
		}
	}

	var out [][]any
	var maxTs time.Time
	for rows.Next() {
		vals := make([]any, len(colNames))
		ptrs := make([]any, len(colNames))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, time.Time{}, err
		}
		// Use COALESCE(updated_at, created_at) so append-only tables whose
		// updated_at is NULL still report a usable change time (otherwise the
		// watermark never advances and every row stays "pending" forever).
		var t time.Time
		ok := false
		if tsIdx >= 0 {
			if v, vok := asTime(vals[tsIdx]); vok {
				t = v
				ok = true
			}
		}
		if !ok && createdIdx >= 0 {
			if v, vok := asTime(vals[createdIdx]); vok {
				t = v
				ok = true
			}
		}
		if ok && t.After(maxTs) {
			maxTs = t
		}
		out = append(out, vals)
	}
	return out, maxTs, rows.Err()
}

// upsertRows writes rows into dst in batches using the strategy's conflict rule.
func upsertRows(ctx context.Context, dst *sql.DB, driver string, ti *tableInfo, strat strategy, rows [][]any, batchSize int) error {
	ncol := len(ti.Columns)
	if ncol == 0 {
		return nil
	}
	batch := batchSize
	if batch <= 0 || batch*ncol > 60000 {
		batch = 60000 / ncol
	}
	if batch < 1 {
		batch = 1
	}

	prefix, suffix := insertParts(driver, ti, strat)
	rowPlaceholder := "(" + strings.TrimSuffix(strings.Repeat("?,", ncol), ",") + ")"

	// Nilai yang dibaca dari MySQL selalu []byte, sedangkan driver SQLite
	// menyimpannya sebagai BLOB saat menerima []byte. BLOB tidak cocok dengan
	// perbandingan teks (WHERE username = "admin" gagal), jadi baris yang
	// ditarik dari hub harus ditulis ulang sebagai teks.
	if driver == driverSQLite {
		rows = textifyRows(rows)
	}

	for start := 0; start < len(rows); start += batch {
		end := start + batch
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[start:end]
		var sb strings.Builder
		sb.WriteString(prefix)
		args := make([]any, 0, len(chunk)*ncol)
		for i, row := range chunk {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(rowPlaceholder)
			args = append(args, row...)
		}
		sb.WriteString(suffix)
		if _, err := dst.ExecContext(ctx, sb.String(), args...); err != nil {
			return err
		}
	}
	return nil
}

// insertParts returns the INSERT prefix and dialect-specific conflict suffix.
func insertParts(driver string, ti *tableInfo, strat strategy) (string, string) {
	cols := quotedColumns(ti)
	prefix := "INSERT INTO `" + ti.Name + "` (" + cols + ") VALUES "
	pk := ti.PK
	if len(pk) == 0 {
		pk = []string{ti.Columns[0]}
	}
	pkList := quotedColumnsList(pk)

	if driver == driverSQLite {
		// Append-only: duplicate primary keys never overwrite history.
		if strat == stratAppend {
			return prefix, " ON CONFLICT (" + pkList + ") DO NOTHING"
		}
		var assigns []string
		for _, c := range ti.Columns {
			if isPK(ti.PK, c) {
				continue
			}
			assigns = append(assigns, "`"+c+"`=excluded.`"+c+"`")
		}
		if len(assigns) == 0 {
			return prefix, " ON CONFLICT (" + pkList + ") DO NOTHING"
		}
		cond := ""
		if ti.HasUpdatedAt {
			cond = " WHERE excluded.`updated_at` >= IFNULL(`" + ti.Name + "`.`updated_at`,'1970-01-01 00:00:00.000')"
		}
		return prefix, " ON CONFLICT (" + pkList + ") DO UPDATE SET " + strings.Join(assigns, ",") + cond
	}

	// MySQL/MariaDB.
	if strat == stratAppend || len(ti.PK) == 0 {
		assigns := make([]string, 0, len(pk))
		for _, c := range pk {
			assigns = append(assigns, "`"+c+"`=`"+c+"`")
		}
		return prefix, " ON DUPLICATE KEY UPDATE " + strings.Join(assigns, ",")
	}

	var assigns []string
	if ti.HasUpdatedAt {
		cond := "VALUES(`updated_at`) >= IFNULL(`updated_at`,'1970-01-01 00:00:00.000')"
		for _, c := range ti.Columns {
			if isPK(ti.PK, c) || c == "updated_at" {
				continue
			}
			assigns = append(assigns, "`"+c+"`=IF("+cond+",VALUES(`"+c+"`),`"+c+"`)")
		}
		assigns = append(assigns, "`updated_at`=IF("+cond+",VALUES(`updated_at`),`updated_at`)")
	} else {
		for _, c := range ti.Columns {
			if isPK(ti.PK, c) {
				continue
			}
			assigns = append(assigns, "`"+c+"`=VALUES(`"+c+"`)")
		}
	}
	if len(assigns) == 0 {
		assigns = []string{"`" + ti.PK[0] + "`=`" + ti.PK[0] + "`"}
	}
	return prefix, " ON DUPLICATE KEY UPDATE " + strings.Join(assigns, ",")
}

// textifyRows mengubah []byte hasil baca MySQL menjadi string agar driver
// SQLite menyimpannya dengan afinitas teks, bukan BLOB.
func textifyRows(rows [][]any) [][]any {
	for i, row := range rows {
		for j, v := range row {
			if b, ok := v.([]byte); ok {
				row[j] = string(b)
			}
		}
		rows[i] = row
	}
	return rows
}

func quotedColumnsList(cols []string) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = "`" + c + "`"
	}
	return strings.Join(parts, ",")
}

// ── small helpers ─────────────────────────────────────────────────────────────

func quotedColumns(ti *tableInfo) string {
	parts := make([]string, len(ti.Columns))
	for i, c := range ti.Columns {
		parts[i] = "`" + c + "`"
	}
	return strings.Join(parts, ",")
}

func isPK(pk []string, col string) bool {
	for _, c := range pk {
		if c == col {
			return true
		}
	}
	return false
}

func strategyName(s strategy) string {
	switch s {
	case stratSoft:
		return "soft"
	case stratAppend:
		return "append"
	default:
		return "keyed"
	}
}

// timeLayouts covers MySQL DATETIME values and the RFC3339-like text that the
// pure-Go SQLite driver stores for time columns.
var timeLayouts = []string{
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05.999999",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05.999999999-07:00",
	time.RFC3339Nano,
	time.RFC3339,
}

// asTime coerces a scanned value into a time.Time.
func asTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case []byte:
		for _, layout := range timeLayouts {
			if parsed, err := time.ParseInLocation(layout, string(t), time.Local); err == nil {
				return parsed, true
			}
		}
	case string:
		for _, layout := range timeLayouts {
			if parsed, err := time.ParseInLocation(layout, t, time.Local); err == nil {
				return parsed, true
			}
		}
	}
	return time.Time{}, false
}

// orderKey sorts master/reference tables before their dependants and logs last.
func orderKey(name string) int {
	switch name {
	case "sync_tombstones":
		return -1 // deletions must reach the hub before keyed tables are merged
	case "roles", "permissions", "unit_kerja", "kode_klasifikasi", "jenis_arsip",
		"lokasi_arsips", "users", "tenants":
		return 0
	case "arsip", "kode_klasifikasi_arsip":
		return 1
	case "arsip_versions", "pemberkasan", "permission_role", "qr_codes",
		"jadwal_retensi", "jadwal_retensi_arsip", "peminjaman_arsips",
		"pemusnahan_arsip":
		return 2
	}
	if appendTables[name] {
		return 4
	}
	return 3
}

func ping(ctx context.Context, db *sql.DB, label string) error {
	// A generous per-attempt timeout absorbs slow DNS/cold-cache lookups (a
	// cloud hub hostname may take several seconds to resolve), and a second
	// attempt absorbs a single transient resolution failure. Both are common on
	// office networks, and a failed sync would otherwise wait a whole interval.
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
		err := db.PingContext(cctx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return fmt.Errorf("hubungkan database %s: %w", label, lastErr)
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("hubungkan database %s: %w", label, lastErr)
}

func openPool(dsn string) (*sql.DB, error) {
	return sql.Open("mysql", dsn)
}

// openSQLitePool membuka database lokal SQLite untuk mesin sinkronisasi.
// Koneksi terpisah dari GORM tetapi menunjuk berkas yang sama (WAL).
func openSQLitePool(path string) (*sql.DB, error) {
	if strings.TrimSpace(path) == "" {
		path = "storage/simarc.db"
	}
	dsn := "file:" + filepath.ToSlash(path) +
		"?_pragma=busy_timeout(15000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(0)"
	return sql.Open("sqlite", dsn)
}

func dsn(host, port, name, user, pass, tls string) string {
	// Normalize the friendly UI values ("required", "disabled", ...) onto the
	// exact modes go-sql-driver understands; see config.NormalizeMySQLTLS.
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local&timeout=30s&readTimeout=180s&writeTimeout=180s&tls=%s",
		user, pass, host, port, name, config.NormalizeMySQLTLS(tls))
}

// dsnFromApp builds the DSN for the local database from the application config.
func dsnFromApp() string {
	return dsn(config.App.DBHost, config.App.DBPort, config.App.DBName,
		config.App.DBUser, config.App.DBPass, config.App.DBTLS)
}
