package syncdb

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"arsippro/internal/config"

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
	cfg    config.SyncConfig
	local  *sql.DB
	hub    *sql.DB
	logf   func(string, ...any)
	tables map[string]*tableInfo
}

// Open builds an Engine using the application's local database and the hub
// configured in cfg.
func Open(ctx context.Context, cfg config.SyncConfig, logf func(string, ...any)) (*Engine, error) {
	if !cfg.HubReady() {
		return nil, fmt.Errorf("hub database belum dikonfigurasi (SYNC_HUB_DB_HOST/NAME/USER)")
	}
	return OpenWithDSN(ctx, dsnFromApp(),
		dsn(cfg.HubDBHost, cfg.HubDBPort, cfg.HubDBName, cfg.HubDBUser, cfg.HubDBPass, cfg.HubDBTLS),
		cfg, logf)
}

// OpenWithDSN builds an Engine from explicit DSNs. It is used by Open and by
// the CLI/tests that need to point at specific databases.
func OpenWithDSN(ctx context.Context, localDSN, hubDSN string, cfg config.SyncConfig, logf func(string, ...any)) (*Engine, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	local, err := openPool(localDSN)
	if err != nil {
		return nil, fmt.Errorf("buka database lokal: %w", err)
	}
	hub, err := openPool(hubDSN)
	if err != nil {
		local.Close()
		return nil, fmt.Errorf("buka database hub: %w", err)
	}

	// A single connection per side keeps the session-level
	// SET FOREIGN_KEY_CHECKS=0 in effect for the whole run.
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
	if _, err := hub.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=0"); err != nil {
		local.Close()
		hub.Close()
		return nil, fmt.Errorf("set foreign_key_checks hub: %w", err)
	}
	// Kedua sisi harus mematikannya. Baris tiba dengan urutan induk/anak yang
	// bisa tertukar dalam satu delta, sehingga node baru yang menarik data hub
	// bisa kena galat foreign key pada tabel anak (mis. arsip sebelum
	// kode_klasifikasi). Sesi ini terpisah dari koneksi aplikasi, jadi aman.
	if _, err := local.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=0"); err != nil {
		local.Close()
		hub.Close()
		return nil, fmt.Errorf("set foreign_key_checks lokal: %w", err)
	}

	if err := ensureLocalSyncTables(ctx, local); err != nil {

		local.Close()
		hub.Close()
		return nil, err
	}

	tables, err := loadTables(ctx, local)
	if err != nil {
		local.Close()
		hub.Close()
		return nil, err
	}

	return &Engine{cfg: cfg, local: local, hub: hub, logf: logf, tables: tables}, nil
}

// Close releases both connections.
func (e *Engine) Close() {
	if e.local != nil {
		e.local.Close()
	}
	if e.hub != nil {
		e.hub.Close()
	}
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
	if err := recordRun(ctx, e.local, res); err != nil {
		e.logf("[sync] gagal mencatat riwayat: %v", err)
	}
	return res, nil
}

