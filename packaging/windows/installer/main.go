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
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ncruces/zenity"
)

//go:embed all:payload
var payload embed.FS

// appWasRunning mencatat apakah SIMARC sedang berjalan saat instalasi dimulai.
// Bila ya, aplikasi dijalankan ulang otomatis setelah pemasangan selesai.
var appWasRunning bool

// quietMode: instalasi massal tanpa interaksi (tidak menunggu tombol Enter).
var quietMode bool

func main() {
	quiet := flag.Bool("quiet", false, "jalankan tanpa interaksi (tanpa jendela GUI)")
	noShortcut := flag.Bool("no-desktop-shortcut", false, "jangan buat shortcut Desktop")
	gui := flag.Bool("gui", false, "paksa tampilkan jendela instalasi grafis")
	cli := flag.Bool("cli", false, "paksa mode konsol (tanpa GUI)")
	modeFlag := flag.String("mode", "", "mode instalasi: auto|offline|client|local")
	hubFlag := flag.String("hub", "", "alamat server hub, mis. http://192.168.1.86:8080")
	flag.Parse()

	// Salinan lokal agar dapat diubah pada jalur fallback GUI.
	mode, hub := *modeFlag, *hubFlag

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
	if exe, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(exe), "simarc-install.txt")
		if data, err := os.ReadFile(sibling); err == nil {
			_ = os.WriteFile(filepath.Join(tmp, "simarc-install.txt"), data, 0o644)
		}
	}

	ps1 := filepath.Join(tmp, "SIMARC-Setup.ps1")
	if _, err := os.Stat(ps1); err != nil {
		fatalGUI("SIMARC-Setup.ps1 tidak ditemukan di paket.")
	}

	// Jalur non-interaktif (deployment massal / otomatisasi): tetap pakai konsol.
	// Jalur interaktif default: zenity GUI wizard murni Go (tanpa PowerShell/WinForms).
	useGUI := !*quiet && !*cli
	if *gui && !*quiet {
		useGUI = true
	}
	if useGUI {
		runZenityGUI(tmp, mode, hub, *noShortcut)
		return
	}

	// ── Jalur konsol (fallback / otomatisasi) ─────────────────────────────────
	runConsole(tmp, ps1, mode, hub, *quiet, *noShortcut)
}

