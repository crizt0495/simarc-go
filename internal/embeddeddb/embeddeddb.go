// Package embeddeddb menjalankan database MariaDB portabel yang disertakan
// bersama aplikasi desktop (mode "offline mandiri").
//
// Tujuannya: satu komputer klien cukup memasang SIMARC (satu file installer),
// lalu aplikasi memakai database-nya sendiri tanpa perlu memasang MySQL/MariaDB
// secara manual dan tanpa bergantung pada komputer server. Database ini hanya
// mendengarkan di loopback (127.0.0.1) sehingga tidak terekspos ke jaringan.
//
// Paket ini sengaja tidak mengimpor paket config (menghindari siklus impor);
// seluruh nilai (nama database, user, sandi, port) diberikan oleh pemanggil.
//
// Hidup-mati database hanya dikelola bila berkas biner MariaDB benar-benar ada
// di samping aplikasi (lihat Discover). Bila tidak ada, paket ini tidak
// melakukan apa pun — aplikasi berjalan seperti biasa (server lokal memakai
// MySQL yang terpasang sendiri, atau mode klien tipis).
package embeddeddb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Manager mengelola satu instance MariaDB portabel.
type Manager struct {
	BaseDir string // folder MariaDB (berisi bin/, share/, lib/)
	DataDir string // folder data (datadir) database ini
	Port    string // port TCP (loopback)
	Name    string // nama database aplikasi
	User    string // user aplikasi
	Pass    string // sandi user aplikasi

	// ReadyTimeout adalah batas tunggu hingga database siap menerima koneksi.
	ReadyTimeout time.Duration
	// StopGrace adalah batas tunggu shutdown rapi sebelum dipaksa mati.
	StopGrace time.Duration

	cmd     *exec.Cmd
	owned   bool // true bila proses ini yang menjalankan mariadbd
	done    chan struct{}
	waitErr error
}

// exeName menambahkan akhiran .exe pada Windows.
func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// Discover mencari MariaDB portabel di sekitar berkas aplikasi.
//
// Urutan pencarian:
//  1. SIMARC_MARIADB_DIR (bila diisi)
//  2. <direktori kerja>/mariadb          (run.bat menjalankan dari folder install)
//  3. <folder biner aplikasi>/mariadb
//  4. <folder biner aplikasi>/../mariadb (biner di tmp/, MariaDB di folder induk)
//
// Mengembalikan folder MariaDB dan true bila biner server ditemukan.
func Discover() (string, bool) {
	var roots []string
	if d := strings.TrimSpace(os.Getenv("SIMARC_MARIADB_DIR")); d != "" {
		roots = append(roots, d)
	}
	if wd, err := os.Getwd(); err == nil && wd != "" {
		roots = append(roots, filepath.Join(wd, "mariadb"))
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		dir := filepath.Dir(exe)
		roots = append(roots, filepath.Join(dir, "mariadb"))
		roots = append(roots, filepath.Join(dir, "..", "mariadb"))
	}
	for _, root := range roots {
		if isMariaDBDir(root) {
			abs, err := filepath.Abs(root)
			if err != nil {
				abs = root
			}
			return abs, true
		}
	}
	return "", false
}

// isMariaDBDir melaporkan apakah dir terlihat seperti folder MariaDB portabel.
func isMariaDBDir(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, "bin", exeName("mariadbd")))
	return err == nil && !fi.IsDir()
}

// New membuat Manager. dataDir boleh dikosongkan untuk memakai
// <BaseDir>/data secara default.
func New(baseDir, dataDir, database, user, pass, port string) *Manager {
	if dataDir == "" {
		dataDir = filepath.Join(baseDir, "data")
	}
	return &Manager{
		BaseDir:      baseDir,
		DataDir:      dataDir,
		Port:         port,
		Name:         database,
		User:         user,
		Pass:         pass,
		ReadyTimeout: 60 * time.Second,
		StopGrace:    10 * time.Second,
	}
}

// Enabled melaporkan apakah pengelolaan database portabel diaktifkan. Secara
// default aktif bila biner ditemukan; setel SIMARC_EMBEDDED_DB=0 untuk mematikan.
func Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SIMARC_EMBEDDED_DB"))) {
	case "0", "false", "no", "off":
		return false
	}
	_, ok := Discover()
	return ok
}

