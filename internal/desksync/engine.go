package desksync

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"arsippro/internal/config"
	"arsippro/internal/database"

	"gorm.io/gorm"
)

// ── ENGINE ────────────────────────────────────────────────────────────────────

// Status ikhtisar kondisi mesin — dikirim ke badge UI & API status.
type Status struct {
	Mode          string     `json:"mode"` // local
	Enabled       bool       `json:"enabled"`
	Running       bool       `json:"running"`   // siklus sedang berjalan
	Online        bool       `json:"online"`    // pusat terjangkau siklus terakhir
	LastSync      *time.Time `json:"last_sync"` // waktu siklus terakhir dijalankan
	LastPushAt    *time.Time `json:"last_push_at"`
	LastPullAt    *time.Time `json:"last_pull_at"`
	Pending       int64      `json:"pending"` // perubahan lokal yang belum terkirim
	LastError     string     `json:"last_error"`
	LastPushCount int        `json:"last_push_count"`
	LastPullCount int        `json:"last_pull_count"`
	Interval      int        `json:"interval"` // detik
	RemoteHost    string     `json:"remote_host"`
	RemoteDB      string     `json:"remote_db"`
	ClientID      string     `json:"client_id"`
	LocalDB       string     `json:"local_db"`
	FirstSyncDone bool       `json:"first_sync_done"`
}

// Engine menyimpan konfigurasi sinkronisasi dan status runtime.
type Engine struct {
	// koneksi pusat
	Host, Port, Name, User, Pass, TLS string
	ClientID                          string
	Interval                          time.Duration
	PullFileBlobs                     bool
	FileMax                           int64

	// openRemote menggantikan dial MySQL pusat — diisi pengujian.
	openRemote func() (*gorm.DB, func(), error)

	runMu    sync.Mutex // serialisasi siklus
	statusMu sync.Mutex
	current  Status

	ctx     context.Context
	cancel  context.CancelFunc
	started bool
}

// Snapshot salinan status terkini (aman untuk konsumsi JSON handler).
func (e *Engine) Snapshot() Status {
	e.statusMu.Lock()
	defer e.statusMu.Unlock()
	return e.current
}

func (e *Engine) stat(fn func(s *Status)) {
	e.statusMu.Lock()
	defer e.statusMu.Unlock()
	fn(&e.current)
}

// ── Konstruksi ───────────────────────────────────────────────────────────────

// Params kumpulan parameter engine (memudahkan pengujian).
type Params struct {
	ClientID      string
	Host          string
	Port          string
	Name          string
	User          string
	Pass          string
	TLS           string
	Interval      time.Duration
	PullFileBlobs bool
	FileMax       int64
}

// New membuat engine dari parameter.
func New(p Params) *Engine {
	if p.Interval <= 0 {
		p.Interval = 60 * time.Second
	}
	if p.ClientID == "" {
		p.ClientID = "local"
	}
	return &Engine{
		Host: p.Host, Port: p.Port, Name: p.Name, User: p.User, Pass: p.Pass, TLS: p.TLS,
		ClientID: p.ClientID, Interval: p.Interval,
		PullFileBlobs: p.PullFileBlobs,
		FileMax:       p.FileMax,
	}
}

// FromConfig membuat engine dari konfigurasi aplikasi (pusat = DB_*).
func FromConfig() *Engine {
	cfg := config.App
	return New(Params{
		ClientID:      cfg.SyncClientID,
		Host:          cfg.DBHost,
		Port:          cfg.DBPort,
		Name:          cfg.DBName,
		User:          cfg.DBUser,
		Pass:          cfg.DBPass,
		TLS:           cfg.DBTLS,
		Interval:      time.Duration(cfg.SyncInterval) * time.Second,
		PullFileBlobs: cfg.SyncPullFileBlobs,
		FileMax:       cfg.SyncFileMaxBytes,
	})
}

// ── Default engine singleton ─────────────────────────────────────────────────

var (
	defMu    sync.Mutex
	defaultE *Engine
)

// SetDefault menetapkan engine aktif global.
func SetDefault(e *Engine) {
	defMu.Lock()
	defaultE = e
	defMu.Unlock()
}

// Default mengembalikan engine aktif global (nil saat mode mysql).
func Default() *Engine {
	defMu.Lock()
	defer defMu.Unlock()
	return defaultE
}

// ── Lifecycle ────────────────────────────────────────────────────────────────

