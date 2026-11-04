package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config represents all settings loaded from environment variables (.env file).
type Config struct {
	AppName string
	AppURL  string
	AppPort int
	Debug   bool
	Timezone string

	// Desktop window mode: normal | maximized | fullscreen | kiosk
	WindowMode string

	// Client mode: client = use server; local = run own server+db
	ClientMode bool

	ServerURL string // SIMARC_SERVER_URL — required for client mode, nil for local

	// Local MySQL
	DBDriver   string // mysql | sqlite
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBDatabase string
	DBPath     string // local SQLite path if DB_DRIVER=sqlite

	// Sync hub DB (for SYNC_ENABLED)
	SyncEnabled bool
	SyncHubHost string
	SyncHubPort int
	SyncHubUser string
	SyncHubPass string

	SessionKey string
}

func Load() (*Config, error) {
	c := &Config{
		AppName:      get("APP_NAME", "SIMARC"),
		AppURL:       get("APP_URL", "http://localhost:8080"),
		AppPort:      getInt("APP_PORT", 8080),
		Debug:        getBool("APP_DEBUG", false),
		Timezone:     get("APP_TIMEZONE", "Asia/Jakarta"),
		WindowMode:   get("SIMARC_WINDOW", "fullscreen"),
		ClientMode:   os.Getenv("SIMARC_SERVER_URL") != "",
		DBDriver:     get("DB_DRIVER", "mysql"),
		DBHost:       get("DB_HOST", "127.0.0.1"),
		DBPort:       getInt("DB_PORT", 3306),
		DBUser:       get("DB_USER", "simarc"),
		DBPassword:   get("DB_PASSWORD", ""),
		DBDatabase:   get("DB_DATABASE", "simarc_local"),
		DBPath:       get("DB_PATH", "") + "/storage/simarc.db",
		SyncEnabled:  getBool("SYNC_ENABLED", false),
		SyncHubHost:  get("SYNC_HUB_DB_HOST", ""),
		SyncHubPort:  getInt("SYNC_HUB_DB_PORT", 3306),
		SyncHubUser:  get("SYNC_HUB_DB_USER", "simarc"),
	}
	if v := get("SESSION_KEY", ""); v != "" {
		c.SessionKey = v
	} else if v := os.Getenv("SESSION_KEY"); v != "" {
		c.SessionKey = v
	} else if v := get("SIMARC_SESSION_KEY", ""); v != "" {
		c.SessionKey = v
	}
	return c, nil
}

// DSN returns a MySQL datasource name.
func (c *Config) DSN() string {
	if c.DBDriver == "sqlite" && c.DBPath != "" {
		// SQLite uses just the file path (no DSN format).
		return c.DBPath
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=%s&multiStatements=true",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBDatabase, mapTz(c.Timezone))
	return dsn
}

// DSNLocal returns localhost DSN.
func (c *Config) DSNLocal() string {
	return fmt.Sprintf("127.0.0.1:%d/simarc_local?parseTime=true&loc=%s&multiStatements=true",
		c.DBPort, mapTz(c.Timezone))
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
	v := get(key, "")
	if v == "" {
		return fb
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fb
	}
	return n
}

func mapTz(tz string) string {
	switch tz {
	case "America/Jakarta":
		return "America/Jakarta"
	case "Asia/Jakarta":
		return "Asia/Jakarta"
	default:
		return "UTC"
	}
}
