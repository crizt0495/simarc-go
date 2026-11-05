// Package config memuat seluruh pengaturan aplikasi desktop SIMARC.
// Sumber nilai: process environment (dengan prioritas) lalu file .env
// di samping executable (dimuat oleh LoadFrom dari package main).
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config menyimpan semua setting dari environment (.env).
// Nama field mengikuti nama variabel .env agar mudah dipetakan.
type Config struct {
	AppName  string        // APP_NAME
	AppURL   string        // APP_URL
	Port     int           // APP_PORT (dipakai hanya bila fitur HTTP diaktifkan)
	Debug    bool          // APP_DEBUG
	Timezone string        // APP_TIMEZONE
	Window   string        // SIMARC_WINDOW: normal|maximized|fullscreen|kiosk

	// MySQL lokal (DB_*)
	DBHost string
	DBPort int
	DBUser string
	DBPass string
	DBName string
	DBTLS  string // dinormalisasi lewat NormalizeMySQLTLS

	// Sinkronisasi ke hub (SYNC_*)
	SyncEnabled  bool
	SyncNodeID   string
	SyncInterval time.Duration // SYNC_INTERVAL_MINUTES
	SyncHubURL   string        // SYNC_HUB_URL (basis URL aplikasi hub)
	SyncHubToken string        // SYNC_HUB_TOKEN
}

// App adalah konfigurasi aktif (dipakai untuk pengujian dan akses nyaman).
var App *Config

// Load membaca environment dan mengembalikan konfigurasi final.
func Load() (*Config, error) {
	c := &Config{
		AppName:  get("APP_NAME", "SIMARC"),
		AppURL:   get("APP_URL", "http://localhost:8080"),
		Port:     getInt("APP_PORT", 8080),
		Debug:    getBool("APP_DEBUG", false),
		Timezone: get("APP_TIMEZONE", "Asia/Jakarta"),
		Window:   normalizeWindow(get("SIMARC_WINDOW", "fullscreen")),

		DBHost: get("DB_HOST", "127.0.0.1"),
		DBPort: getInt("DB_PORT", 3306),
		DBUser: get("DB_USERNAME", get("DB_USER", "simarc")),
		DBPass: os.Getenv("DB_PASSWORD"),
		DBName: get("DB_DATABASE", get("DB_NAME", "simarc")),
		DBTLS:  NormalizeMySQLTLS(get("DB_TLS", "preferred")),

		SyncEnabled: getBool("SYNC_ENABLED", false),
		SyncNodeID:  get("SYNC_NODE_ID", ""),
		SyncInterval: time.Duration(getInt("SYNC_INTERVAL_MINUTES", 15)) * time.Minute,
		SyncHubURL:   strings.TrimRight(get("SYNC_HUB_URL", ""), "/"),
		SyncHubToken: os.Getenv("SYNC_HUB_TOKEN"),
	}

	// DATABASE_URL (format mysql://user:pass@host:port/db?ssl-mode=...)
	// menimpa nilai field DB_* bila diisi.
	if raw := get("DATABASE_URL", ""); raw != "" {
		if u, err := url.Parse(raw); err == nil && u.Scheme == "mysql" {
			c.DBHost = u.Hostname()
			if p := u.Port(); p != "" {
				c.DBPort = atoi(p, c.DBPort)
			}
			c.DBName = strings.TrimPrefix(u.Path, "/")
			c.DBUser = u.User.Username()
			c.DBPass, _ = u.User.Password()
			c.DBTLS = NormalizeMySQLTLS(u.Query().Get("ssl-mode"))
		}
	}

	// NODE_NAME fallback: pakai hostname komputer bila SYNC_NODE_ID kosong.
	if c.SyncNodeID == "" {
		if h, err := os.Hostname(); err == nil && h != "" {
			c.SyncNodeID = h
		} else {
			c.SyncNodeID = "local"
		}
	}

	App = c
	return c, nil
}

// DSN membangun DSN MySQL untuk koneksi database lokal.
// multiStatements=true agar migrator bisa mengeksekusi file SQL multi-statement.
func (c *Config) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=%s&multiStatements=true&tls=%s",
		c.DBUser, c.DBPass, c.DBHost, c.DBPort, c.DBName,
		url.QueryEscape(mapTz(c.Timezone)), c.DBTLS)
}

// HubPullURL mengembalikan endpoint pull hub untuk sebuah tabel.
func (c *Config) HubPullURL(table, cursor string) string {
	return fmt.Sprintf("%s/api/v1/sync/pull?table=%s&cursor=%s", c.SyncHubURL,
		url.QueryEscape(table), url.QueryEscape(cursor))
}

// HubPushURL mengembalikan endpoint push hub.
func (c *Config) HubPushURL() string { return c.SyncHubURL + "/api/v1/sync/push" }

// normalizeWindow memastikan nilai SIMARC_WINDOW valid
// (fallback ke fullscreen bila nilai tidak dikenal).
func normalizeWindow(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "normal":
		return "normal"
	case "maximized":
		return "maximized"
	case "kiosk":
		return "kiosk"
	default:
		if strings.EqualFold(v, "fullscreen") {
			return "fullscreen"
		}
		if strings.TrimSpace(v) == "" {
			return "fullscreen"
		}
		// Nilai asing → fullscreen (default aman).
		return "fullscreen"
	}
}

func mapTz(tz string) string {
	switch tz {
	case "Asia/Jakarta", "Asia/Pontianak", "Asia/Makassar", "Asia/Jayapura", "UTC":
		return tz
	default:
		return "UTC"
	}
}

func get(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getBool(key string, fb bool) bool {
	v := get(key, "")
	if v == "" {
		return fb
	}
	b, err := strconv.ParseBool(strings.ToLower(v))
	if err != nil {
		return fb
	}
	return b
}

func getInt(key string, fb int) int {
	return atoi(os.Getenv(key), fb)
}

func atoi(s string, fb int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return fb
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fb
	}
	return n
}
