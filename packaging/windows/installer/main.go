// ===========================================================================
//
//	SIMARC - Single-file Windows Installer / Updater (self-extracting)
//
//	Satu berkas .exe ini sudah MEMUAT seluruh aplikasi (biner server, web/,
//	template, dokumentasi, .env.example, skrip integrasi desktop). Menjalankan
//	exe ini akan mengekstrak payload ke folder sementara lalu memanggil
//	SIMARC-Setup.ps1 yang memasang / memperbarui aplikasi di
//	%LocalAppData%\SIMARC secara aman (data .env & storage\ tidak disentuh).
//
//	UPDATE = jalankan exe versi BARU; tidak perlu uninstall / install ulang.
//
//	Build (dari packaging/make-windows-package.sh):
//	  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o SIMARC-Setup.exe .
//
// ===========================================================================
package main

import (
	"archive/zip"
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed all:payload
var payload embed.FS

func main() {
	quiet := flag.Bool("quiet", false, "jalankan tanpa interaksi (tanpa jendela GUI)")
	noShortcut := flag.Bool("no-desktop-shortcut", false, "jangan buat shortcut Desktop")
	gui := flag.Bool("gui", false, "paksa tampilkan jendela instalasi grafis")
	cli := flag.Bool("cli", false, "paksa mode konsol (tanpa GUI)")
	mode := flag.String("mode", "", "mode instalasi: auto|offline|client|local")
	hub := flag.String("hub", "", "alamat server hub, mis. http://192.168.1.86:8080")
	flag.Parse()

	// Ekstrak payload ke folder sementara sekali di awal.
	tmp, err := os.MkdirTemp("", "simarc-setup-")
	if err != nil {
		fatalGUI("Gagal membuat folder sementara: %v", err)
	}
	defer os.RemoveAll(tmp)

	if err := extract(payload, "payload", tmp); err != nil {
		fatalGUI("Gagal mengekstrak payload: %v", err)
	}

	// Konfigurasi instalasi otomatis (opsional): bila ada berkas
	// `simarc-install.txt` di SAMPING .exe ini, salin ke folder payload agar
	// dibaca SIMARC-Setup.ps1 (mengatur mode/hub/sync tanpa ditanya).
	hasSideCfg := false
	if exe, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(exe), "simarc-install.txt")
		if data, err := os.ReadFile(sibling); err == nil {
			_ = os.WriteFile(filepath.Join(tmp, "simarc-install.txt"), data, 0o644)
			hasSideCfg = true
		}
	}

	ps1 := filepath.Join(tmp, "SIMARC-Setup.ps1")
	if _, err := os.Stat(ps1); err != nil {
		fatalGUI("SIMARC-Setup.ps1 tidak ditemukan di paket.")
	}

	// Jalur non-interaktif (deployment massal / otomatisasi): tetap pakai konsol.
	// Jalur interaktif default: jendela instalasi grafis yang menarik.
	useGUI := !*quiet && !*cli
	if *gui && !*quiet {
		useGUI = true
	}
	// Bila config mode diset (offline/client/local) tanpa minta GUI, tetap
	// gunakan GUI agar pengguna melihat progres yang rapi — kecuali -quiet/-cli.
	if hasSideCfg && *mode == "" && !*quiet && !*cli {
		// biarkan GUI membaca mode dari simarc-install.txt
	}
	if useGUI {
		if err := runGUI(tmp, ps1, *noShortcut); err == nil {
			return
		}
	}

	// ── Jalur konsol (fallback / otomatisasi) ─────────────────────────────────
	runConsole(tmp, ps1, *mode, *hub, *quiet, *noShortcut)
}

// runGUI menampilkan jendela instalasi grafis modern lalu menjalankan pemasangan.
func runGUI(tmp, ps1 string, noShortcut bool) error {
	guiScript := filepath.Join(tmp, "SIMARC-Installer-GUI.ps1")
	if _, err := os.Stat(guiScript); err != nil {
		return err
	}
	args := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", guiScript, "-SetupScript", ps1}
	if noShortcut {
		args = append(args, "-NoDesktopShortcut")
	}
	return exec.Command("powershell.exe", args...).Run()
}

