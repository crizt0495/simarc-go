package embeddeddb

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// writeExec membuat skrip shell yang dapat dieksekusi (Linux/macOS).
func writeExec(t *testing.T, path, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("skrip uji hanya untuk Linux/macOS")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// fakeMariaDB menyiapkan folder MariaDB tiruan dengan biner install-db &
// mariadbd (yang hanya tidur, tidak membuka port) untuk menguji alur Start.
func fakeMariaDB(t *testing.T, mariadbdBody string) string {
	t.Helper()
	base := t.TempDir()
	writeExec(t, filepath.Join(base, "bin", "mariadb-install-db"), "mkdir -p data/mysql")
	writeExec(t, filepath.Join(base, "bin", "mariadbd"), mariadbdBody)
	return base
}

// freePort memilih port TCP yang bebas pada loopback.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

// containsArg melaporkan apakah args memuat nilai persis want.
func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// hasArgPrefix melaporkan apakah ada arg yang diawali prefix.
func hasArgPrefix(args []string, prefix string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, prefix) {
			return true
		}
	}
	return false
}

func TestInstallDBArgsWindows(t *testing.T) {
	args := installDBArgs("windows", `C:\app\mariadb`, `C:\app\mariadb\data`, "3307", "rahasia")
	if !containsArg(args, "--datadir="+`C:\app\mariadb\data`) {
		t.Fatalf("datadir hilang: %v", args)
	}
	if !containsArg(args, "--port=3307") {
		t.Fatalf("port hilang: %v", args)
	}
	if !containsArg(args, "--password=rahasia") {
		t.Fatalf("password hilang: %v", args)
	}
	// Windows tidak mengenal opsi Unix.
	if hasArgPrefix(args, "--basedir=") {
		t.Fatalf("Windows tak boleh memakai --basedir: %v", args)
	}
	if hasArgPrefix(args, "--auth-root-authentication-method") {
		t.Fatalf("Windows tak boleh memakai opsi auth Unix: %v", args)
	}

	// Tanpa sandi root (mis. bila sudah tersimpan), --password tidak ditambahkan.
	args = installDBArgs("windows", "", "", "3307", "")
	if hasArgPrefix(args, "--password=") {
		t.Fatalf("tak seharusnya ada --password kosong: %v", args)
	}
}

func TestInstallDBArgsUnix(t *testing.T) {
	args := installDBArgs("linux", "/opt/mariadb", "/opt/mariadb/data", "3307", "")
	if !containsArg(args, "--basedir=/opt/mariadb") {
		t.Fatalf("basedir hilang: %v", args)
	}
	if !containsArg(args, "--datadir=/opt/mariadb/data") {
		t.Fatalf("datadir hilang: %v", args)
	}
	if !containsArg(args, "--auth-root-authentication-method=normal") {
		t.Fatalf("opsi auth Unix hilang: %v", args)
	}
	if !containsArg(args, "--skip-test-db") {
		t.Fatalf("--skip-test-db hilang: %v", args)
	}
	// Unix tidak mengenal --port/--password pada skrip install-db.
	if hasArgPrefix(args, "--port=") || hasArgPrefix(args, "--password=") {
		t.Fatalf("Unix tak boleh memakai --port/--password: %v", args)
	}
}

