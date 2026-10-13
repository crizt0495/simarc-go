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

// Running reports whether a sync is currently in progress.
func (m *Manager) Running() bool { return m.running.Load() }

// RunNow performs one synchronisation using the current settings. It refuses to
// run concurrently with another sync.
func (m *Manager) RunNow(ctx context.Context) (Result, error) {
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
	if err := ensureLocalSyncTables(ctx, db); err != nil {
		return st
	}
	if runs, err := RecentRuns(ctx, db, 10); err == nil {
		st.Recent = runs
	}
	return st
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

// StartScheduler launches the background synchronisation goroutine. It is a
// no-op until SYNC_ENABLED=true and the hub is configured; the loop re-reads the
// settings on every tick, so enabling sync in the settings page takes effect
// without a restart.
func StartScheduler() {
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
			if Mgr.Running() {
				continue
			}
			lastScheduled = time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			if _, err := Mgr.RunNow(ctx); err != nil {
				log.Printf("[sync] terjadwal: %v", err)
			}
			cancel()
		}
	}()
}