// bin mengembalikan path sebuah program di dalam folder bin MariaDB.
func (m *Manager) bin(name string) string {
	return filepath.Join(m.BaseDir, "bin", exeName(name))
}

// Start memastikan database portabel sudah siap dan berjalan:
//  1. bila port sudah menerima koneksi (mis. dijalankan launcher lain) → selesai;
//  2. bila datadir belum ada → inisialisasi (mariadb-install-db);
//  3. jalankan mariadbd lalu tunggu siap;
//  4. selaraskan database & user aplikasi dengan nilai di .env (idempoten).
func (m *Manager) Start(ctx context.Context) error {
	m.prepareTools()
	if m.listening() {
		return nil // sudah berjalan (instance lain memegang port)
	}

	fresh, err := m.ensureDataDir(ctx)
	if err != nil {
		return err
	}
	if err := m.spawn(); err != nil {
		return err
	}
	m.owned = true
	if err := m.waitReady(ctx, m.readyTimeout()); err != nil {
		m.Stop()
		return err
	}
	// Bootstrap bersifat idempoten: menyelaraskan database/user/password dengan
	// nilai di .env. Ini penting bila .env dibuat ulang (sandi baru) sedangkan
	// datadir lama tetap ada. Pada datadir baru, kegagalan bersifat fatal.
	if err := m.bootstrap(ctx); err != nil {
		if fresh {
			m.Stop()
			return fmt.Errorf("bootstrap database portabel: %w", err)
		}
		// Datadir lama: biarkan aplikasi mencoba terhubung dengan kredensialnya.
	}
	return nil
}

// prepareTools membuat alias nama MySQL (mysqldump/mysql) dan menambahkan
// folder bin MariaDB ke PATH, supaya fitur backup/restore/offsite yang
// memanggil program bernama MySQL tetap berfungsi pada mode offline mandiri.
func (m *Manager) prepareTools() {
	binDir := filepath.Join(m.BaseDir, "bin")
	if fi, err := os.Stat(binDir); err != nil || !fi.IsDir() {
		return
	}
	aliases := map[string]string{
		"mysqldump": "mariadb-dump",
		"mysql":     "mariadb",
	}
	for alias, src := range aliases {
		link := filepath.Join(binDir, exeName(alias))
		if _, err := os.Stat(link); err == nil {
			continue
		}
		_ = copyFile(filepath.Join(binDir, exeName(src)), link)
	}
	path := os.Getenv("PATH")
	if !strings.Contains(path, binDir) {
		_ = os.Setenv("PATH", binDir+string(os.PathListSeparator)+path)
	}
}

// copyFile menyalin berkas sumber ke tujuan (menggantikan symlink/hardlink yang
// tidak selalu tersedia di Windows).
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// Stop mematikan database bila proses inilah yang menjalankannya. Dicoba
// shutdown rapi lewat mariadb-admin, lalu paksa mati sebagai cadangan.
func (m *Manager) Stop() {
	if !m.owned || m.cmd == nil || m.cmd.Process == nil {
		return
	}
	m.owned = false

	// Graceful: minta server menutup diri. Dipakai akun root karena selalu
	// memiliki hak SHUTDOWN; bila gagal (mis. sandi root tak terbaca), coba
	// akun aplikasi, lalu sebagai jalan terakhir paksa mati.
	shutdown := func(args []string) bool {
		admin := exec.Command(m.bin("mariadb-admin"), args...)
		admin.Dir = m.BaseDir
		return admin.Run() == nil
	}
	if !shutdown(append(m.rootArgs(), "shutdown")) {
		if m.User != "" {
			_ = shutdown([]string{
				"--protocol=TCP", "-h127.0.0.1", "-P" + m.Port,
				"-u" + m.User, "-p" + m.Pass, "shutdown",
			})
		}
	}

	// Beri kesempatan proses berhenti sendiri.
	select {
	case <-m.done:
	case <-time.After(m.stopGrace()):
		_ = m.cmd.Process.Kill()
		<-m.done
	}
}

func (m *Manager) readyTimeout() time.Duration {
	if m.ReadyTimeout > 0 {
		return m.ReadyTimeout
	}
	return 60 * time.Second
}

