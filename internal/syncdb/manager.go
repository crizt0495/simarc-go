package syncdb

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"arsippro/internal/config"
	"arsippro/internal/database"
)

// ErrBusy is returned when a sync is already in progress.
var ErrBusy = fmt.Errorf("sinkronisasi sedang berjalan")

// Manager runs synchronisation in the background and exposes its status to the
// UI. A single process-wide instance lives in Mgr.
type Manager struct {
	running atomic.Bool
	mu      sync.Mutex
	last    *Result
}

// Mgr is the process-wide sync manager.
var Mgr = &Manager{}

// baselineRunning menandai tarik-baseline pertama (instalasi baru) sedang
// berjalan. Scheduler normal tidak boleh ikut menyinkronkan tabel yang sama
// secara bersamaan.
var baselineRunning atomic.Bool

// MarkBaselineRunning dipakai proses bootstrap awal agar scheduler menunggu.
func MarkBaselineRunning(v bool) { baselineRunning.Store(v) }

// BaselineRunning melaporkan apakah tarik-baseline awal masih berjalan.
func BaselineRunning() bool { return baselineRunning.Load() }

// Running reports whether a sync is currently in progress.
func (m *Manager) Running() bool { return m.running.Load() }

// RunNow performs one synchronisation using the current settings. It refuses to
// run concurrently with another sync.
func (m *Manager) RunNow(ctx context.Context) (Result, error) {
	if baselineRunning.Load() {
		return Result{}, ErrBusy
	}
	if !m.running.CompareAndSwap(false, true) {
		return Result{}, ErrBusy
	}
	defer m.running.Store(false)

	cfg := config.LoadSync()
	if !cfg.HubReady() {
		return Result{}, fmt.Errorf("hub belum dikonfigurasi (SYNC_HUB_DB_HOST / SYNC_HUB_DB_NAME / SYNC_HUB_DB_USER)")
	}

	eng, err := Open(ctx, cfg, log.Printf)
	if err != nil {
		return Result{}, err
	}
	defer eng.Close()

	res, err := eng.Run(ctx)
	if err != nil {
		return res, err
	}

	// Archive files travel separately, over the hub's HTTP endpoints.
	if cfg.FilesActive() {
		fr, ferr := SyncFiles(ctx, cfg, log.Printf)
		res.FilesPushed = fr.Pushed
		res.FilesPulled = fr.Pulled
		res.FileBytesOut = fr.BytesOut
		res.FileBytesIn = fr.BytesIn
		if ferr != nil {
			log.Printf("[sync] berkas: %v", ferr)
		} else if fr.Pushed > 0 || fr.Pulled > 0 {
			log.Printf("[sync] berkas: dorong=%d tarik=%d (%.1f MB keluar, %.1f MB masuk)",
				fr.Pushed, fr.Pulled, float64(fr.BytesOut)/1048576, float64(fr.BytesIn)/1048576)
		}
	}

	m.mu.Lock()
	r := res
	m.last = &r
	m.mu.Unlock()
	log.Printf("[sync] selesai: dorong=%d tarik=%d tabel_error=%d (%s)", res.Pushed, res.Pulled, res.Errors, res.Status())
	return res, nil
}

// Last returns the most recent in-process run result, if any.
func (m *Manager) Last() (Result, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.last == nil {
		return Result{}, false
	}
	return *m.last, true
}

// Status is the JSON view consumed by the settings/status endpoint.
type Status struct {
	Enabled         bool         `json:"enabled"`
	Ready           bool         `json:"ready"`
	NodeID          string       `json:"node_id"`
	HubHost         string       `json:"hub_host"`
	HubDatabase     string       `json:"hub_database"`
	IntervalMinutes int          `json:"interval_minutes"`
	Running         bool         `json:"running"`
	PendingChanges  int          `json:"pending_changes"`
	Recent          []RunSummary `json:"recent"`
}