// syncTable pushes local changes to the hub and pulls remote changes back.
func (e *Engine) syncTable(ctx context.Context, ti *tableInfo, strat strategy, tr *TableResult) error {
	exists, err := tableExistsIn(ctx, e.hub, ti.Name)
	if err != nil {
		return err
	}
	if !exists {
		if err := createTableLike(ctx, e.local, e.hub, ti.Name); err != nil {
			return fmt.Errorf("buat tabel di hub: %w", err)
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
		if err := upsertRows(ctx, e.hub, ti, strat, pushedRows, e.cfg.BatchSize); err != nil {
			return fmt.Errorf("tulis ke hub: %w", err)
		}
		tr.Pushed = len(pushedRows)
	}
	if err := setWatermark(ctx, e.local, e.cfg.NodeID, ti.Name, "push", advance(pushSince, pushMax)); err != nil {
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
		if err := upsertRows(ctx, e.local, ti, strat, pulledRows, e.cfg.BatchSize); err != nil {
			return fmt.Errorf("tulis ke lokal: %w", err)
		}
		tr.Pulled = len(pulledRows)
	}
	return setWatermark(ctx, e.local, e.cfg.NodeID, ti.Name, "pull", advance(pullSince, pullMax))
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
	exists, err := tableExistsIn(ctx, e.hub, ti.Name)
	if err != nil {
		return err
	}
	if !exists {
		if err := createTableLike(ctx, e.local, e.hub, ti.Name); err != nil {
			return fmt.Errorf("buat tabel di hub: %w", err)
		}
	}

	// 1. Destroyed rows this side become tombstones.
	if n, err := detectDeletes(ctx, e.local, ti); err != nil {
		return fmt.Errorf("deteksi penghapusan: %w", err)
	} else if n > 0 {
		e.logf("[sync] %s: %d penghapusan permanen terdeteksi", ti.Name, n)
	}

	// 2. Send tombstones to the hub and apply them there.
	if err := copyTombstones(ctx, e.local, e.hub, ti.Name); err != nil {
		return fmt.Errorf("kirim tombstone ke hub: %w", err)
	}
	if n, err := applyTombstones(ctx, e.hub, ti); err != nil {
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
		if err := upsertRows(ctx, e.hub, ti, stratKeyed, pushedRows, e.cfg.BatchSize); err != nil {
			return fmt.Errorf("tulis ke hub: %w", err)
		}
		tr.Pushed = len(pushedRows)
	}
	if err := setWatermark(ctx, e.local, e.cfg.NodeID, ti.Name, "push", advance(pushSince, pushMax)); err != nil {
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
		if err := upsertRows(ctx, e.local, ti, stratKeyed, pulledRows, e.cfg.BatchSize); err != nil {
			return fmt.Errorf("tulis ke lokal: %w", err)
		}
		tr.Pulled = len(pulledRows)
	}
	if err := setWatermark(ctx, e.local, e.cfg.NodeID, ti.Name, "pull", advance(pullSince, pullMax)); err != nil {
		return err
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
	for i, c := range colNames {
		if tsCol != "" && strings.EqualFold(c, tsCol) {
			tsIdx = i
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
		if tsIdx >= 0 {
			if t, ok := asTime(vals[tsIdx]); ok && t.After(maxTs) {
				maxTs = t
			}
		}
		out = append(out, vals)
	}
	return out, maxTs, rows.Err()
}

// upsertRows writes rows into dst in batches using the strategy's conflict rule.
func upsertRows(ctx context.Context, dst *sql.DB, ti *tableInfo, strat strategy, rows [][]any, batchSize int) error {
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

	prefix, suffix := insertParts(ti, strat)
	rowPlaceholder := "(" + strings.TrimSuffix(strings.Repeat("?,", ncol), ",") + ")"

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

// insertParts returns the (prefix, ON DUPLICATE KEY suffix) for a strategy.
func insertParts(ti *tableInfo, strat strategy) (string, string) {
	cols := quotedColumns(ti)
	prefix := "INSERT INTO `" + ti.Name + "` (" + cols + ") VALUES "

	// Append-only: a duplicate key is a no-op (never overwrite history).
	if strat == stratAppend || len(ti.PK) == 0 {
		pk := ti.PK
		if len(pk) == 0 {
			pk = []string{ti.Columns[0]}
		}
		assigns := make([]string, 0, len(pk))
		for _, c := range pk {
			assigns = append(assigns, "`"+c+"`=`"+c+"`")
		}
		return prefix, " ON DUPLICATE KEY UPDATE " + strings.Join(assigns, ",")
	}

	// Tables with updated_at use last-write-wins; others overwrite.
	var assigns []string
	if ti.HasUpdatedAt {
		cond := "VALUES(`updated_at`) >= IFNULL(`updated_at`,'1970-01-01 00:00:00.000')"
		for _, c := range ti.Columns {
			if isPK(ti.PK, c) || c == "updated_at" {
				continue
			}
			assigns = append(assigns, "`"+c+"`=IF("+cond+",VALUES(`"+c+"`),`"+c+"`)")
		}
		// Assign updated_at last so the conditions above read the previous value.
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
		// Only primary-key columns: a duplicate is a no-op.
		assigns = []string{"`" + ti.PK[0] + "`=`" + ti.PK[0] + "`"}
	}
	return prefix, " ON DUPLICATE KEY UPDATE " + strings.Join(assigns, ",")
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

// asTime coerces a scanned value into a time.Time.
func asTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case []byte:
		for _, layout := range []string{"2006-01-02 15:04:05.999999", "2006-01-02 15:04:05", time.RFC3339} {
			if parsed, err := time.ParseInLocation(layout, string(t), time.Local); err == nil {
				return parsed, true
			}
		}
	case string:
		for _, layout := range []string{"2006-01-02 15:04:05.999999", "2006-01-02 15:04:05", time.RFC3339} {
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
		"pemusnahan_arsip", "berita_acara":
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