// runConsole menjalankan pemasangan tanpa antarmuka grafis.
func runConsole(tmp, ps1, mode, hub string, quiet, noShortcut bool) {
	if !quiet {
		fmt.Println("============================================================")
		fmt.Println("  SIMARC - Installer / Updater (satu file)")
		fmt.Println("============================================================")
	}

	args := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", ps1}
	if quiet {
		args = append(args, "-Quiet")
	}
	if noShortcut {
		args = append(args, "-NoDesktopShortcut")
	}
	if mode != "" {
		args = append(args, "-Mode", mode)
	}
	if hub != "" {
		args = append(args, "-Hub", hub)
	}

	cmd := exec.Command("powershell.exe", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	psErr := cmd.Run()

	// PowerShell pada beberapa lingkungan (mis. Wine tanpa PowerShell fungsional)
	// dapat mengembalikan exit code 0 tanpa benar-benar mengeksekusi skrip.
	// Jika proses gagal ATAU instalasi tidak ditemukan, pasang langsung via Go.
	installDir := resolveInstallDir()
	_, appErr := os.Stat(filepath.Join(installDir, "run.bat"))
	if psErr != nil || appErr != nil {
		fmt.Println("[INFO] PowerShell tidak menyelesaikan instalasi, menjalankan pemasangan langsung...")
		if err := nativeInstall(tmp, installDir, mode, hub); err != nil {
			fail("Instalasi langsung gagal: %v", err)
		}
	}

	fmt.Println()
	fmt.Println("[OK] Instalasi/update SIMARC selesai.")
	fmt.Println("     Buka dari menu Start -> SIMARC (atau shortcut Desktop).")
	fmt.Println("     Catatan: .env dan storage\\ dipertahankan pada setiap update.")
	fmt.Println()
	pause()
}

// resolveInstallDir mengembalikan %LocalAppData%\SIMARC.
func resolveInstallDir() string {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		if home, err := os.UserHomeDir(); err == nil {
			localAppData = filepath.Join(home, "AppData", "Local")
		}
	}
	return filepath.Join(localAppData, "SIMARC")
}

// fatalGUI menampilkan pesan kesalahan dalam kotak dialog grafis (bila mungkin)
// lalu keluar dengan kode 1. Aman dipanggil dari biner bersubsidi windowsgui.
func fatalGUI(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	// MessageBox native via PowerShell (tersedia di semua Windows).
	_ = exec.Command("powershell.exe", "-NoProfile", "-Command",
		fmt.Sprintf("Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.MessageBox]::Show('%s','SIMARC - Kesalahan','OK','Error')",
			strings.ReplaceAll(msg, "'", "''"))).Run()
	fmt.Fprintln(os.Stderr, "[ERR] "+msg)
	os.Exit(1)
}

// extract menyalin seluruh isi FS (di bawah root) ke direktori tujuan.
func extract(fsys embed.FS, root, dest string) error {
	return fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, root)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			return nil
		}
		target := filepath.Join(dest, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fsys.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func fail(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "[ERR]  "+format+"\n", a...)
	pause()
	os.Exit(1)
}

func pause() {
	if fi, err := os.Stdin.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) != 0 {
		fmt.Print("Tekan Enter untuk menutup...")
		fmt.Scanln()
	}
}

// nativeInstall menyalin file dan mengatur .env secara mandiri (murni Go)
// apabila PowerShell tidak tersedia atau gagal dijalankan.
func nativeInstall(srcDir, destDir, mode, hub string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	// 1. Salin komponen aplikasi
	components := []string{"run.bat", ".env.example", "simarc.ico", "web", "docs", "dist", "packaging"}
	for _, item := range components {
		s := filepath.Join(srcDir, item)
		d := filepath.Join(destDir, item)
		if fi, err := os.Stat(s); err == nil {
			if fi.IsDir() {
				_ = copyDir(s, d)
			} else {
				_ = copyFile(s, d)
			}
		}
	}

	// 2. Siapkan direktori runtime
	for _, sub := range []string{"tmp", filepath.Join("storage", "app", "uploads"), filepath.Join("public", "storage")} {
		_ = os.MkdirAll(filepath.Join(destDir, sub), 0o755)
	}

	// 3. Salin server binary ke tmp/simarc-server.exe
	binAmd := filepath.Join(srcDir, "dist", "simarc-server-windows-amd64.exe")
	if _, err := os.Stat(binAmd); err == nil {
		_ = copyFile(binAmd, filepath.Join(destDir, "tmp", "simarc-server.exe"))
	}

	// 4. Siapkan .env bila belum ada (mode-aware) — konfigurasi milik komputer
	//    ini TIDAK pernah ditimpa.
	envDest := filepath.Join(destDir, ".env")
	if _, err := os.Stat(envDest); os.IsNotExist(err) {
		envEx := filepath.Join(srcDir, ".env.example")
		data, _ := os.ReadFile(envEx)
		content := string(data)

		chosen := strings.ToLower(strings.TrimSpace(mode))
		if chosen == "" || chosen == "auto" {
			chosen = readInstallMode(srcDir)
		}
		hubURL := strings.TrimRight(strings.TrimSpace(hub), "/")
		if hubURL == "" {
			hubURL = "http://192.168.1.86:8080"
		}

		// Bundel MariaDB ada? -> memungkinkan mode offline mandiri.
		hasDB := false
		if _, err := os.Stat(filepath.Join(srcDir, "mariadb-winx64.zip")); err == nil {
			hasDB = true
		}

		switch chosen {
		case "offline":
			if hasDB {
				pw := randomPassword(24)
				content = setEnvKeys(content, map[string]string{
					"SIMARC_SERVER_URL":  "",
					"DB_HOST":            "127.0.0.1",
					"DB_PORT":            "3307",
					"DB_DATABASE":        "simarc",
					"DB_USERNAME":        "simarc",
					"DB_PASSWORD":        pw,
					"SIMARC_EMBEDDED_DB": "1",
				})
				content = applySyncEnv(content, srcDir)
				fmt.Println("[OK]   Mode OFFLINE MANDIRI: database lokal otomatis (port 3307).")
			} else {
				content = setEnvKeys(content, map[string]string{"SIMARC_SERVER_URL": hubURL})
				fmt.Println("[OK]   Mode KLIEN (bundel DB tidak ada) ke " + hubURL)
			}
		case "client":
			content = setEnvKeys(content, map[string]string{"SIMARC_SERVER_URL": hubURL})
			fmt.Println("[OK]   Mode KLIEN ke " + hubURL)
		default: // local
			fmt.Println("[OK]   Mode SERVER LOKAL (butuh MySQL/MariaDB lokal).")
		}
		_ = os.WriteFile(envDest, []byte(content), 0o644)
	}

	// 5. Ekstrak bundel MariaDB portabel jika ada
	zipMaria := filepath.Join(srcDir, "mariadb-winx64.zip")
	mariadbDest := filepath.Join(destDir, "mariadb")
	mariadbdExe := filepath.Join(mariadbDest, "bin", "mariadbd.exe")
	if _, err := os.Stat(zipMaria); err == nil {
		if _, err := os.Stat(mariadbdExe); os.IsNotExist(err) {
			_ = unzip(zipMaria, mariadbDest)
		}
	}

	return nil
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(fpath, os.ModePerm)
			continue
		}
		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}
		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	return os.WriteFile(dst, in, 0o755)
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