// runZenityGUI menampilkan wizard instalasi menggunakan dialog native (zenity).
func runZenityGUI(tmp, mode, hub string, noShortcut bool) {
	modeStr := strings.ToLower(strings.TrimSpace(mode))
	hubStr := strings.TrimSpace(hub)

	// Auto-config dari simarc-install.txt (bila ada): lewati dialog.
	cfg := readInstallConfig(tmp)
	if modeStr == "" || modeStr == "auto" {
		if m := strings.ToLower(strings.TrimSpace(cfg["MODE"])); m != "" {
			modeStr = m
		}
	}
	if hubStr == "" && cfg["HUB"] != "" {
		hubStr = strings.TrimSpace(cfg["HUB"])
	}
	if hubStr == "" && cfg["SYNC_HUB_URL"] != "" {
		hubStr = strings.TrimSpace(cfg["SYNC_HUB_URL"])
	}

	// Instalasi OTOMATIS (tanpa dialog sama sekali): mode/hub berasal dari
	// simarc-install.txt yang tertanam di exe, atau dari flag baris-perintah.
	// Dipakai untuk mass-deployment — jendela instalasi tidak pernah menuntut
	// klik pengguna, termasuk dialog penutup.
	cfgMode := strings.ToLower(strings.TrimSpace(cfg["MODE"]))
	autoInstall := (cfgMode != "" && cfgMode != "auto") ||
		cfg["HUB"] != "" || cfg["SYNC_HUB_URL"] != "" ||
		(mode != "" && mode != "auto") || hub != ""

	// 1. Pilih mode instalasi (hanya bila belum ditentukan lewat flag/config).
	if modeStr == "" || modeStr == "auto" {
		chosen, err := zenity.List(
			"Pilih mode instalasi SIMARC:",
			[]string{"Offline (Mandiri)", "Klien Jaringan"},
			zenity.Title("SIMARC Installer"),
			zenity.RadioList(),
			zenity.DefaultItems("Offline (Mandiri)"),
		)
		if err != nil {
			if errors.Is(err, zenity.ErrCanceled) {
				return // dibatalkan pengguna
			}
			modeStr = "offline" // fallback jika dialog gagal
		} else {
			if strings.HasPrefix(chosen, "Klien") {
				modeStr = "client"
			} else {
				modeStr = "offline"
			}
		}
	}

	// 2. Mode klien: minta alamat hub.
	if modeStr == "client" && hubStr == "" {
		entered, err := zenity.Entry(
			"Masukkan alamat server HUB (contoh: http://192.168.1.86:8080):",
			zenity.Title("SIMARC Installer"),
			zenity.EntryText("http://192.168.1.86:8080"),
		)
		if err != nil {
			if errors.Is(err, zenity.ErrCanceled) {
				return
			}
			hubStr = "http://192.168.1.86:8080"
		} else {
			hubStr = strings.TrimSpace(entered)
		}
	}

	installDir := resolveInstallDir()

	// 3. Jalankan pemasangan sambil menampilkan jendela progres bergaya
	//    Windows (judul, bilah progres, dan teks status). Jendela ini TIDAK
	//    menuntut klik apa pun; hanya `-quiet` (deployment massal) yang
	//    benar-benar tanpa jendela.
	prog, err := zenity.Progress(
		zenity.Title("SIMARC - Progres Instalasi"),
		zenity.MaxValue(100),
		zenity.NoCancel(),
	)
	if err != nil {
		// Jendela progres tidak tersedia: jalankan tanpa GUI lalu beri tahu.
		if err := nativeInstall(tmp, installDir, modeStr, hubStr); err != nil {
			failGUI("Instalasi gagal: " + err.Error())
		}
		if err := verifyInstall(installDir, modeStr); err != nil {
			failGUI("Verifikasi gagal: " + err.Error())
		}
		_ = createShortcuts(installDir, filepath.Join(installDir, "run.bat"), filepath.Join(installDir, "simarc.ico"), noShortcut)
		_ = zenity.Info("SIMARC berhasil diinstal.", zenity.Title("SIMARC - Instalasi Selesai"))
		return
	}

	installErr := nativeInstallWithProgress(tmp, installDir, modeStr, hubStr, prog, noShortcut)
	_ = prog.Complete()
	_ = prog.Close()

	if installErr != nil {
		failGUI("Instalasi gagal: " + installErr.Error())
	}

	// Konfigurasi otomatis (tertanam): tidak ada pertanyaan, tetapi pengguna
	// tetap melihat jendela progres + pemberitahuan selesai seperti installer
	// Windows pada umumnya. Aplikasi langsung dijalankan.
	if autoInstall {
		logInstall("Pemasangan selesai (otomatis); aplikasi dijalankan.")
		launchInstalled(installDir)
		_ = zenity.Info(
			"SIMARC berhasil diinstal dan sedang dijalankan.\n\n"+
				"Buka kapan saja dari menu Start → SIMARC\n"+
				"atau ikon SIMARC di Desktop.",
			zenity.Title("SIMARC - Instalasi Selesai"),
		)
		return
	}

	// Aplikasi sempat berjalan sebelum update? Jalankan ulang otomatis agar
	// pengguna kembali bekerja tanpa perlu membuka manual.
	if appWasRunning {
		logInstall("Pemasangan selesai; aplikasi dijalankan ulang otomatis.")
		launchInstalled(installDir)
		_ = zenity.Info("SIMARC berhasil diperbarui dan dijalankan kembali.", zenity.Title("SIMARC - Instalasi Selesai"))
		return
	}
	logInstall("Pemasangan selesai (OK).")

	// 4. Selesai. Tawarkan membuka aplikasi.
	openNow := zenity.Question("SIMARC berhasil diinstal.\n\nBuka SIMARC sekarang?",
		zenity.Title("SIMARC - Instalasi Selesai"),
		zenity.OKLabel("Buka SIMARC"),
		zenity.CancelLabel("Nanti"),
	) == nil
	if openNow {
		launchInstalled(installDir)
	}
}

