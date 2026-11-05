package config

import (
	"os"
	"testing"
)

var offsiteKeys = []string{
	"OFFSITE_ENABLED", "OFFSITE_HOUR", "OFFSITE_MINUTE", "OFFSITE_BACKUP_DB", "OFFSITE_BACKUP_FILES",
	"OFFSITE_DB_HOST", "OFFSITE_DB_PORT", "OFFSITE_DB_NAME", "OFFSITE_DB_USER", "OFFSITE_DB_PASS",
	"OFFSITE_DB_TLS", "OFFSITE_DB_CA_FILE",
	"OFFSITE_S3_ENDPOINT", "OFFSITE_S3_REGION", "OFFSITE_S3_BUCKET", "OFFSITE_S3_ACCESS_KEY",
	"OFFSITE_S3_SECRET_KEY", "OFFSITE_S3_PREFIX", "OFFSITE_S3_PATH_STYLE",
	"OFFSITE_FILES_MODE", "OFFSITE_RCLONE_REMOTE", "OFFSITE_RCLONE_PATH",
}

func clearOffsiteEnv() {
	for _, k := range offsiteKeys {
		os.Unsetenv(k)
	}
}

func TestLoadOffsiteDefaults(t *testing.T) {
	clearOffsiteEnv()
	t.Cleanup(clearOffsiteEnv)

	c := LoadOffsite()
	if c.Enabled {
		t.Fatal("offsite should be disabled by default")
	}
	if c.Hour != 2 || c.Minute != 0 {
		t.Fatalf("unexpected default schedule: %02d:%02d", c.Hour, c.Minute)
	}
	if !c.BackupDB || !c.BackupFiles {
		t.Fatal("db and files backup should default to enabled")
	}
	if c.DBPort != "3306" {
		t.Fatalf("unexpected default port: %q", c.DBPort)
	}
	if c.DBTLS != "required" {
		t.Fatalf("unexpected default TLS: %q", c.DBTLS)
	}
	if c.S3Region != "auto" || c.S3Prefix != "simarc" {
		t.Fatalf("unexpected S3 defaults: region=%q prefix=%q", c.S3Region, c.S3Prefix)
	}
	if c.DBReady() || c.S3Ready() {
		t.Fatal("nothing should be ready without credentials")
	}
}

func TestLoadOffsiteParsingAndClamp(t *testing.T) {
	clearOffsiteEnv()
	t.Cleanup(clearOffsiteEnv)

	os.Setenv("OFFSITE_ENABLED", "true")
	os.Setenv("OFFSITE_HOUR", "99")
	os.Setenv("OFFSITE_MINUTE", "-5")
	os.Setenv("OFFSITE_DB_HOST", "db.example.com")
	os.Setenv("OFFSITE_DB_PORT", "3306")
	os.Setenv("OFFSITE_DB_NAME", "simarc")
	os.Setenv("OFFSITE_DB_USER", "avnadmin")
	os.Setenv("OFFSITE_S3_ENDPOINT", "http://localhost:9000")
	os.Setenv("OFFSITE_S3_BUCKET", "simarc-backup")
	os.Setenv("OFFSITE_S3_ACCESS_KEY", "key")
	os.Setenv("OFFSITE_S3_SECRET_KEY", "secret")

	c := LoadOffsite()
	if !c.Enabled {
		t.Fatal("expected enabled")
	}
	if c.Hour != 2 || c.Minute != 0 {
		t.Fatalf("schedule not clamped: %02d:%02d", c.Hour, c.Minute)
	}
	if !c.DBReady() {
		t.Fatal("cloud DB should be ready")
	}
	if !c.S3Ready() {
		t.Fatal("S3 should be ready")
	}
	if !c.HasWork() {
		t.Fatal("expected work to do")
	}
	if c.S3Scheme() != "http" {
		t.Fatalf("expected http scheme, got %q", c.S3Scheme())
	}
	if c.S3Host() != "localhost:9000" {
		t.Fatalf("unexpected S3 host: %q", c.S3Host())
	}
}

func TestFileBackendSelection(t *testing.T) {
	clearOffsiteEnv()
	t.Cleanup(clearOffsiteEnv)

	// Nothing configured → no backend.
	if got := LoadOffsite().FileBackend(); got != "" {
		t.Fatalf("expected no backend, got %q", got)
	}
	if LoadOffsite().FilesReady() {
		t.Fatal("files should not be ready with no backend")
	}

	// Only rclone remote → rclone backend.
	os.Setenv("OFFSITE_RCLONE_REMOTE", "gdrive")
	if got := LoadOffsite().FileBackend(); got != "rclone" {
		t.Fatalf("expected rclone backend, got %q", got)
	}
	if !LoadOffsite().FilesReady() {
		t.Fatal("files should be ready with an rclone remote")
	}
	if got := LoadOffsite().RclonePath; got != "simarc-backup" {
		t.Fatalf("unexpected default rclone path: %q", got)
	}

	// S3 fully configured → auto mode prefers S3.
	os.Setenv("OFFSITE_S3_ENDPOINT", "https://acc.r2.cloudflarestorage.com")
	os.Setenv("OFFSITE_S3_BUCKET", "simarc-backup")
	os.Setenv("OFFSITE_S3_ACCESS_KEY", "key")
	os.Setenv("OFFSITE_S3_SECRET_KEY", "secret")
	if got := LoadOffsite().FileBackend(); got != "s3" {
		t.Fatalf("auto mode should prefer s3, got %q", got)
	}

	// Explicit rclone mode wins even when S3 is configured.
	os.Setenv("OFFSITE_FILES_MODE", "rclone")
	if got := LoadOffsite().FileBackend(); got != "rclone" {
		t.Fatalf("explicit rclone mode ignored, got %q", got)
	}

	// HasWork is true with only rclone files (no DB).
	os.Unsetenv("OFFSITE_FILES_MODE")
	os.Setenv("OFFSITE_BACKUP_DB", "false")
	os.Setenv("OFFSITE_BACKUP_FILES", "true")
	if !LoadOffsite().HasWork() {
		t.Fatal("rclone-only configuration should still count as work")
	}
}
