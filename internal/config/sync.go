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
	return c
}

// HubReady reports whether the hub database settings are complete enough to
// attempt a connection.
func (c SyncConfig) HubReady() bool {
	return c.HubDBHost != "" && c.HubDBName != "" && c.HubDBUser != ""
}

// Active reports whether synchronisation should run at all.
func (c SyncConfig) Active() bool {
	return c.Enabled && c.HubReady()
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
