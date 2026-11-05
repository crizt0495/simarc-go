package main

import (
	"bufio"
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"arsippro/internal/config"
	"arsippro/internal/database"
	synceng "arsippro/internal/sync"
)

// Versi aplikasi (selaras wails.json productVersion).
const appVersion = "1.0.0"

var errNotReady = errors.New("database belum tersambung — periksa konfigurasi MySQL lokal")

// App memegang seluruh proses bisnis & akses database lokal.
// Frontend (React) hanya memanggil binding di bawah — tidak pernah
// menyentuh database/http server secara langsung.
type App struct {
	ctx context.Context

	migrations embed.FS
	window     string

	// DB + engine
	mu     sync.Mutex
	cfg    *config.Config
	eng    *database.LocalEngine
	engErr error

	// sinkronisasi
	syncEng     *synceng.Engine
	syncMu      sync.Mutex
	startSync   sync.Once
	syncLast    string
	syncLastOK  bool
	syncRunErr  error
	syncEnabled bool

	startedAt time.Time
}

// NewApp menyiapkan aplikasi dengan FS migrasi SQL dan mode jendela awal.
func NewApp(migrations embed.FS) *App {
	return &App{migrations: migrations}
}

// startup dipanggil Wails saat aplikasi dimulai: memuat config, membuka DB,
// dan menjalankan migrasi otomatis (offline-first resilient).
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.startedAt = time.Now()

	cfg, envPath := loadEnvConfig()
	a.cfg = cfg

	if eng, err := database.NewLocalEngine(cfg.DSN()); err == nil {
		a.eng, a.engErr, a.syncEnabled = eng, nil, cfg.SyncEnabled
		if err := eng.Migrate(fs.FS(a.migrations)); err != nil {
			a.engErr = fmt.Errorf("migrasi skema: %w", err)
		}
		if a.engErr == nil {
			a.initSync()
			a.startSync.Do(a.syncLoop)
		}
	} else {
		a.eng, a.engErr = nil, err
	}
	if a.cfg.Debug {
		if a.engErr != nil {
			fmt.Fprintf(os.Stderr, "[simarc] DB gagal: %v\n", a.engErr)
		} else {
			fmt.Fprintf(os.Stderr, "[simarc] DB ok: %s:%d/%s\n", cfg.DBHost, cfg.DBPort, cfg.DBName)
		}
	}
	_ = envPath

	// Bila DB belum siap (mis. MariaDB baru menyala setelah login),
	// coba lagi beberapa waktu kemudian tanpa membekukan UI.
	if a.engErr != nil {
		go a.reconnectLoop()
	}
}

// loadEnvConfig membaca .env di samping executable (portable & offline);
// bila tidak ada, fallback ke CWD (mode dev), lalu default.
func loadEnvConfig() (*config.Config, string) {
	var paths []string
	if exe, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(exe), ".env"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths, filepath.Join(home, ".env"))
	}
	paths = append(paths, ".env")
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			if c, err := config.LoadFrom(p); err == nil {
				return c, p
			}
		}
	}
	c, _ := config.Load()
	return c, ".env"
}

// initSync menyiapkan sync engine terhadap hub (bila dikonfigurasi).
func (a *App) initSync() {
	if !a.cfg.SyncEnabled || a.eng == nil || a.cfg.SyncHubURL == "" {
		return
	}
	a.syncEng = synceng.New(a.eng.DB(), a.cfg.SyncHubURL, a.cfg.SyncHubToken)
	_ = a.syncEng.ResetOnRestart()
}

// syncLoop sinkronisasi otomatis di latar belakang; aman bila offline.
func (a *App) syncLoop() {
	a.mu.Lock()
	eng := a.syncEng
	interval := a.cfg.SyncInterval
	if interval <= time.Minute {
		interval = 15 * time.Minute
	}
	a.mu.Unlock()
	if eng == nil {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-t.C:
			a.mu.Lock()
			err := eng.RunSync(a.ctx)
			a.syncRunErr, a.syncLastOK, a.syncLast = err, err == nil, time.Now().Format(time.DateTime)
			a.mu.Unlock()
		}
	}
}

