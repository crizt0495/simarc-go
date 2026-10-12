package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"arsippro/internal/config"
)

func TestRcloneDest(t *testing.T) {
	if got := rcloneDest(config.OffsiteConfig{RcloneRemote: "gdrive", RclonePath: "/simarc-backup/"}); got != "gdrive:simarc-backup" {
		t.Fatalf("unexpected dest: %q", got)
	}
	if got := rcloneDest(config.OffsiteConfig{RcloneRemote: "b2:", RclonePath: ""}); got != "b2:" {
		t.Fatalf("unexpected dest: %q", got)
	}
}

func TestDirStats(t *testing.T) {
	dir := t.TempDir()
	if c, b, err := dirStats(dir); err != nil || c != 0 || b != 0 {
		t.Fatalf("empty dir: c=%d b=%d err=%v", c, b, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("world!"), 0644); err != nil {
		t.Fatal(err)
	}
	c, b, err := dirStats(dir)
	if err != nil || c != 2 || b != 11 {
		t.Fatalf("expected 2 files/11 bytes, got c=%d b=%d err=%v", c, b, err)
	}
	if c, b, err := dirStats(filepath.Join(dir, "missing")); err != nil || c != 0 || b != 0 {
		t.Fatalf("missing dir: c=%d b=%d err=%v", c, b, err)
	}
}

func TestNormalizeDumpForCloudUUIDDefault(t *testing.T) {
	in := []byte("`id` varchar(36) NOT NULL DEFAULT uuid(),\n`x` char(36) NOT NULL DEFAULT UUID(),\n")
	out := string(normalizeDumpForCloud(in))
	if strings.Count(out, "DEFAULT (uuid())") != 2 {
		t.Fatalf("expected both defaults normalized, got:\n%s", out)
	}
	if strings.Contains(strings.ToLower(out), "default uuid()") {
		t.Fatalf("unconverted default remains:\n%s", out)
	}
}

func TestNormalizeDumpForCloudNoop(t *testing.T) {
	in := []byte("CREATE TABLE t (id varchar(36) NOT NULL, PRIMARY KEY (id));\n")
	if string(normalizeDumpForCloud(in)) != string(in) {
		t.Fatal("content without uuid() should be returned unchanged")
	}
}

func TestNextOffsiteRunBeforeTime(t *testing.T) {
	cfg := config.OffsiteConfig{Hour: 2, Minute: 30}
	now := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	got := nextOffsiteRun(now, cfg)
	want := time.Date(2026, 1, 1, 2, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestNextOffsiteRunAfterTimeRollsToTomorrow(t *testing.T) {
	cfg := config.OffsiteConfig{Hour: 2, Minute: 30}
	now := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
	got := nextOffsiteRun(now, cfg)
	want := time.Date(2026, 1, 2, 2, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