// Status builds the current synchronisation status. Recent history is read from
// the local database.
func (m *Manager) Status(ctx context.Context) Status {
	cfg := config.LoadSync()
	st := Status{
		Enabled:         cfg.Enabled,
		Ready:           cfg.HubReady(),
		NodeID:          cfg.NodeID,
		HubHost:         cfg.HubDBHost,
		HubDatabase:     cfg.HubDBName,
		IntervalMinutes: cfg.IntervalMinutes,
		Running:         m.Running(),
	}

	db := localSQL()
	if db == nil {
		return st
	}
	if err := ensureLocalSyncTables(ctx, db, database.Driver()); err != nil {
		return st
	}
	st.PendingChanges = pendingChanges(ctx, db, cfg.NodeID)
	if runs, err := RecentRuns(ctx, db, 10); err == nil {
		st.Recent = runs
	}
	return st
}

// pendingChanges counts rows across all syncable tables that have changed
// since the last successful push watermark. Returns 0 on any error so the
// status endpoint never fails because of this best-effort count.
func pendingChanges(ctx context.Context, db *sql.DB, node string) int {
	if db == nil || node == "" {
		return 0
	}
	tables, err := loadTables(ctx, db, database.Driver())
	if err != nil {
		return 0
	}
	total := 0
	for name, ti := range tables {
		strat, ok := strategyFor(ti)
		if !ok {
			continue
		}
		_ = strat
		wm, err := getWatermark(ctx, db, node, name, "push")
		if err != nil {
			continue
		}
		// Build the change-time expression.
		var expr string
		switch {
		case ti.HasUpdatedAt && ti.HasCreatedAt:
			expr = "COALESCE(`updated_at`, `created_at`)"
		case ti.HasUpdatedAt:
			expr = "`updated_at`"
		case ti.HasCreatedAt:
			expr = "`created_at`"
		default:
			// No timestamp column: cannot determine pending rows cheaply.
			continue
		}
		var n int
		q := "SELECT COUNT(*) FROM `" + name + "` WHERE " + expr + " > ?"
		if err := db.QueryRowContext(ctx, q, wm).Scan(&n); err == nil {
			total += n
		}
	}
	return total
}

// localSQL returns the *sql.DB behind the primary GORM connection, if open.
func localSQL() *sql.DB {
	if database.DB == nil {
		return nil
	}
	db, err := database.DB.DB()
	if err != nil {
		return nil
	}
	return db
}

// lastScheduled tracks the previous scheduler tick to enforce the interval.
var lastScheduled time.Time

// consecutiveFailures tracks how many scheduled runs in a row have failed, used
// for exponential backoff on the retry delay.
var consecutiveFailures int

// schedulerOnce guarantees a single background scheduler even when the settings
// page is saved repeatedly.
var schedulerOnce sync.Once

// StartScheduler launches the background synchronisation goroutine. It is a
// no-op until SYNC_ENABLED=true and the hub is configured; the loop re-reads the
// settings on every tick, so enabling sync in the settings page takes effect
// without a restart. Safe to call many times.
func StartScheduler() {
	schedulerOnce.Do(startScheduler)
}

func startScheduler() {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			cfg := config.LoadSync()
			if !cfg.Active() || cfg.IntervalMinutes <= 0 {
				continue
			}
			interval := time.Duration(cfg.IntervalMinutes) * time.Minute
			if !lastScheduled.IsZero() && time.Since(lastScheduled) < interval {
				continue
			}
			if Mgr.Running() || baselineRunning.Load() {
				continue
			}
			lastScheduled = time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			if _, err := Mgr.RunNow(ctx); err != nil {
				consecutiveFailures++
				log.Printf("[sync] terjadwal: %v (kegagalan ke-%d)", err, consecutiveFailures)
				// Exponential backoff: 2^failures minutes, capped at the
				// configured interval. A persistently unreachable hub is not
				// hammered every tick, but transient failures recover quickly.
				retry := time.Duration(1<<uint(consecutiveFailures)) * time.Minute
				if retry > interval {
					retry = interval
				}
				if retry < 2*time.Minute {
					retry = 2 * time.Minute
				}
				lastScheduled = time.Now().Add(retry - interval)
			} else {
				if consecutiveFailures > 0 {
					log.Printf("[sync] terhubung kembali setelah %d kegagalan", consecutiveFailures)
				}
				consecutiveFailures = 0
			}
			cancel()
		}
	}()
}