func TestRootArgsWithPassword(t *testing.T) {
	dataDir := t.TempDir()
	m := New(t.TempDir(), dataDir, "simarc", "simarc", "pw", "3307")
	if err := os.WriteFile(m.rootPasswordFile(), []byte("S3cret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := m.rootArgs()
	for _, want := range []string{"--protocol=TCP", "-h127.0.0.1", "-P3307", "-uroot", "-pS3cret"} {
		if !containsArg(args, want) {
			t.Fatalf("rootArgs kurang %q: %v", want, args)
		}
	}
	if hasArgPrefix(args, "--skip-password") {
		t.Fatalf("tak seharusnya --skip-password: %v", args)
	}
}

func TestRootArgsWithoutPassword(t *testing.T) {
	m := New(t.TempDir(), t.TempDir(), "simarc", "simarc", "pw", "3307")
	args := m.rootArgs()
	if !containsArg(args, "--skip-password") {
		t.Fatalf("tanpa sandi tersimpan harus memakai --skip-password: %v", args)
	}
	if hasArgPrefix(args, "-p") {
		t.Fatalf("tak seharusnya ada sandi: %v", args)
	}
}

func TestDiscoverViaEnv(t *testing.T) {
	base := fakeMariaDB(t, "exec sleep 60")
	t.Setenv("SIMARC_MARIADB_DIR", base)

	got, ok := Discover()
	if !ok {
		t.Fatalf("Discover tidak menemukan MariaDB di %s", base)
	}
	if got != base {
		t.Fatalf("Discover = %q, mau %q", got, base)
	}
}

func TestEnabledToggle(t *testing.T) {
	base := fakeMariaDB(t, "exec sleep 60")
	t.Setenv("SIMARC_MARIADB_DIR", base)

	if !Enabled() {
		t.Fatal("Enabled harus true bila biner ada")
	}
	t.Setenv("SIMARC_EMBEDDED_DB", "0")
	if Enabled() {
		t.Fatal("Enabled harus false bila SIMARC_EMBEDDED_DB=0")
	}
}

func TestStartSkipsWhenListening(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	_, port, _ := net.SplitHostPort(l.Addr().String())

	m := New(t.TempDir(), "", "simarc", "simarc", "pw", port)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start harus sukses bila port sudah listening: %v", err)
	}
	if m.owned {
		t.Fatal("owned harus false; kita tidak menjalankan server")
	}
	m.Stop() // tidak melakukan apa-apa
}

func TestStartTimeoutThenStop(t *testing.T) {
	base := fakeMariaDB(t, "exec sleep 60")
	port := freePort(t)

	m := New(base, "", "simarc", "simarc", "pw", port)
	m.ReadyTimeout = 1200 * time.Millisecond
	m.StopGrace = 500 * time.Millisecond

	err := m.Start(context.Background())
	if err == nil {
		t.Fatal("Start harus gagal (mariadbd tiruan tidak membuka port)")
	}
	if m.owned {
		t.Fatal("owned harus false setelah Start gagal (Stop sudah dipanggil)")
	}
	// Datadir baru dibuat → proses install-db dijalankan.
	if _, statErr := os.Stat(filepath.Join(base, "data", "mysql")); statErr != nil {
		t.Fatalf("datadir tidak diinisialisasi: %v", statErr)
	}
}

func TestStartSucceedsWhenFakeServerOpensPort(t *testing.T) {
	// mariadbd tiruan membuka port memakai python3 (bila ada), lalu menunggu.
	if runtime.GOOS == "windows" {
		t.Skip("skrip uji hanya untuk Linux/macOS")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 tidak tersedia")
	}
	port := freePort(t)
	body := "exec python3 -c 'import socket,time;" +
		`s=socket.socket();s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);` +
		"s.bind((\"127.0.0.1\",int(\"" + port + "\")));s.listen(1);time.sleep(60)'"
	base := fakeMariaDB(t, body)
	// Tandai datadir sudah pernah diinisialisasi agar bootstrap dilewati
	// (biner mariadb klien memang tidak disertakan pada uji ini).
	if err := os.MkdirAll(filepath.Join(base, "data", "mysql"), 0o755); err != nil {
		t.Fatal(err)
	}

	m := New(base, "", "simarc", "simarc", "pw", port)
	m.ReadyTimeout = 5 * time.Second
	m.StopGrace = 500 * time.Millisecond

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start gagal: %v", err)
	}
	if !m.owned {
		t.Fatal("owned harus true karena kita yang menjalankan mariadbd")
	}
	if !m.listening() {
		t.Fatal("server tiruan harus mendengarkan di port")
	}
	m.Stop()
	select {
	case <-m.done:
	case <-time.After(5 * time.Second):
		t.Fatal("proses tiruan tidak berhenti setelah Stop")
	}
}