// Start menjalankan loop sinkronisasi: siklus pertama langsung, lalu per
// interval. Non-blocking.
func (e *Engine) Start() {
	e.runMu.Lock()
	if e.started {
		e.runMu.Unlock()
		return
	}
	e.started = true
	e.runMu.Unlock()

	e.ctx, e.cancel = context.WithCancel(context.Background())
	e.stat(func(s *Status) {
		s.Enabled = true
		s.Mode = "local"
		s.Interval = int(e.Interval.Seconds())
		s.ClientID = e.ClientID
		s.RemoteHost = e.Host
		s.RemoteDB = e.Name
		s.LocalDB = database.SQLitePath()
	})

	log.Printf("[SYNC] Mesin sinkronisasi aktif — interval %s, pusat %s:%s/%s",
		e.Interval, e.Host, e.Port, e.Name)

	go e.loop()
}

func (e *Engine) loop() {
	// Siklus pertama langsung agar akun pusat segera tersedia.
	_ = e.runOnce()
	ticker := time.NewTicker(e.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			_ = e.runOnce()
		}
	}
}

// Stop menghentikan loop.
func (e *Engine) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
}

// TriggerSync memicu satu siklus sinkronisasi segera (non-blocking).
func (e *Engine) TriggerSync() {
	go func() { _ = e.runOnce() }()
}

// BootstrapSync menjalankan satu siklus secara blocking dengan batas waktu —
// dipanggil sekali di startup agar akun/katalog pusat langsung tersedia.
func (e *Engine) BootstrapSync(maxWait time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- e.runOnce() }()
	select {
	case err := <-done:
		return err
	case <-time.After(maxWait):
		return fmt.Errorf("sinkronisasi awal melebihi %s — proses tetap berjalan", maxWait)
	}
}

// runOnce mengeksekusi satu siklus penuh; aman dipanggil dari thread mana pun.
func (e *Engine) runOnce() error {
	if !e.runMu.TryLock() {
		return nil // siklus lain sedang berjalan
	}
	defer e.runMu.Unlock()

	e.stat(func(s *Status) { s.Running = true })

	err := e.cycle()
	now := time.Now()
	e.stat(func(s *Status) {
		s.Running = false
		s.LastSync = &now
		if err != nil {
			s.LastError = err.Error()
		} else {
			s.LastError = ""
			s.Online = true
		}
	})
	return err
}

// CycleResult memberi hasil satu siklus untuk pengujian.
type CycleResult struct {
	Pushed int
	Pulled int
}

// cycle: dial pusat, push, pull, reconcile, catat status.
func (e *Engine) cycle() error {
	cycleStart := time.Now().UTC().Truncate(time.Second)

	local := database.DB
	if local == nil {
		return fmt.Errorf("database lokal belum siap")
	}

	// Seluruh tulisan mesin (peta id, apply pull, rekonsiliasi seed,
	// ringkasan blockchain) bukan aksi pengguna — hentikan pencatatan outbox
	// agar tidak terjadi dorongan-berulang. Perubahan pengguna yang terjadi
	// DI SEKITAR siklus tetap tertangkap rescan watermark siklus berikutnya.
	database.PauseSyncHooks()
	defer database.ResumeSyncHooks()

	remote, closer, err := func() (*gorm.DB, func(), error) {
		if e.openRemote != nil {
			return e.openRemote()
		}
		return database.OpenMySQL(e.Host, e.Port, e.Name, e.User, e.Pass, e.TLS)
	}()
	if err != nil {
		e.stat(func(s *Status) { s.Online = false })
		return fmt.Errorf("pusat tidak terjangkau: %w", err)
	}
	defer closer()

	// Ping eksplisit — gorm.Open dengan DSN MySQL lazily.
	if sqlDB, err := remote.DB(); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := sqlDB.PingContext(ctx); err != nil {
			e.stat(func(s *Status) { s.Online = false })
			return fmt.Errorf("pusat tidak membalas ping: %w", err)
		}
	}

	e.stat(func(s *Status) { s.Online = true })

	pushed, pushErr := e.pushAll(local, remote, cycleStart)
	pulled, pullErr := e.pullAll(local, remote, cycleStart)
	e.reconcileAfterFirstPull(local)

	e.stat(func(s *Status) {
		s.LastPushCount = pushed
		s.LastPullCount = pulled
		s.Pending = database.OutboxCountPending()
		if pushed > 0 || pulled > 0 {
			s.FirstSyncDone = true
		}
	})

	e.writeChainSummary(remote, local, pushed, pulled)

	// Gagal parsial biasanya baris tunggal yang dilewati (dilog) — laporan
	// kesalahan level fatal hanya untuk kegagalan seluruh fase.
	if pushErr != nil && pullErr != nil {
		return fmt.Errorf("push: %v; pull: %v", pushErr, pullErr)
	} else if pushErr != nil {
		return fmt.Errorf("push: %w", pushErr)
	} else if pullErr != nil {
		return fmt.Errorf("pull: %w", pullErr)
	}
	return nil
}