// nativeInstallWithProgress menjalankan nativeInstall sambil memperbarui progres.
func nativeInstallWithProgress(tmp, installDir, mode, hub string, prog zenity.ProgressDialog, noShortcut bool) error {
	_ = prog.Text("Menyiapkan folder instalasi...")
	_ = prog.Value(5)

	_ = prog.Text("Menyalin aplikasi & menyiapkan konfigurasi...")
	_ = prog.Value(30)

	if err := nativeInstall(tmp, installDir, mode, hub); err != nil {
		return err
	}

	_ = prog.Text("Verifikasi berkas...")
	_ = prog.Value(70)
	if err := verifyInstall(installDir, mode); err != nil {
		return err
	}

	_ = prog.Text("Membuat shortcut...")
	_ = prog.Value(90)
	_ = createShortcuts(installDir, filepath.Join(installDir, "run.bat"), filepath.Join(installDir, "simarc.ico"), noShortcut)

	_ = prog.Value(100)
	return nil
}

// launchInstalled membuka SIMARC yang sudah terpasang (run.bat) TANPA jendela
// konsol sama sekali (tidak ada CMD yang berkedip / muncul di taskbar).
// run.bat tetap dipakai agar logika database portabel & auto-update berjalan.
func launchInstalled(installDir string) {
	bat := filepath.Join(installDir, "run.bat")
	if _, err := os.Stat(bat); err != nil {
		logInstall("[WARN] run.bat tidak ditemukan; aplikasi tidak dijalankan otomatis.")
		return
	}
	cmd := exec.Command("cmd", "/C", bat)
	cmd.Dir = installDir
	setHiddenWindow(cmd) // sembunyikan total (SW_HIDE + CREATE_NO_WINDOW)
	if err := cmd.Start(); err != nil {
		logInstall("[WARN] Gagal menjalankan aplikasi: %v", err)
		return
	}
	_ = cmd.Process.Release()
}

// runConsole menjalankan pemasangan tanpa antarmuka grafis.
func runConsole(tmp, ps1, mode, hub string, quiet, noShortcut bool) {
	if !quiet {
		fmt.Println("============================================================")
		fmt.Println("  SIMARC - Installer / Updater (satu file)")
		fmt.Println("============================================================")
	}

	args := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-File", ps1}
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

	installDir := resolveInstallDir()
	quietMode = quiet
	// Hentikan SIMARC yang sedang berjalan agar binary & database tidak
	// terkunci saat instalasi/upgrade (wajib untuk install ulang mulus).
	stopRunningApp(installDir)
	if mode == "" || mode == "auto" {
		mode = "offline"
	}
	logInstall("Mulai pemasangan (konsol) ke %s (mode=%s).", installDir, mode)

	installed := false
	if _, err := exec.LookPath("powershell.exe"); err == nil {
		cmd := exec.Command("powershell.exe", args...)
		setHiddenWindow(cmd)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		psErr := cmd.Run()
		_, appErr := os.Stat(filepath.Join(installDir, "run.bat"))
		if psErr == nil && appErr == nil {
			// PowerShell pada beberapa lingkungan (mis. Wine tanpa PowerShell
			// fungsional, atau execution policy yang menghalangi) dapat
			// mengembalikan exit code 0 tanpa benar-benar memasang lengkap.
			// Khusus skrip PS1, penyalinan memakai -ErrorAction SilentlyContinue
			// sehingga berkas yang terkunci bisa GAGAL TANPA PESAN dan versi
			// lama tertinggal. Karena itu hasilnya tetap diverifikasi,
			// termasuk membandingkan binary server dengan paket ini.
			verr := verifyInstall(installDir, mode)
			if verr == nil && !serverBinaryMatches(tmp, installDir) {
				verr = errors.New("binary server terpasang bukan dari paket ini (salinan PowerShell gagal diam-diam)")
			}
			if verr == nil {
				installed = true
			} else {
				fmt.Printf("[INFO] Hasil PowerShell belum lengkap (%v); menyelesaikan dengan pemasangan langsung...\n", verr)
			}
		}
	}

	if !installed {
		fmt.Println("[INFO] Menjalankan pemasangan langsung (tanpa PowerShell)...")
		if err := nativeInstall(tmp, installDir, mode, hub); err != nil {
			fail("Instalasi langsung gagal: %v", err)
		}
		if err := verifyInstall(installDir, mode); err != nil {
			fail("Verifikasi instalasi gagal: %v", err)
		}
	}

	// Pastikan shortcut Start Menu + Desktop selalu ada (idempoten).
	if err := createShortcuts(installDir, filepath.Join(installDir, "run.bat"), filepath.Join(installDir, "simarc.ico"), noShortcut); err != nil {
		fmt.Println("[WARN] Gagal membuat shortcut:", err)
	}

	fmt.Println()
	fmt.Println("[OK] Instalasi/update SIMARC selesai.")
	fmt.Println("     Buka dari menu Start -> SIMARC (atau shortcut Desktop).")
	fmt.Println("     Catatan: .env dan storage\\ dipertahankan pada setiap update.")
	logInstall("Pemasangan selesai (OK).")
	// Aplikasi berjalan sebelum instalasi? Nyalakan kembali otomatis.
	if appWasRunning {
		fmt.Println("[OK]   Aplikasi sebelumnya berjalan - dijalankan ulang otomatis.")
		launchInstalled(installDir)
	}
	fmt.Println()
	pause()
}