func (m *Manager) stopGrace() time.Duration {
	if m.StopGrace > 0 {
		return m.StopGrace
	}
	return 10 * time.Second
}

// listening melaporkan apakah sudah ada server yang mendengarkan di port.
func (m *Manager) listening() bool {
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+m.Port, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// ensureDataDir menginisialisasi datadir bila belum ada. Mengembalikan true
// bila datadir baru saja dibuat (perlu bootstrap user/database).
//
// Catatan: mariadb-install-db.exe (Windows) memakai opsi yang BERBEDA dari
// skrip mariadb-install-db (Unix) — Windows tidak mengenal --basedir /
// --auth-root-authentication-method / --skip-test-db. Di Windows, basedir
// disimpulkan otomatis dari lokasi biner, dan sandi root di-set lewat
// --password (disimpan agar bootstrap berikutnya tetap bisa login).
func (m *Manager) ensureDataDir(ctx context.Context) (bool, error) {
	marker := filepath.Join(m.DataDir, "mysql")
	if fi, err := os.Stat(marker); err == nil && fi.IsDir() {
		return false, nil // sudah pernah diinisialisasi
	}
	if err := os.MkdirAll(m.DataDir, 0o755); err != nil {
		return false, fmt.Errorf("buat folder data: %w", err)
	}

	rootPw := ""
	needPersist := false
	if runtime.GOOS == "windows" {
		if pw, ok := m.readRootPassword(); ok {
			rootPw = pw
		} else {
			pw, err := randomPassword(24)
			if err != nil {
				return false, err
			}
			rootPw = pw
			needPersist = true
		}
	}

	install := exec.CommandContext(ctx, m.bin("mariadb-install-db"),
		installDBArgs(runtime.GOOS, m.BaseDir, m.DataDir, m.Port, rootPw)...)
	install.Dir = m.BaseDir
	out, err := install.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("inisialisasi database (%v): %s", err, firstLines(out, 400))
	}

	// Sandi root disimpan SETELAH inisialisasi berhasil, agar datadir tidak
	// dianggap berisi berkas saat mariadb-install-db dijalankan.
	if needPersist {
		if err := os.WriteFile(m.rootPasswordFile(), []byte(rootPw+"\n"), 0o600); err != nil {
			return false, fmt.Errorf("simpan sandi root: %w", err)
		}
	}
	return true, nil
}

// installDBArgs menyusun argumen untuk program inisialisasi datadir. Dipisah
// sebagai fungsi murni agar perbedaan Windows/Unix dapat diuji tanpa menjalankan
// Windows.
//
// Windows (mariadb-install-db.exe) hanya mengenal --datadir/--port/--password;
// basedir disimpulkan dari lokasi biner. Unix (skrip mariadb-install-db) justru
// memakai --basedir dan tidak mengenal --port/--password.
func installDBArgs(goos, baseDir, dataDir, port, rootPw string) []string {
	if goos == "windows" {
		args := []string{"--datadir=" + dataDir, "--port=" + port}
		if rootPw != "" {
			args = append(args, "--password="+rootPw)
		}
		return args
	}
	return []string{
		"--basedir=" + baseDir,
		"--datadir=" + dataDir,
		"--auth-root-authentication-method=normal",
		"--skip-test-db",
	}
}

// rootPasswordFile menyimpan sandi root instance portabel (per datadir).
func (m *Manager) rootPasswordFile() string {
	return filepath.Join(m.DataDir, "simarc-root.pw")
}

// readRootPassword membaca sandi root yang tersimpan (bila ada).
func (m *Manager) readRootPassword() (string, bool) {
	b, err := os.ReadFile(m.rootPasswordFile())
	if err != nil {
		return "", false
	}
	s := strings.TrimSpace(string(b))
	return s, s != ""
}

// randomPassword menghasilkan sandi acak heksadesimal sepanjang n karakter.
func randomPassword(n int) (string, error) {
	buf := make([]byte, (n+1)/2)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("buat sandi acak: %w", err)
	}
	return hex.EncodeToString(buf)[:n], nil
}