// readInstallMode membaca mode= dari simarc-install.txt (bila ada), default auto.
func readInstallMode(srcDir string) string {
	cfg := readInstallConfig(srcDir)
	if m := strings.ToLower(strings.TrimSpace(cfg["MODE"])); m != "" {
		return m
	}
	return "auto"
}

// readInstallConfig mengurai simarc-install.txt (KEY=VALUE, kunci di-uppercase).
func readInstallConfig(srcDir string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(filepath.Join(srcDir, "simarc-install.txt"))
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.IndexByte(line, '=')
		if i <= 0 {
			continue
		}
		key := strings.ToUpper(strings.TrimSpace(line[:i]))
		val := strings.TrimSpace(line[i+1:])
		if key != "" {
			out[key] = val
		}
	}
	return out
}

// randomPassword menghasilkan sandi acak alfanumerik sepanjang n karakter.
func randomPassword(n int) string {
	const charset = "0123456789abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// setEnvKeys mengganti/menambahkan kunci pada isi .env tanpa menyentuh kunci lain.
func setEnvKeys(content string, kv map[string]string) string {
	lines := strings.Split(content, "\n")
	seen := map[string]bool{}
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		j := strings.IndexByte(trimmed, '=')
		if j <= 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:j])
		if v, ok := kv[key]; ok {
			lines[i] = key + "=" + v
			seen[key] = true
		}
	}
	for k, v := range kv {
		if !seen[k] {
			lines = append(lines, k+"="+v)
		}
	}
	return strings.Join(lines, "\n")
}

// applySyncEnv menambahkan kunci sinkronisasi (SYNC_*) dari simarc-install.txt,
// termasuk identitas simpul unik per komputer, tanpa menimpa nilai eksplisit.
func applySyncEnv(content, srcDir string) string {
	cfg := readInstallConfig(srcDir)
	if strings.TrimSpace(cfg["SYNC_HUB_DB_HOST"]) == "" {
		fmt.Println("[INFO] Sinkronisasi antar-komputer tidak disetel (tanpa SYNC_HUB_DB_HOST).")
		return content
	}

	kv := map[string]string{}
	for k, v := range cfg {
		if strings.HasPrefix(k, "SYNC_") {
			kv[k] = v
		}
	}
	if _, ok := kv["SYNC_ENABLED"]; !ok {
		kv["SYNC_ENABLED"] = "true"
	}
	if _, ok := kv["SYNC_NODE_ID"]; !ok {
		kv["SYNC_NODE_ID"] = sanitizeNodeID(os.Getenv("COMPUTERNAME"))
	}
	if _, ok := kv["SYNC_INTERVAL_MINUTES"]; !ok {
		kv["SYNC_INTERVAL_MINUTES"] = "15"
	}

	out := setEnvKeys(content, kv)
	fmt.Printf("[OK]   Sinkronisasi otomatis AKTIF (simpul: %s) - data menyatu lewat hub.\n", kv["SYNC_NODE_ID"])
	return out
}

// sanitizeNodeID membersihkan nama komputer menjadi identitas simpul yang aman.
func sanitizeNodeID(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	id := strings.Trim(strings.ToLower(b.String()), "-")
	if id == "" {
		return "node-unknown"
	}
	return id
}