// resolveInstallDir mengembalikan %LocalAppData%\SIMARC.
func verifyInstall(installDir, mode string) error {
	required := []string{
		"run.bat", ".env", "tmp/simarc-server.exe",
		"web/templates/layouts/app.html",
	}
	for _, rel := range required {
		fi, err := os.Stat(filepath.Join(installDir, filepath.FromSlash(rel)))
		if err != nil || fi.IsDir() || fi.Size() == 0 {
			return fmt.Errorf("file wajib tidak ada atau kosong: %s", rel)
		}
	}
	// Komponen database portabel wajib ada bila mode offline, ATAU bila .env
	// komputer ini memang mengaktifkan database tertanam (SIMARC_EMBEDDED_DB).
	// Memakai .env sebagai sumber kebenaran membuat verifikasi tetap tepat
	// walau mode tidak ditentukan eksplisit (mis. dipasang lewat skrip lain).
	if strings.EqualFold(mode, "offline") || envEmbeddedDB(installDir) {
		for _, rel := range []string{"mariadb/bin/mariadbd.exe", "mariadb/bin/mariadb-install-db.exe"} {
			fi, err := os.Stat(filepath.Join(installDir, filepath.FromSlash(rel)))
			if err != nil || fi.IsDir() || fi.Size() == 0 {
				return fmt.Errorf("komponen database portabel tidak ada atau kosong: %s", rel)
			}
		}
	}
	return nil
}

// envEmbeddedDB melaporkan apakah .env pada folder instalasi mengaktifkan
// database portabel (SIMARC_EMBEDDED_DB=1/true).
func envEmbeddedDB(installDir string) bool {
	data, err := os.ReadFile(filepath.Join(installDir, ".env"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		l := strings.TrimSpace(line)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if strings.HasPrefix(l, "SIMARC_EMBEDDED_DB=") {
			v := strings.TrimSpace(strings.TrimPrefix(l, "SIMARC_EMBEDDED_DB="))
			v = strings.Trim(v, "\"'")
			return v == "1" || strings.EqualFold(v, "true")
		}
	}
	return false
}

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
	_ = zenithMessageBox(msg) // show native dialog via zenity if available, else console
	fmt.Fprintln(os.Stderr, "[ERR] "+msg)
	logInstall("[ERR] %s", msg)
	os.Exit(1)
}

// failGUI memperlihatkan kesalahan instalasi ke pengguna (dialog), sekaligus
// mencatatnya ke stderr dan install.log agar mudah didiagnosis dari jarak jauh,
// lalu keluar dengan kode 1.
func failGUI(msg string) {
	_ = zenithMessageBox(msg)
	fmt.Fprintln(os.Stderr, "[ERR] "+msg)
	logInstall("[ERR] %s", msg)
	os.Exit(1)
}

