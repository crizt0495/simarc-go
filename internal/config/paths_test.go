package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDataDirIsUserScoped(t *testing.T) {
	// An explicit override always wins, so tests and portable installs can
	// point somewhere predictable.
	t.Setenv("SIMARC_DATA_DIR", "/tmp/simarc-test")
	if got := DataDir(); got != "/tmp/simarc-test" {
		t.Errorf("DataDir() = %q, ingin /tmp/simarc-test", got)
	}

	t.Setenv("SIMARC_DATA_DIR", "")
	dir := DataDir()
	if dir == "" || dir == "." {
		t.Fatalf("DataDir() = %q, ingin sebuah direktori", dir)
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("DataDir() = %q, ingin path absolut", dir)
	}

	// The per-platform location the documentation promises.
	if !strings.Contains(strings.ToLower(dir), "simarc") {
		t.Errorf("DataDir() = %q, harus memuat nama aplikasi", dir)
	}
}

func TestIsSourceCheckout(t *testing.T) {
	dir := t.TempDir()
	if IsSourceCheckout(dir) {
		t.Error("direktori kosong dianggap checkout")
	}

	// A web/templates directory is what marks a checkout.
	templates := filepath.Join(dir, "web", "templates")
	if err := os.MkdirAll(templates, 0o755); err != nil {
		t.Fatal(err)
	}
	if !IsSourceCheckout(dir) {
		t.Error("direktori dengan web/templates seharusnya dianggap checkout")
	}

	// A file of that name is not enough.
	other := t.TempDir()
	if err := os.MkdirAll(filepath.Join(other, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "web", "templates"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if IsSourceCheckout(other) {
		t.Error("web/templates yang berupa file seharusnya tidak dianggap checkout")
	}
}

func TestSourceRootWalksUp(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "web", "templates"), 0o755); err != nil {
		t.Fatal(err)
	}

	deep := filepath.Join(root, "desktop", "build", "bin")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	got := SourceRoot(deep)
	// macOS puts temp dirs behind a symlink, so compare resolved paths.
	want, _ := filepath.EvalSymlinks(root)
	resolvedGot, _ := filepath.EvalSymlinks(got)
	if got == "" || resolvedGot != want {
		t.Errorf("SourceRoot(%q) = %q, ingin %q", deep, got, root)
	}

	if got := SourceRoot(t.TempDir()); got != "" {
		t.Errorf("SourceRoot pada direktori acak = %q, ingin kosong", got)
	}
}

func TestLoadEnvFilePrefersWorkingDirectory(t *testing.T) {
	// A .env in the working directory must win, so development is unaffected.
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	if err := os.WriteFile(env, []byte("SIMARC_TEST_MARKER=from_cwd\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	restore := chdir(t, dir)
	defer restore()

	t.Setenv("SIMARC_TEST_MARKER", "")
	os.Unsetenv("SIMARC_TEST_MARKER")
	loadEnvFile()

	if got := os.Getenv("SIMARC_TEST_MARKER"); got != "from_cwd" {
		t.Errorf("SIMARC_TEST_MARKER = %q, ingin from_cwd", got)
	}
}

func TestLoadEnvFileFallsBackToDataDir(t *testing.T) {
	// With no .env in the working directory, the per-user data directory is
	// searched — this is the path an installed app depends on.
	dataDir := t.TempDir()
	t.Setenv("SIMARC_DATA_DIR", dataDir)

	if err := os.WriteFile(filepath.Join(dataDir, ".env"),
		[]byte("SIMARC_TEST_MARKER=from_data_dir\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	empty := t.TempDir()
	restore := chdir(t, empty)
	defer restore()

	t.Setenv("SIMARC_TEST_MARKER", "")
	os.Unsetenv("SIMARC_TEST_MARKER")
	loadEnvFile()

	if got := os.Getenv("SIMARC_TEST_MARKER"); got != "from_data_dir" {
		t.Errorf("SIMARC_TEST_MARKER = %q, ingin from_data_dir", got)
	}
}

func chdir(t *testing.T, dir string) func() {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return func() { _ = os.Chdir(old) }
}