// reconnectLoop menyambungkan ulang DB lokal tiap 10 detik bila gagal startup.
func (a *App) reconnectLoop() {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for round := 0; round < 120; round++ { // maksimal ~20 menit
		select {
		case <-a.ctx.Done():
			return
		case <-t.C:
			a.mu.Lock()
			cfg := a.cfg
			a.mu.Unlock()
			eng, err := database.NewLocalEngine(cfg.DSN())
			if err != nil {
				continue
			}
			a.mu.Lock()
			a.eng, a.engErr, a.syncEnabled = eng, nil, cfg.SyncEnabled
			if err := eng.Migrate(fs.FS(a.migrations)); err != nil {
				a.engErr = fmt.Errorf("migrasi skema: %w", err)
			} else {
				a.initSync()
				a.startSync.Do(a.syncLoop)
			}
			a.mu.Unlock()
			return
		}
	}
}

// shutdown dipanggil Wails saat aplikasi ditutup: menutup pool DB bersih.
func (a *App) shutdown(context.Context) {
	a.mu.Lock()
	eng := a.eng
	a.mu.Unlock()
	if eng != nil {
		_ = eng.Close()
	}
}

// envPath aktif (dipakai SaveSettings bila dev-friendly).
var _ = config.EnvFilePath

// ── Bindings: frontend ⟷ Golang ─────────────────────────────────────────