// zenithMessageBox shows a simple error dialog using zenity (fallback to console).
func zenithMessageBox(msg string) error {
	if zenity.IsAvailable() {
		return zenity.Error(msg, zenity.Title("SIMARC - Kesalahan"))
	}
	// If zenity not available (unlikely on Windows), just print.
	return nil
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

// extractZip mengekstrak berkas .zip ke direktori tujuan.
func extractZip(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for _, f := range r.File {
		target := filepath.Join(dest, filepath.FromSlash(f.Name))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		out.Close()
		rc.Close()
	}
	return nil
}

func fail(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintln(os.Stderr, "[ERR]  "+msg)
	logInstall("[ERR] %s", msg)
	pause()
	os.Exit(1)
}

func pause() {
	if quietMode { // mass-deployment: jangan pernah menunggu input
		return
	}
	if fi, err := os.Stdin.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) != 0 {
		fmt.Print("Tekan Enter untuk menutup...")
		fmt.Scanln()
	}
}

// logInstall menulis jejak instalasi ke <folder instal>\install.log agar
// kegagalan di komputer pengguna dapat diperiksa dari jarak jauh (biner ini
// ber-subsystem windowsgui sehingga keluaran konsol tidak selalu terlihat).
func logInstall(format string, a ...interface{}) {
	dir := resolveInstallDir()
	_ = os.MkdirAll(dir, 0o755)
	f, err := os.OpenFile(filepath.Join(dir, "install.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, a...))
}

// appRunning memeriksa apakah server SIMARC masih berjalan (tasklist).
// Bila tasklist tidak tersedia di sistem, dianggap tidak berjalan.
func appRunning() bool {
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq simarc-server.exe").Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "simarc-server.exe")
}

// stopRunningApp menghentikan SIMARC yang sedang berjalan SEBELUM instalasi /
// update, agar binary (tmp\simarc-server.exe) dan database portabel tidak
// terkunci — tanpa ini, install ulang di atas aplikasi yang hidup GAGAL.
// Mengembalikan true bila ternyata ada proses yang dihentikan.
func stopRunningApp(installDir string) bool {
	wasRunning := appRunning()

	// 1) Matikan database portabel lewat pid-file (sama seperti run.bat).
	if data, err := os.ReadFile(filepath.Join(installDir, "mariadb", "data", "mysqld.pid")); err == nil {
		if pid := strings.TrimSpace(string(data)); pid != "" && pid != "0" {
			_ = exec.Command("taskkill", "/PID", pid, "/T", "/F").Run()
		}
	}
	// 2) Matikan server beserta seluruh proses anaknya (termasuk mariadbd
	//    yang dikandungnya). Bila tidak berjalan, abaikan saja.
	_ = exec.Command("taskkill", "/IM", "simarc-server.exe", "/T", "/F").Run()

	if !wasRunning {
		return false
	}
	appWasRunning = true
	// 3) Tunggu sampai benar-benar berhenti lalu beri waktu OS melepas kunci.
	for i := 0; i < 20; i++ {
		if !appRunning() {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	time.Sleep(1 * time.Second)
	logInstall("Aplikasi yang berjalan dihentikan sebelum instalasi (update di atas sesi aktif).")
	return true
}

// serverBinaryMatches memastikan binary server yang terpasang benar-benar
// berasal dari paket ini (ukurannya sama). Ini menangkap kasus salinan
// PowerShell yang gagal diam-diam (-ErrorAction SilentlyContinue) sehingga
// versi lama tetap tertinggal padahal instalasi dilaporkan berhasil.
func serverBinaryMatches(srcDir, installDir string) bool {
	src := filepath.Join(srcDir, "dist", "simarc-server-windows-amd64.exe")
	dst := filepath.Join(installDir, "tmp", "simarc-server.exe")
	si, err := os.Stat(src)
	if err != nil {
		return true // tidak bisa dibandingkan: jangan menghalangi instalasi
	}
	di, err := os.Stat(dst)
	if err != nil {
		return false
	}
	return si.Size() == di.Size()
}

// nativeInstall menyalin file dan mengatur .env secara mandiri (murni Go)
// apabila PowerShell tidak tersedia atau gagal dijalankan.
func nativeInstall(srcDir, destDir, mode, hub string) error {
	// Hentikan SIMARC yang sedang berjalan lebih dulu: binary & database
	// portabel harus bebas kunci agar install ulang / update selalu mulus.
	stopRunningApp(destDir)
	logInstall("Mulai pemasangan ke %s (mode=%s).", destDir, mode)

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
				err = copyDir(s, d)
			} else {
				err = copyFile(s, d)
			}
			if err != nil {
				return fmt.Errorf("menyalin %s: %w", item, err)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("memeriksa %s: %w", item, err)
		}
	}

	// 1b. Ekstrak MariaDB portabel (bila ada bundel) — diperlukan untuk mode
	//     offline mandiri. Hanya diekstrak bila mariadbd.exe belum ada (update
	//     tidak menimpa database yang sudah terpasang).
	if _, err := os.Stat(filepath.Join(srcDir, "mariadb-winx64.zip")); err == nil {
		dbDir := filepath.Join(destDir, "mariadb")
		if _, err := os.Stat(filepath.Join(dbDir, "bin", "mariadbd.exe")); os.IsNotExist(err) {
			if err := extractZip(filepath.Join(srcDir, "mariadb-winx64.zip"), dbDir); err != nil {
				return fmt.Errorf("mengekstrak MariaDB: %w", err)
			}
		}
	}

	// 2. Siapkan direktori runtime
	for _, sub := range []string{"tmp", filepath.Join("storage", "app", "uploads"), filepath.Join("public", "storage")} {
		if err := os.MkdirAll(filepath.Join(destDir, sub), 0o755); err != nil {
			return fmt.Errorf("membuat folder %s: %w", sub, err)
		}
	}

	// 3. Salin server binary ke tmp/simarc-server.exe
	binAmd := filepath.Join(srcDir, "dist", "simarc-server-windows-amd64.exe")
	if _, err := os.Stat(binAmd); err == nil {
		if err := copyFile(binAmd, filepath.Join(destDir, "tmp", "simarc-server.exe")); err != nil {
			return fmt.Errorf("menyalin server: %w", err)
		}
	} else {
		return fmt.Errorf("biner server Windows tidak ditemukan")
	}

	// 4. Siapkan .env bila belum ada (mode-aware) — konfigurasi milik komputer
	//    ini TIDAK pernah ditimpa.
	envDest := filepath.Join(destDir, ".env")
	if _, err := os.Stat(envDest); os.IsNotExist(err) {
		envEx := filepath.Join(srcDir, ".env.example")
		data, err := os.ReadFile(envEx)
		if err != nil {
			return fmt.Errorf("membaca .env.example: %w", err)
		}
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
		if err := os.WriteFile(envDest, []byte(content), 0o644); err != nil {
			return fmt.Errorf("menulis .env: %w", err)
		}
	}

	// 5. Ekstrak bundel MariaDB portabel jika ada
	zipMaria := filepath.Join(srcDir, "mariadb-winx64.zip")
	mariadbDest := filepath.Join(destDir, "mariadb")
	mariadbdExe := filepath.Join(mariadbDest, "bin", "mariadbd.exe")
	if _, err := os.Stat(zipMaria); err == nil {
		if _, err := os.Stat(mariadbdExe); os.IsNotExist(err) {
			fmt.Println("[INFO] Mengekstrak database portabel...")
			if err := unzip(zipMaria, mariadbDest); err != nil {
				return fmt.Errorf("mengekstrak database portabel: %w", err)
			}
		}
	}

	// MariaDB tidak boleh diinisialisasi di sini. Server SIMARC sendiri
	// menginisialisasi datadir, membuat kata sandi root, lalu membuat user
	// aplikasi dengan password dari .env secara atomik pada startup pertama.
	// Dengan begitu fallback ini setara dengan jalur PowerShell dan tidak
	// menghasilkan user/password database yang tidak cocok.

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

// copyFile menyalin satu berkas, dengan percobaan ulang bila berkas tujuan
// masih terkunci sementara (antivirus / sisa proses lama). 5 x 400 ms.
func copyFile(src, dst string) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(400 * time.Millisecond)
		}
		if err = copyFileOnce(src, dst); err == nil {
			return nil
		}
	}
	return err
}

func copyFileOnce(src, dst string) error {
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
