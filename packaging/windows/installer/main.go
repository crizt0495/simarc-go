// ===========================================================================
//  SIMARC - Single-file Windows Installer / Updater (self-extracting)
//
//  Satu berkas .exe ini sudah MEMUAT seluruh aplikasi (biner server, web/,
//  template, dokumentasi, .env.example, skrip integrasi desktop). Menjalankan
//  exe ini akan mengekstrak payload ke folder sementara lalu memanggil
//  SIMARC-Setup.ps1 yang memasang / memperbarui aplikasi di
//  %LocalAppData%\SIMARC secara aman (data .env & storage\ tidak disentuh).
//
//  UPDATE = jalankan exe versi BARU; tidak perlu uninstall / install ulang.
//
//  Build (dari packaging/make-windows-package.sh):
//    GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o SIMARC-Setup.exe .
// ===========================================================================
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed all:payload
var payload embed.FS

func main() {
	quiet := flag.Bool("quiet", false, "jalankan tanpa interaksi")
	noShortcut := flag.Bool("no-desktop-shortcut", false, "jangan buat shortcut Desktop")
	mode := flag.String("mode", "", "mode instalasi: auto|offline|client|local")
	hub := flag.String("hub", "", "alamat server hub, mis. http://192.168.1.86:8080")
	flag.Parse()

	if !*quiet {
		fmt.Println("============================================================")
		fmt.Println("  SIMARC - Installer / Updater (satu file)")
		fmt.Println("============================================================")
	}

	// 1) Ekstrak payload ke folder sementara.
	tmp, err := os.MkdirTemp("", "simarc-setup-")
	if err != nil {
		fail("Gagal membuat folder sementara: %v", err)
	}
	defer os.RemoveAll(tmp)

	if err := extract(payload, "payload", tmp); err != nil {
		fail("Gagal mengekstrak payload: %v", err)
	}

	// 1b) Konfigurasi instalasi otomatis (opsional): bila ada berkas
	//     `simarc-install.txt` di SAMPING .exe ini, salin ke folder payload agar
	//     dibaca SIMARC-Setup.ps1 (mengatur mode/hub/sync tanpa ditanya).
	if exe, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(exe), "simarc-install.txt")
		if data, err := os.ReadFile(sibling); err == nil {
			_ = os.WriteFile(filepath.Join(tmp, "simarc-install.txt"), data, 0o644)
		}
	}

	// 2) Jalankan SIMARC-Setup.ps1 (memasang/memperbarui + shortcut, data aman).
	ps1 := filepath.Join(tmp, "SIMARC-Setup.ps1")
	if _, err := os.Stat(ps1); err != nil {
		fail("SIMARC-Setup.ps1 tidak ditemukan di paket.")
	}
	args := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", ps1}
	if *quiet {
		args = append(args, "-Quiet")
	}
	if *noShortcut {
		args = append(args, "-NoDesktopShortcut")
	}
	if *mode != "" {
		args = append(args, "-Mode", *mode)
	}
	if *hub != "" {
		args = append(args, "-Hub", *hub)
	}

	cmd := exec.Command("powershell.exe", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fail("Instalasi gagal: %v", err)
	}

	fmt.Println()
	fmt.Println("[OK] Instalasi/update SIMARC selesai.")
	fmt.Println("     Buka dari menu Start -> SIMARC (atau shortcut Desktop).")
	fmt.Println("     Catatan: .env dan storage\\ dipertahankan pada setiap update.")
	fmt.Println()
	pause()
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