// AppStatus status ringkas aplikasi (tanpa kredensial).
type AppStatus struct {
	App       string `json:"app"`
	Version   string `json:"version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Started   string `json:"started"`
	Uptime    string `json:"uptime"`
	Window    string `json:"window"`
	DBOK      bool   `json:"db_ok"`
	DBErr     string `json:"db_err"`
	DBEngine  string `json:"db_engine"`
	DBAddress string `json:"db_address"`
	DBName    string `json:"db_name"`
	DBTLS     string `json:"db_tls"`
	SyncOK    bool   `json:"sync_ok"`
	SyncErr   string `json:"sync_err"`
	SyncOn    bool   `json:"sync_on"`
	LastSync  string `json:"last_sync"`
	NodeID    string `json:"node_id"`
}

// GetStatus mengembalikan status aplikasi & database lokal.
func (a *App) GetStatus() AppStatus {
	a.mu.Lock()
	cfg, eng, engErr, syncEnabled := a.cfg, a.eng, a.engErr, a.syncEnabled
	a.mu.Unlock()

	s := AppStatus{
		App:       "SIMARC",
		Version:   appVersion,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		Started:   a.startedAt.Format(time.DateTime),
		Uptime:    time.Since(a.startedAt).Round(time.Second).String(),
		Window:    cfg.Window,
		DBEngine:  "MySQL/MariaDB",
		DBAddress: fmt.Sprintf("%s:%d", cfg.DBHost, cfg.DBPort),
		DBName:    cfg.DBName,
		DBTLS:     cfg.DBTLS,
		SyncOn:    syncEnabled,
		SyncOK:    a.syncLastOK,
		LastSync:  a.syncLast,
		NodeID:    cfg.SyncNodeID,
	}
	if a.syncRunErr != nil {
		s.SyncErr = a.syncRunErr.Error()
	}
	if eng != nil {
		s.DBOK = true
		if err := eng.HealthCheck(); err != nil {
			s.DBOK = false
			s.DBErr = err.Error()
		}
	} else if engErr != nil {
		s.DBErr = engErr.Error()
	}
	return s
}

// PingBackend memastikan bindings Golang ⟷ React aktif.
func (a *App) PingBackend() string {
	return fmt.Sprintf("pong @ %s", time.Now().Format("15:04:05"))
}

// ListArsip daftar arsip terbaru dari DB lokal.
func (a *App) ListArsip() []database.Arsip {
	a.mu.Lock()
	eng := a.eng
	a.mu.Unlock()
	if eng == nil {
		return []database.Arsip{}
	}
	out, err := eng.ListArsip(200)
	if err != nil {
		return []database.Arsip{}
	}
	return out
}

// CreateArsip menambahkan arsip baru (tulis ke DB lokal).
func (a *App) CreateArsip(nama, kategori string) (database.Arsip, error) {
	a.mu.Lock()
	eng := a.eng
	a.mu.Unlock()
	if eng == nil {
		return database.Arsip{}, errNotReady
	}
	return eng.CreateArsip(nama, kategori)
}

// DeleteArsip soft-delete arsip berdasar hex UUID.
func (a *App) DeleteArsip(id string) (bool, error) {
	a.mu.Lock()
	eng := a.eng
	a.mu.Unlock()
	if eng == nil {
		return false, errNotReady
	}
	return eng.DeleteArsip(id)
}

// TriggerSync menjalankan push+pull manual ke hub.
func (a *App) TriggerSync() (map[string]interface{}, error) {
	a.mu.Lock()
	cfg, eng := a.cfg, a.eng
	if eng == nil {
		a.mu.Unlock()
		return nil, errNotReady
	}
	switch {
	case !cfg.SyncEnabled:
		a.mu.Unlock()
		return map[string]interface{}{"ran": false, "msg": "sinkronisasi dinonaktifkan (SYNC_ENABLED=false)"}, nil
	case cfg.SyncHubURL == "":
		a.mu.Unlock()
		return map[string]interface{}{"ran": false, "msg": "SYNC_HUB_URL belum diisi"}, nil
	}
	if a.syncEng == nil {
		a.initSync()
	}
	sEng := a.syncEng
	a.mu.Unlock()
	if sEng == nil {
		return nil, errors.New("engine sinkronisasi belum siap")
	}

	a.syncMu.Lock()
	defer a.syncMu.Unlock()
	err := sEng.RunSync(a.ctx)
	a.syncMu.Lock()
	a.syncRunErr, a.syncLastOK, a.syncLast = err, err == nil, time.Now().Format(time.DateTime)
	a.syncMu.Unlock()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"ran": true, "msg": "sinkronisasi selesai"}, nil
}

// SyncQueueCount menghitung operasi pending di antrean sinkronisasi lokal.
func (a *App) SyncQueueCount() int64 {
	a.mu.Lock()
	eng := a.eng
	a.mu.Unlock()
	if eng == nil {
		return 0
	}
	var n int64
	_ = eng.DB().QueryRow(`SELECT COUNT(*) FROM sync_queue WHERE status='pending'`).Scan(&n)
	return n
}

// GetSettings mengembalikan pengaturan non-rahasia (untuk halaman Pengaturan).
func (a *App) GetSettings() map[string]string {
	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()
	return map[string]string{
		"APP_NAME":       cfg.AppName,
		"APP_URL":        cfg.AppURL,
		"APP_TIMEZONE":   cfg.Timezone,
		"SIMARC_WINDOW":  cfg.Window,
		"DB_HOST":        cfg.DBHost,
		"DB_PORT":        fmt.Sprint(cfg.DBPort),
		"DB_DATABASE":    cfg.DBName,
		"DB_USERNAME":    cfg.DBUser,
		"DB_TLS":         cfg.DBTLS,
		"SYNC_ENABLED":   boolStr(a.syncEnabled),
		"SYNC_NODE_ID":   cfg.SyncNodeID,
		"SYNC_INTERVAL":  fmt.Sprint(int(cfg.SyncInterval.Minutes())),
		"SYNC_HUB_URL":   cfg.SyncHubURL,
		"ENV_FILE":       envFileShown(),
	}
}

var allowedSettingKeys = []string{
	"APP_NAME", "APP_URL", "APP_TIMEZONE", "SIMARC_WINDOW",
	"DB_HOST", "DB_PORT", "DB_DATABASE", "DB_USERNAME", "DB_PASSWORD", "DB_TLS",
	"SESSION_KEY",
	"SYNC_ENABLED", "SYNC_NODE_ID", "SYNC_INTERVAL_MINUTES", "SYNC_HUB_URL", "SYNC_HUB_TOKEN",
}

// SaveSettings menulis perubahan pengguna ke .env (UpdateEnv mempertahankan
// komentar), lalu me-reload config; bila DSN berubah, koneksi diputar ulang.
func (a *App) SaveSettings(values map[string]string) (map[string]string, error) {
	clean := map[string]string{}
	for _, k := range allowedSettingKeys {
		if v, ok := values[k]; ok {
			clean[k] = strings.TrimSpace(v)
		}
	}
	if v, ok := clean["SYNC_ENABLED"]; ok {
		clean["SYNC_ENABLED"] = boolStr(v == "true" || v == "1" || v == "yes")
	}
	if err := config.UpdateEnv(clean); err != nil {
		return nil, fmt.Errorf("menulis .env: %w", err)
	}

	// Reload file .env sebagai sumber kebenaran.
	c, err := config.Load()
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	oldDSN := a.cfg.DSN()
	a.cfg, a.syncEnabled = c, c.SyncEnabled
	a.mu.Unlock()

	if c.DSN() != oldDSN {
		a.mu.Lock()
		if eng, err := database.NewLocalEngine(c.DSN()); err == nil {
			if a.eng != nil {
				_ = a.eng.Close()
			}
			a.eng, a.engErr = eng, nil
			if err := eng.Migrate(fs.FS(a.migrations)); err != nil {
				a.engErr = fmt.Errorf("migrasi skema: %w", err)
			} else {
				a.initSync()
				a.startSync.Do(a.syncLoop)
			}
		} else {
			a.eng, a.engErr = nil, err
		}
		a.mu.Unlock()
	}

	msg := "pengaturan disimpan"
	if c.DSN() != oldDSN {
		msg += " — koneksi database diperbarui"
	}
	return map[string]string{"msg": msg}, nil
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// envFileShown menampilkan path .env yang aktif (untuk UI, tanpa rahasia).
func envFileShown() string { return config.EnvFilePath() }

// ── Dashboard (data asli, tanpa dummy) ──────────────────────────────────

// DashboardPoint satu titik seri per hari.
type DashboardPoint struct {
	Day   string `json:"day"`
	Arsip int64  `json:"arsip"`
	Berkas int64 `json:"berkas"`
}

// DashboardOverview ringkasan KPI + grafik 14 hari.
type DashboardOverview struct {
	TotalArsip       int64            `json:"total_arsip"`
	TotalPemberkasan int64            `json:"total_pemberkasan"`
	TotalUsers       int64            `json:"total_users"`
	SyncQueuePending int64            `json:"sync_queue_pending"`
	LastSync         string           `json:"last_sync"`
	Series           []DashboardPoint `json:"series"`
}

// GetOverview mengambil semua angka & grafik langsung dari database lokal.
func (a *App) GetOverview() (DashboardOverview, error) {
	a.mu.Lock()
	eng, lastSync := a.eng, a.syncLast
	a.mu.Unlock()
	if eng == nil {
		return DashboardOverview{}, errNotReady
	}
	db := eng.DB()
	ov := DashboardOverview{LastSync: lastSync, Series: []DashboardPoint{}}

	queries := []struct {
		sql  string
		dest *int64
	}{
		{`SELECT COUNT(*) FROM arsip WHERE deleted_at IS NULL`, &ov.TotalArsip},
		{`SELECT COUNT(*) FROM pemberkasan WHERE deleted_at IS NULL`, &ov.TotalPemberkasan},
		{`SELECT COUNT(*) FROM users WHERE deleted_at IS NULL AND active=1`, &ov.TotalUsers},
		{`SELECT COUNT(*) FROM sync_queue WHERE status='pending'`, &ov.SyncQueuePending},
	}
	for _, q := range queries {
		if err := db.QueryRow(q.sql).Scan(q.dest); err != nil {
			return DashboardOverview{}, fmt.Errorf("overview: %w", err)
		}
	}

	rows, err := db.Query(`
        SELECT day, SUM(arsip) AS arsip, SUM(berkas) AS berkas FROM (
            SELECT DATE(created_at) AS day, COUNT(*) AS arsip, 0 AS berkas
              FROM arsip WHERE created_at >= CURDATE() - INTERVAL 13 DAY GROUP BY day
            UNION ALL
            SELECT DATE(created_at), 0, COUNT(*)
              FROM pemberkasan WHERE created_at >= CURDATE() - INTERVAL 13 DAY GROUP BY day
        ) t GROUP BY day ORDER BY day`)
	if err != nil {
		return DashboardOverview{}, fmt.Errorf("grafik arsip: %w", err)
	}
	defer rows.Close()

	dayMap := make(map[string]DashboardPoint)
	for rows.Next() {
		var d string
		var p DashboardPoint
		if err := rows.Scan(&d, &p.Arsip, &p.Berkas); err != nil {
			return DashboardOverview{}, err
		}
		dayMap[d] = p
	}
	for i := 13; i >= 0; i-- {
		day := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		if p, ok := dayMap[day]; ok {
			ov.Series = append(ov.Series, p)
		} else {
			ov.Series = append(ov.Series, DashboardPoint{Day: day, Arsip: 0, Berkas: 0})
		}
	}
	return ov, nil
}

// ListMigrations mengembalikan daftar migrasi ter-terapkan (audit/ UI).
func (a *App) ListMigrations() []string {
	a.mu.Lock()
	eng := a.eng
	a.mu.Unlock()
	if eng == nil {
		return []string{}
	}
	names, err := eng.AppliedMigrations()
	if err != nil {
		return []string{}
	}
	return names
}

// ── Backup database lokal ───────────────────────────────────────────────

// BackupInfo satu berkas cadangan.
type BackupInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Created string `json:"created"`
}

// backupDir dari EXE.
func (a *App) backupDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "./storage/backups"
	}
	return filepath.Join(filepath.Dir(exe), "storage", "backups")
}

// CreateBackupNow dump seluruh tabel MySQL lokal ke berkas .sql di
// storage/backups — dibaca lewat SQL standar oleh backend (tanpa tool luar).
func (a *App) CreateBackupNow() (map[string]interface{}, error) {
	a.mu.Lock()
	eng := a.eng
	a.mu.Unlock()
	if eng == nil {
		return nil, errNotReady
	}

	dir := a.backupDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("buat folder backup: %w", err)
	}
	name := fmt.Sprintf("simarc-%s.sql", time.Now().Format("20060102-150405"))
	target := filepath.Join(dir, name)

	f, err := os.Create(target)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	defer w.Flush()
	dbn := a.cfg.DBName
	fmt.Fprintf(w, "-- SIMARC backup\n-- database: %s\n-- created: %s\n\n", dbn, time.Now().Format(time.RFC3339))

	var tables []string
	rows, err := eng.DB().Query(`SELECT table_name FROM information_schema.tables WHERE table_schema=? ORDER BY table_name`, dbn)
	if err != nil {
		return nil, fmt.Errorf("baca daftar tabel: %w", err)
	}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err == nil {
			tables = append(tables, t)
		}
	}
	rows.Close()

	for _, t := range tables {
		var ddl string
		if err := eng.DB().QueryRow(`SELECT IFNULL(GROUP_CONCAT(column_type SEPARATOR ''), '') FROM information_schema.columns WHERE table_schema=? AND table_name=?`, dbn, t).Scan(&ddl); err != nil {
			continue
		}
		_ = ddl // detail schema dump tidak penting untuk restore minimal
		fmt.Fprintf(w, "\n-- tabel %s (backup seluruh baris)\n", t)
		dataRows, err := eng.DB().Query(fmt.Sprintf("SELECT * FROM `%s`", t))
		if err != nil {
			fmt.Fprintf(w, "-- gagal membaca: %v\n", err)
			continue
		}
		defer dataRows.Close()
		cols, _ := dataRows.Columns()
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		for dataRows.Next() {
			if err := dataRows.Scan(ptrs...); err != nil {
				continue
			}
			parts := make([]string, len(cols))
			for i, v := range vals {
				switch d := v.(type) {
				case nil:
					parts[i] = "NULL"
				case []byte:
					parts[i] = "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\n", `\n`, "\r", `\r`).Replace(string(d)) + "'"
				case time.Time:
					parts[i] = "'" + d.Format("2006-01-02 15:04:05") + "'"
				default:
					parts[i] = fmt.Sprintf("%v", d)
				}
			}
			fmt.Fprintf(w, "INSERT INTO `%s`(%s) VALUES(%s);\n", t,
				strings.Join(cols, ", "), strings.Join(parts, ", "))
		}
	}

	if st, err := os.Stat(target); err == nil {
		return map[string]interface{}{"name": name, "path": target, "size": st.Size()}, nil
	}
	return nil, errors.New("berkas backup tidak ditemukan setelah dibuat")
}

// ListBackups daftar berkas cadangan di storage/backups.
func (a *App) ListBackups() []BackupInfo {
	dir := a.backupDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []BackupInfo{}
	}
	out := []BackupInfo{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, BackupInfo{
			Name:    e.Name(),
			Path:    filepath.Join(dir, e.Name()),
			Size:    info.Size(),
			Created: info.ModTime().Format(time.DateTime),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out
}