// spawn menjalankan proses mariadbd dengan konfigurasi loopback-only.
func (m *Manager) spawn() error {
	if err := os.MkdirAll(m.DataDir, 0o755); err != nil {
		return fmt.Errorf("buat folder data: %w", err)
	}
	logFile, err := os.OpenFile(filepath.Join(m.DataDir, "error.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("buka log database: %w", err)
	}

	args := []string{
		"--no-defaults",
		"--basedir=" + m.BaseDir,
		"--datadir=" + m.DataDir,
		"--port=" + m.Port,
		"--bind-address=127.0.0.1",
		"--character-set-server=utf8mb4",
		"--collation-server=utf8mb4_unicode_ci",
		"--pid-file=" + filepath.Join(m.DataDir, "mysqld.pid"),
		"--console",
	}
	cmd := exec.Command(m.bin("mariadbd"), args...)
	cmd.Dir = m.BaseDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("jalankan database portabel: %w", err)
	}
	m.cmd = cmd
	m.done = make(chan struct{})
	go func() {
		m.waitErr = cmd.Wait()
		close(m.done)
	}()
	// logFile dibiarkan terbuka sampai proses anak berhenti; ditutup oleh OS.
	return nil
}

// waitReady menunggu port database menerima koneksi (atau proses mati).
func (m *Manager) waitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if m.listening() {
			return nil
		}
		select {
		case <-m.done:
			return fmt.Errorf("database portabel berhenti sendiri: %v (lihat %s)",
				m.waitErr, filepath.Join(m.DataDir, "error.log"))
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("database portabel tidak siap dalam %s (lihat %s)",
				timeout, filepath.Join(m.DataDir, "error.log"))
		}
	}
}

// bootstrap membuat database aplikasi dan user-nya pada datadir yang baru.
func (m *Manager) bootstrap(ctx context.Context) error {
	sql := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%[1]s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"+
			"CREATE USER IF NOT EXISTS '%[2]s'@'127.0.0.1' IDENTIFIED BY '%[3]s';"+
			"CREATE USER IF NOT EXISTS '%[2]s'@'localhost' IDENTIFIED BY '%[3]s';"+
			"ALTER USER '%[2]s'@'127.0.0.1' IDENTIFIED BY '%[3]s';"+
			"ALTER USER '%[2]s'@'localhost' IDENTIFIED BY '%[3]s';"+
			"GRANT ALL PRIVILEGES ON `%[1]s`.* TO '%[2]s'@'127.0.0.1';"+
			"GRANT ALL PRIVILEGES ON `%[1]s`.* TO '%[2]s'@'localhost';"+
			"GRANT SHUTDOWN ON *.* TO '%[2]s'@'127.0.0.1';"+
			"GRANT SHUTDOWN ON *.* TO '%[2]s'@'localhost';"+
			"FLUSH PRIVILEGES;",
		m.Name, m.User, m.Pass)

	args := append(m.rootArgs(), "-e", sql)

	client := exec.CommandContext(ctx, m.bin("mariadb"), args...)
	client.Dir = m.BaseDir
	out, err := client.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, firstLines(out, 400))
	}
	return nil
}

// rootArgs mengembalikan argumen koneksi klien sebagai root instance portabel.
//
// Root dipakai untuk tugas administratif (bootstrap, shutdown) karena selalu
// memiliki hak penuh; sandi root dibaca dari datadir (Windows) atau dikosongkan
// (Unix, root tanpa sandi). Koneksi memakai TCP ke 127.0.0.1 agar tunduk pada
// akun root@localhost/root@127.0.0.1 yang dibuat mariadb-install-db.
func (m *Manager) rootArgs() []string {
	args := []string{"--protocol=TCP", "-h127.0.0.1", "-P" + m.Port, "-uroot"}
	if pw, ok := m.readRootPassword(); ok {
		// -p<sandi> (tanpa spasi) agar tidak dipisah sebagai argumen lain.
		return append(args, "-p"+pw)
	}
	// Tanpa sandi root tersimpan: pakai koneksi tanpa sandi (Unix, root kosong).
	return append(args, "--skip-password")
}

// firstLines memendekkan keluaran perintah untuk pesan kesalahan.
func firstLines(b []byte, max int) string {
	s := strings.TrimSpace(string(b))
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}
