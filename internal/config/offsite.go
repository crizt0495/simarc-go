package config

import (
	"os"
	"strconv"
	"strings"
)

// OffsiteConfig holds the settings for the offsite (cloud) backup target:
// a cloud MySQL/MariaDB database (e.g. Aiven for MySQL, TiDB Cloud) plus an
// S3-compatible object-storage bucket (e.g. Cloudflare R2, Backblaze B2) used
// to mirror the archive files that live on the server's disk.
//
// The local database remains the source of truth; the cloud targets are only
// written to by the scheduled offsite backup job (see services.RunOffsiteNow).
type OffsiteConfig struct {
	Enabled     bool
	Hour        int
	Minute      int
	BackupDB    bool
	BackupFiles bool

	// Cloud MySQL (MySQL/MariaDB wire-compatible)
	DBHost   string
	DBPort   string
	DBName   string
	DBUser   string
	DBPass   string
	DBTLS    string // disabled | preferred | required | verify
	DBCAFile string
	// IncludeLegacy, when true, mirrors *every* table so the cloud copy is a
	// true 1:1 duplicate. When false (default) the large/unused legacy tables
	// (Laravel Telescope, caches, duplicate plural tables) are skipped.
	IncludeLegacy bool

	// S3-compatible object storage (Cloudflare R2 / Backblaze B2 / MinIO)
	S3Endpoint  string
	S3Region    string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string
	S3Prefix    string
	S3PathStyle bool

	// File backup backend selection + rclone remote (e.g. Google Drive).
	// FilesMode is "auto" | "s3" | "rclone". In "auto" mode S3 is preferred
	// when fully configured, otherwise an existing rclone remote is used.
	FilesMode    string
	RcloneRemote string
	RclonePath   string
}

// LoadOffsite reads the offsite backup settings from the environment.
// It is cheap and always reflects the current .env, so it can be called
// per request / per scheduler tick (no restart needed after a save).
func LoadOffsite() OffsiteConfig {
	c := OffsiteConfig{
		Enabled:     getBoolEnv("OFFSITE_ENABLED", false),
		Hour:        getIntEnv("OFFSITE_HOUR", 2),
		Minute:      getIntEnv("OFFSITE_MINUTE", 0),
		BackupDB:    getBoolEnv("OFFSITE_BACKUP_DB", true),
		BackupFiles: getBoolEnv("OFFSITE_BACKUP_FILES", true),

		DBHost:   os.Getenv("OFFSITE_DB_HOST"),
		DBPort:   getEnv("OFFSITE_DB_PORT", "3306"),
		DBName:   os.Getenv("OFFSITE_DB_NAME"),
		DBUser:   os.Getenv("OFFSITE_DB_USER"),
		DBPass:   os.Getenv("OFFSITE_DB_PASS"),
		DBTLS:    getEnv("OFFSITE_DB_TLS", "required"),
		DBCAFile: os.Getenv("OFFSITE_DB_CA_FILE"),

		IncludeLegacy: getBoolEnv("OFFSITE_DB_INCLUDE_LEGACY", false),

		S3Endpoint:  os.Getenv("OFFSITE_S3_ENDPOINT"),
		S3Region:    getEnv("OFFSITE_S3_REGION", "auto"),
		S3Bucket:    os.Getenv("OFFSITE_S3_BUCKET"),
		S3AccessKey: os.Getenv("OFFSITE_S3_ACCESS_KEY"),
		S3SecretKey: os.Getenv("OFFSITE_S3_SECRET_KEY"),
		S3Prefix:    getEnv("OFFSITE_S3_PREFIX", "simarc"),
		S3PathStyle: getBoolEnv("OFFSITE_S3_PATH_STYLE", true),

		FilesMode:    getEnv("OFFSITE_FILES_MODE", "auto"),
		RcloneRemote: os.Getenv("OFFSITE_RCLONE_REMOTE"),
		RclonePath:   getEnv("OFFSITE_RCLONE_PATH", "simarc-backup"),
	}

	// Clamp the schedule to a valid time of day.
	if c.Hour < 0 || c.Hour > 23 {
		c.Hour = 2
	}
	if c.Minute < 0 || c.Minute > 59 {
		c.Minute = 0
	}
	return c
}

// DBReady reports whether the cloud database settings are complete enough to
// attempt a connection.
func (c OffsiteConfig) DBReady() bool {
	return c.DBHost != "" && c.DBName != "" && c.DBUser != ""
}

// S3Ready reports whether the object-storage settings are complete enough to
// attempt a connection.
func (c OffsiteConfig) S3Ready() bool {
	return c.S3Endpoint != "" && c.S3Bucket != "" && c.S3AccessKey != "" && c.S3SecretKey != ""
}

// RcloneReady reports whether an rclone remote has been configured for file
// backup (the remote itself is authenticated outside the app via
// `rclone config`, e.g. a Google Drive account).
func (c OffsiteConfig) RcloneReady() bool {
	return strings.TrimSpace(c.RcloneRemote) != ""
}

// FileBackend returns which file-storage backend a run should use: "s3",
// "rclone", or "" when none is configured. In the default "auto" mode S3 wins
// when it is fully configured; otherwise an existing rclone remote is used.
func (c OffsiteConfig) FileBackend() string {
	switch strings.ToLower(strings.TrimSpace(c.FilesMode)) {
	case "s3":
		if c.S3Ready() {
			return "s3"
		}
		return ""
	case "rclone":
		if c.RcloneReady() {
			return "rclone"
		}
		return ""
	default: // "auto" or blank
		if c.S3Ready() {
			return "s3"
		}
		if c.RcloneReady() {
			return "rclone"
		}
		return ""
	}
}

// FilesReady reports whether the configured file-storage backend is usable.
func (c OffsiteConfig) FilesReady() bool {
	return c.FileBackend() != ""
}

// HasWork reports whether the enabled configuration has at least one usable
// target (cloud DB and/or file storage).
func (c OffsiteConfig) HasWork() bool {
	return (c.BackupDB && c.DBReady()) || (c.BackupFiles && c.FilesReady())
}

// S3Scheme returns the scheme declared in the endpoint, defaulting to https.
func (c OffsiteConfig) S3Scheme() string {
	switch {
	case len(c.S3Endpoint) >= 7 && c.S3Endpoint[:7] == "http://":
		return "http"
	case len(c.S3Endpoint) >= 8 && c.S3Endpoint[:8] == "https://":
		return "https"
	default:
		return "https"
	}
}

// S3Host returns the endpoint without its scheme, as required by minio-go.
func (c OffsiteConfig) S3Host() string {
	e := c.S3Endpoint
	e = trimPrefix(e, "https://")
	e = trimPrefix(e, "http://")
	return e
}

func trimPrefix(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}

func getBoolEnv(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "TRUE", "True", "on", "ON", "yes", "YES":
		return true
	case "0", "false", "FALSE", "False", "off", "OFF", "no", "NO":
		return false
	default:
		return fallback
	}
}

func getIntEnv(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
