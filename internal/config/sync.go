package config

import (
	"os"
	"strings"
	"time"
)

// SyncConfig holds the settings for multi-computer (offline-first) database
// synchronisation.
//
// Model: every computer runs its own MySQL database (so it keeps working with
// no network at all). One of the databases — or a cloud MySQL such as the
// existing Aiven mirror — acts as the "hub". Each computer periodically (and on
// demand) exchanges its changes with the hub using a last-write-wins merge.
//
// The local database is always the machine's working copy and remains usable
// offline; the hub is only contacted when it is reachable.
type SyncConfig struct {
	Enabled bool
	// NodeID uniquely identifies this computer among all synced nodes.
	NodeID string
	// Interval in minutes between automatic background syncs (0 disables the
	// scheduler, leaving only manual/on-demand syncs).
	IntervalMinutes int
	// OverlapSeconds re-reads a small window before the stored watermark so
	// clock skew between computers cannot drop a change. Re-sends are harmless
	// thanks to last-write-wins.
	OverlapSeconds int
	// BatchSize is the maximum number of rows sent per INSERT statement.
	BatchSize int

	// Hub MySQL connection (the shared merge point).
	HubDBHost string
	HubDBPort string
	HubDBName string
	HubDBUser string
	HubDBPass string
	HubDBTLS  string

	// HubDriver menentukan mesin database hub: "mysql" (default: Aiven, MySQL
	// kantor, atau komputer server) atau "sqlite" (berkas pada komputer yang
	// menjadi titik temu — berguna untuk instalasi tanpa server database).
	HubDriver string
	// HubDBPath adalah lokasi berkas SQLite bila HubDriver = "sqlite".
	HubDBPath string

	// Hub HTTP (archive-file transfer). The hub application serves
	// token-protected endpoints so nodes can push/pull files under
	// storage/app/uploads. HubURL is the base URL of the hub app, e.g.
	// http://192.168.1.86:8080.
	HubURL       string
	HubToken     string
	FilesEnabled bool
	FilesMaxMB   int
	// FilesExclude lists uploads sub-directories that are machine-local and
	// must never be copied between computers (temporary working dirs).
	FilesExclude []string
}

// LoadSync reads the synchronisation settings from the environment. Like the
// offsite settings it is cheap and always reflects the current .env, so a save
// is picked up without restarting the server.
func LoadSync() SyncConfig {
	c := SyncConfig{
		Enabled:         getBoolEnv("SYNC_ENABLED", false),
		NodeID:          strings.TrimSpace(os.Getenv("SYNC_NODE_ID")),
		IntervalMinutes: getIntEnv("SYNC_INTERVAL_MINUTES", 15),
		OverlapSeconds:  getIntEnv("SYNC_OVERLAP_SECONDS", 30),
		BatchSize:       getIntEnv("SYNC_BATCH_SIZE", 200),

		HubDBHost: strings.TrimSpace(os.Getenv("SYNC_HUB_DB_HOST")),
		HubDBPort: getEnv("SYNC_HUB_DB_PORT", "3306"),
		HubDBName: strings.TrimSpace(os.Getenv("SYNC_HUB_DB_NAME")),
		HubDBUser: strings.TrimSpace(os.Getenv("SYNC_HUB_DB_USER")),
		HubDBPass: os.Getenv("SYNC_HUB_DB_PASS"),
		HubDBTLS:  getEnv("SYNC_HUB_DB_TLS", "preferred"),

		HubDriver: strings.ToLower(strings.TrimSpace(getEnv("SYNC_HUB_DRIVER", "mysql"))),
		HubDBPath: strings.TrimSpace(os.Getenv("SYNC_HUB_DB_PATH")),

		HubURL:       strings.TrimRight(strings.TrimSpace(os.Getenv("SYNC_HUB_URL")), "/"),
		HubToken:     strings.TrimSpace(os.Getenv("SYNC_HUB_TOKEN")),
		FilesEnabled: getBoolEnv("SYNC_FILES_ENABLED", true),
		FilesMaxMB:   getIntEnv("SYNC_FILES_MAX_MB", 128),
		FilesExclude: splitList(getEnv("SYNC_FILES_EXCLUDE", "ocr-temp,imports,restore")),
	}

	if c.NodeID == "" {
		c.NodeID = defaultNodeID()
	}
	if c.IntervalMinutes < 0 {
		c.IntervalMinutes = 0
	}
	if c.OverlapSeconds < 0 {
		c.OverlapSeconds = 0
	}
	if c.BatchSize < 1 {
		c.BatchSize = 200
	}
	if c.FilesMaxMB < 1 {
		c.FilesMaxMB = 128
	}
	return c
}

// HubReady reports whether the hub database settings are complete enough to
// attempt a connection.
func (c SyncConfig) HubReady() bool {
	if c.HubIsSQLite() {
		return c.HubDBPath != ""
	}
	return c.HubDBHost != "" && c.HubDBName != "" && c.HubDBUser != ""
}

// HubIsSQLite reports whether the hub runs on SQLite instead of MySQL.
func (c SyncConfig) HubIsSQLite() bool { return c.HubDriver == "sqlite" }

// Active reports whether synchronisation should run at all.
func (c SyncConfig) Active() bool {
	return c.Enabled && c.HubReady()
}

// FilesReady reports whether archive-file transfer is configured. The hub app
// URL and the shared token are both required.
func (c SyncConfig) FilesReady() bool {
	return c.HubURL != "" && c.HubToken != ""
}

// FilesActive reports whether archive-file transfer should run alongside the
// database sync.
func (c SyncConfig) FilesActive() bool {
	return c.Active() && c.FilesEnabled && c.FilesReady()
}

// FilesMaxBytes is the largest file (in bytes) that will be copied.
func (c SyncConfig) FilesMaxBytes() int64 {
	return int64(c.FilesMaxMB) * 1024 * 1024
}

// ExcludesUpload reports whether a slash-separated relative path is inside one
// of the machine-local uploads sub-directories that must not be synced.
func (c SyncConfig) ExcludesUpload(rel string) bool {
	rel = strings.TrimPrefix(strings.ReplaceAll(rel, "\\", "/"), "/")
	for _, ex := range c.FilesExclude {
		ex = strings.Trim(strings.TrimSpace(ex), "/")
		if ex == "" {
			continue
		}
		if rel == ex || strings.HasPrefix(rel, ex+"/") {
			return true
		}
	}
	return false
}

// splitList parses a comma/space separated list, dropping empty entries.
func splitList(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == ';' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// Overlap returns the safety window used when reading changes.
func (c SyncConfig) Overlap() time.Duration {
	return time.Duration(c.OverlapSeconds) * time.Second
}

// defaultNodeID derives a stable, human-readable node identifier from the host
// name. It can always be overridden with SYNC_NODE_ID when two machines share a
// host name (rare) or when a friendlier label is preferred.
func defaultNodeID() string {
	h, err := os.Hostname()
	if err != nil || strings.TrimSpace(h) == "" {
		return "node-unknown"
	}
	h = strings.ToLower(strings.TrimSpace(h))
	// Keep it filename/column friendly.
	repl := func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}
	return strings.Map(repl, h)
}
