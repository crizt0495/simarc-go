// Command server runs SIMARC as a standalone HTTP server (local / VPS).
// The server binds to all interfaces so any client on the same WiFi/LAN can
// reach it; the LAN IP printed in the banner is auto-detected (or overridden
// via the LAN_IP environment variable).
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"arsippro/internal/app"
	"arsippro/internal/config"
	"arsippro/internal/database"
	"arsippro/internal/embeddeddb"
	"arsippro/internal/handlers"
)

// isVirtualIface reports whether the interface is a known virtual, container,
// VPN, tunnel or VM bridge that should never be advertised as the LAN address.
func isVirtualIface(name string) bool {
	lower := strings.ToLower(name)
	skips := []string{
		"lo", "docker", "veth", "br-", "virbr", "vmnet", "vboxnet",
		"tailscale", "tun", "tap", "wg", "zt", "ppp", "sit",
	}
	for _, s := range skips {
		if strings.HasPrefix(lower, s) {
			return true
		}
	}
	return false
}

// isPrivateIPv4 reports whether ip is in an RFC1918 private range
// (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16) — i.e. a real WiFi/LAN address.
func isPrivateIPv4(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil {
		return false
	}
	return ip[0] == 10 ||
		(ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31) ||
		(ip[0] == 192 && ip[1] == 168)
}

// lanIPCandidates enumerates all up, non-loopback, non-virtual interfaces and
// returns their IPv4 addresses ordered by preference: private (RFC1918) LAN
// addresses first, then any other routable address.
func lanIPCandidates() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var preferred, other []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if isVirtualIface(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			ip4 := ip.To4()
			if ip4 == nil || ip4.IsLoopback() {
				continue
			}
			s := ip4.String()
			if isPrivateIPv4(ip4) {
				preferred = append(preferred, s)
			} else {
				other = append(other, s)
			}
		}
	}
	return append(preferred, other...)
}

// getLANIP returns the best LAN/WiFi IPv4 address to advertise.
// A LAN_IP environment variable always wins (useful on machines with many
// interfaces or for deployments). Otherwise the first private IPv4 from a
// real (non-virtual) interface is used; it falls back to any non-loopback
// IPv4, and finally to 127.0.0.1. This never depends on internet access,
// unlike the previous UDP-dial-to-8.8.8.8 approach.
func getLANIP() string {
	if override := strings.TrimSpace(os.Getenv("LAN_IP")); override != "" {
		return override
	}
	if ips := lanIPCandidates(); len(ips) > 0 {
		return ips[0]
	}
	return "127.0.0.1"
}

// bannerLine centers text inside a box of the given inner width.
func bannerLine(text string, width int) string {
	if len(text) > width {
		text = text[:width]
	}
	padding := width - len(text)
	left := padding / 2
	right := padding - left
	return "  ║" + strings.Repeat(" ", left) + text + strings.Repeat(" ", right) + "║"
}

// printBanner prints the startup box with local/network URLs, auto-detected
// LAN IP, and the active database (MySQL/MariaDB).
func printBanner(port, lanIP string) {
	w := 60
	bar := func(r rune) string { return "  " + string(r) + strings.Repeat("═", w) + string(r) }

	fmt.Println()
	fmt.Println(bar('╔'))
	fmt.Println(bannerLine("S I M A R C", w))
	fmt.Println(bannerLine("Sistem Informasi Manajemen Arsip Record Center", w))
	fmt.Println(bannerLine("", w))
	fmt.Println(bar('╠'))
	fmt.Println(bannerLine(fmt.Sprintf("Local     : http://127.0.0.1:%s", port), w))
	fmt.Println(bannerLine(fmt.Sprintf("Network   : http://%s:%s", lanIP, port), w))
	if database.IsSQLite() {
		fmt.Println(bannerLine(fmt.Sprintf("Database  : SQLite (%s)", config.App.DBPath), w))
	} else {
		fmt.Println(bannerLine(fmt.Sprintf("Database  : MySQL  (%s:%s/%s)", config.App.DBHost, config.App.DBPort, config.App.DBName), w))
	}
	fmt.Println(bannerLine("", w))
	fmt.Println(bannerLine("Client di WiFi/LAN yang sama bisa akses:", w))
	fmt.Println(bannerLine(fmt.Sprintf("-> http://%s:%s", lanIP, port), w))
	fmt.Println(bannerLine("", w))
	fmt.Println(bannerLine("Tekan Ctrl+C untuk berhenti", w))
	fmt.Println(bar('╚'))
	fmt.Println()
}

// openBrowser opens the application URL. In desktop app mode it returns the
// started app-window process so the caller can stop the server when the window
// is closed; otherwise it returns nil.
func openBrowser(url string) *exec.Cmd {
	// When running under the desktop launcher, the app window is opened by
	// the launcher itself (standalone Chromium window). Skip the browser.
	if os.Getenv("SIMARC_NO_BROWSER") != "" {
		return nil
	}
	// Desktop app mode: open as a standalone app window (no tabs / URL bar).
	if isTruthy(os.Getenv("SIMARC_APP_WINDOW")) {
		if cmd, ok := openAppWindow(url); ok {
			return cmd
		}
	}
	openDefaultBrowser(url)
	return nil
}

// openDefaultBrowser opens url in the operating system's default browser.
func openDefaultBrowser(url string) {
	switch runtime.GOOS {
	case "windows":
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		cmd := exec.Command("open", url)
		cmd.SysProcAttr = browserProcAttr()
		cmd.Start()
	case "linux":
		cmd := exec.Command("xdg-open", url)
		cmd.SysProcAttr = browserProcAttr()
		cmd.Start()
	}
}

// isTruthy reports whether an environment value should be treated as enabled.
func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// openAppWindow launches the URL in a standalone Chromium app window
// (no tabs / address bar). It returns the started process (and true) so the
// caller can stop the server when the window closes; it returns false when no
// Chromium browser is found so the caller can fall back to the default browser.
func openAppWindow(url string) (*exec.Cmd, bool) {
	bin := findChromiumBrowser()
	if bin == "" {
		return nil, false
	}
	profile := filepath.Join(userCacheDir(), "simarc", "app-profile")
	_ = os.MkdirAll(profile, 0o755)
	args := []string{
		"--app=" + url,
		"--user-data-dir=" + profile,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-translate",
		"--class=SIMARC",
		"--name=SIMARC",
	}
	// Mode jendela: normal | maximized | fullscreen | kiosk (default: fullscreen)
	windowMode := strings.ToLower(strings.TrimSpace(os.Getenv("SIMARC_WINDOW")))
	if windowMode == "" {
		windowMode = "fullscreen"
	}
	switch windowMode {
	case "maximized":
		args = append(args, "--start-maximized")
	case "fullscreen":
		args = append(args, "--start-fullscreen")
	case "kiosk":
		args = append(args, "--kiosk")
	}
	cmd := exec.Command(bin, args...)
	if err := cmd.Start(); err != nil {
		return nil, false
	}
	return cmd, true
}

// findChromiumBrowser returns the path of an installed Chromium-based browser,
// or an empty string when none is available.
func findChromiumBrowser() string {
	for _, name := range []string{
		"google-chrome", "google-chrome-stable", "chromium", "chromium-browser",
		"brave-browser", "microsoft-edge", "chrome", "msedge",
	} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
			base := os.Getenv(env)
			if base == "" {
				continue
			}
			candidates = append(candidates,
				filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"),
			)
		}
	case "darwin":
		candidates = append(candidates,
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		)
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

// userCacheDir returns a per-user cache directory with portable fallbacks.
func userCacheDir() string {
	if d, err := os.UserCacheDir(); err == nil && d != "" {
		return d
	}
	if d, err := os.UserHomeDir(); err == nil && d != "" {
		return filepath.Join(d, ".cache")
	}
	return os.TempDir()
}

func main() {
	// Load configuration first (this also loads .env) so SIMARC_SERVER_URL is
	// available before we decide whether to run as a server or as a client.
	config.Load()

	// ── Client mode ──────────────────────────────────────────────────────────
	// When SIMARC_SERVER_URL is set, this installation acts as a thin client:
	// it does NOT start a local HTTP server and does NOT connect to a database.
	// It only opens the desktop app window pointing to a central SIMARC server,
	// so many computers can share one database and one file storage.
	if serverURL := strings.TrimSpace(os.Getenv("SIMARC_SERVER_URL")); serverURL != "" {
		serverURL = strings.TrimRight(serverURL, "/")
		log.Printf("Mode klien: membuka %s", serverURL)
		if _, ok := openAppWindow(serverURL); !ok {
			openDefaultBrowser(serverURL)
		}
		return
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = config.App.AppPort
	}
	addr := ":" + port
	lanIP := getLANIP()

	// HTTPS (opsional): diperlukan agar PWA bisa di-install di Android
	// (Chrome Android menolak install PWA pada HTTP non-localhost).
	// Aktifkan dengan APP_HTTPS=true. Sertifikat bisa disediakan lewat
	// APP_CERT_FILE/APP_KEY_FILE; bila tidak ada, dibuat self-signed otomatis.
	// Catatan: self-signed tidak dipercaya Android tanpa instalasi manual —
	// untuk produksi gunakan reverse proxy (Caddy/nginx) dengan Let's Encrypt
	// atau tunnel (Cloudflare/ngrok) yang memberi HTTPS terpercaya.
	useHTTPS := strings.EqualFold(strings.TrimSpace(os.Getenv("APP_HTTPS")), "true") ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("APP_HTTPS")), "1")
	certFile := strings.TrimSpace(os.Getenv("APP_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("APP_KEY_FILE"))

	// ── Port & jendela aplikasi dibuka LEBIH DULU ────────────────────────────
	// Inisialisasi database portabel (instalasi pertama) dan sinkronisasi awal
	// dapat memerlukan puluhan detik sampai beberapa menit. Karena itu port
	// dibuka dan jendela aplikasi ditampilkan lebih dulu, dengan halaman
	// "Menyiapkan data SIMARC" sampai aplikasi benar-benar siap. Tanpa ini
	// pengguna melihat layar kosong / "site can't be reached" dan mengira
	// aplikasi tidak terbuka.
	// Nilai disimpan dalam struct agar tipe konkretnya konsisten
	// (http.HandlerFunc vs *gin.Engine tidak boleh bergantian di atomic.Value).
	type handlerBox struct{ h http.Handler }
	var handler atomic.Value
	handler.Store(handlerBox{app.PreparingHandler()})
	srv := &http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			handler.Load().(handlerBox).h.ServeHTTP(w, req)
		}),
	}

	ln, err := listenWithRetry(addr, 20)
	if err != nil {
		// Port dipakai, tetapi kemungkinan besar oleh SIMARC yang SUDAH
		// berjalan (mis. pengguna mengklik shortcut lagi). Dalam hal ini
		// jangan tampilkan galat — cukup buka jendela aplikasi yang ada.
		if existing := existingSimarcURL(port); existing != "" {
			log.Printf("[OK] SIMARC sudah berjalan - membuka jendela aplikasi.")
			if _, ok := openAppWindow(existing); !ok {
				openDefaultBrowser(existing)
			}
			os.Exit(exitAlreadyRunning)
		}
		log.Fatalf("Gagal membuka port %s: %v (ubah APP_PORT di .env bila port dipakai program lain)", port, err)
	}

	var appCmd *exec.Cmd
	if config.IsVercel() {
		log.Printf("Vercel deployment detected, listening on :%s", port)
	} else {
		scheme := "http"
		if useHTTPS {
			scheme = "https"
		}
		printBanner(port, lanIP)
		appCmd = openBrowser(fmt.Sprintf("%s://127.0.0.1:%s", scheme, port))
	}

	go func() {
		if useHTTPS {
			if certFile == "" || keyFile == "" {
				cf, kf, err := ensureSelfSignedCert()
				if err != nil {
					log.Fatalf("Gagal membuat sertifikat TLS: %v", err)
				}
				certFile, keyFile = cf, kf
			}
			log.Printf("SIMARC starting on %s (HTTPS, LAN: https://%s:%s)", addr, lanIP, port)
			if err := srv.ServeTLS(ln, certFile, keyFile); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Server error: %v", err)
			}
		} else {
			log.Printf("SIMARC starting on %s (LAN: http://%s:%s)", addr, lanIP, port)
			if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Server error: %v", err)
			}
		}
	}()

	// ── Database portabel (mode offline mandiri) ─────────────────────────────
	// Bila installer desktop menyertakan MariaDB portabel di samping aplikasi,
	// jalankan sebagai database lokal komputer ini supaya tetap bisa dipakai
	// tanpa server pusat (dan tanpa memasang MySQL manual). Matikan dengan
	// SIMARC_EMBEDDED_DB=0. Lihat internal/embeddeddb.
	var mdb *embeddeddb.Manager
	if embeddeddb.Enabled() {
		base, _ := embeddeddb.Discover()
		mdb = embeddeddb.New(base, "", config.App.DBName, config.App.DBUser, config.App.DBPass, config.App.DBPort)
		if err := mdb.Start(context.Background()); err != nil {
			log.Printf("[WARN] Database portabel gagal dijalankan: %v", err)
		} else {
			log.Printf("Database portabel aktif di 127.0.0.1:%s", config.App.DBPort)
			defer mdb.Stop()
		}
	}

	r, err := app.Init()
	if err != nil {
		if mdb != nil {
			mdb.Stop()
		}
		log.Fatalf("Gagal menginisialisasi aplikasi: %v", err)
	}
	// Aplikasi siap: ganti halaman splash dengan router sebenarnya. Halaman
	// "Menyiapkan data" tetap tampil dari dalam app sampai baseline selesai.
	handler.Store(handlerBox{r})

	// Auto-update: periodically check the hub for a newer binary and download
	// it to tmp/simarc-server.exe.new. It is applied on the next restart by
	// run.bat, so the running process is never replaced while in use.
	go autoUpdateCheck(port)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	// Desktop app: when the app window is closed, stop the server automatically
	// (gives Windows/macOS the same behaviour the Linux launcher already has).
	if appCmd != nil {
		go func() {
			_ = appCmd.Wait()
			log.Println("Jendela aplikasi ditutup, menghentikan server...")
			quit <- syscall.SIGTERM
		}()
	}

	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited gracefully.")
}

// exitAlreadyRunning dipakai server saat port sudah dipakai oleh instance
// SIMARC lain. run.bat memakai kode ini agar tidak menganggapnya kegagalan dan
// tidak mematikan database portabel milik instance yang sedang berjalan.
const exitAlreadyRunning = 75

// existingSimarcURL mengembalikan URL SIMARC yang sudah berjalan di port ini
// (bila /ping merespons), atau string kosong bila port dipakai program lain.
func existingSimarcURL(port string) string {
	base := "http://127.0.0.1:" + port
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get(base + "/ping")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	text := string(body)
	// Saat masih menyiapkan data, /ping menjawab halaman splash SIMARC;
	// setelah siap menjawab {"status":"ok"}. Keduanya menandakan SIMARC.
	if !strings.Contains(text, "SIMARC") && !strings.Contains(text, `"status"`) {
		return ""
	}
	return base
}

// listenWithRetry membuka port TCP dengan beberapa percobaan. Ini menangani
// kasus peluncuran ulang tepat setelah aplikasi ditutup: Windows kadang belum
// melepaskan port lama sehingga percobaan pertama gagal ("Only one usage of
// each socket address..."). Total tunggu ± (attempts-1) × 500 ms.
func listenWithRetry(addr string, attempts int) (net.Listener, error) {
	var err error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		var ln net.Listener
		if ln, err = net.Listen("tcp", addr); err == nil {
			if i > 0 {
				log.Printf("Port %s berhasil dibuka setelah %d percobaan.", addr, i+1)
			}
			return ln, nil
		}
	}
	return nil, err
}

// autoUpdateCheck periodically asks the hub whether a newer application binary
// is available. When one is, it is downloaded into tmp/simarc-server.exe.new and
// a small marker file is written; run.bat swaps it in on the next start, so the
// running process is never replaced while in use. The check is best-effort: any
// error (hub unreachable, offline, no token) is logged and retried later.
func autoUpdateCheck(localPort string) {
	// Give the server time to finish booting.
	time.Sleep(30 * time.Second)

	cfg := config.LoadSync()
	if !cfg.FilesReady() {
		log.Printf("[update] auto-update dinonaktifkan (SYNC_HUB_URL/TOKEN kosong)")
		return
	}
	hub := cfg.HubURL
	token := cfg.HubToken
	localVersion := handlers.AppVersion()

	// Resolve the binary path the same way the rest of the app does.
	exe, err := os.Executable()
	if err != nil {
		log.Printf("[update] tidak bisa menentukan path binary: %v", err)
		return
	}
	dir := filepath.Dir(exe)
	if filepath.Base(dir) == "tmp" {
		dir = filepath.Dir(dir)
	}
	newPath := filepath.Join(dir, "tmp", "simarc-server.exe.new")
	markerPath := filepath.Join(dir, "tmp", "update-pending")

	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	check := func() {
		// 1. Ask the hub for its version.
		req, err := http.NewRequest(http.MethodGet, hub+"/api/update/version", nil)
		if err != nil {
			return
		}
		req.Header.Set("X-Sync-Token", token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("[update] hub tidak terjangkau: %v", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return
		}
		var info struct {
			Version string `json:"version"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
			return
		}
		if info.Version == "" || info.Version == localVersion {
			return
		}
		log.Printf("[update] versi baru tersedia: %s (lokal: %s) — mengunduh...", info.Version, localVersion)

		// 2. Download the new binary.
		dreq, err := http.NewRequest(http.MethodGet, hub+"/api/update/binary", nil)
		if err != nil {
			return
		}
		dreq.Header.Set("X-Sync-Token", token)
		dresp, err := http.DefaultClient.Do(dreq)
		if err != nil {
			log.Printf("[update] gagal mengunduh: %v", err)
			return
		}
		defer dresp.Body.Close()
		if dresp.StatusCode != http.StatusOK {
			log.Printf("[update] unduhan gagal: HTTP %d", dresp.StatusCode)
			return
		}
		f, err := os.Create(newPath)
		if err != nil {
			log.Printf("[update] gagal membuat file .new: %v", err)
			return
		}
		if _, err := io.Copy(f, dresp.Body); err != nil {
			f.Close()
			os.Remove(newPath)
			log.Printf("[update] gagal menyimpan binary: %v", err)
			return
		}
		f.Close()
		_ = os.WriteFile(markerPath, []byte(info.Version), 0o644)
		log.Printf("[update] pembaruan %s diunduh. Restart aplikasi untuk menerapkan (run.bat akan menukar binary otomatis).", info.Version)
	}

	check()
	for range ticker.C {
		check()
	}
}

// ensureSelfSignedCert generates a self-signed TLS certificate (cert.pem +
// key.pem) in the project root when none is provided. It is intended for
// development and LAN testing only — production deployments should use a
// trusted certificate (Let's Encrypt / reverse proxy).
func ensureSelfSignedCert() (string, string, error) {
	certFile := "cert.pem"
	keyFile := "key.pem"

	// Reuse an existing pair if present.
	if _, err := os.Stat(certFile); err == nil {
		if _, err := os.Stat(keyFile); err == nil {
			return certFile, keyFile, nil
		}
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}

	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"SIMARC (self-signed)"},
			CommonName:   "SIMARC Local",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return "", "", err
	}

	certOut, err := os.Create(certFile)
	if err != nil {
		return "", "", err
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		certOut.Close()
		return "", "", err
	}
	certOut.Close()

	keyOut, err := os.Create(keyFile)
	if err != nil {
		return "", "", err
	}
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		keyOut.Close()
		return "", "", err
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}); err != nil {
		keyOut.Close()
		return "", "", err
	}
	keyOut.Close()

	log.Printf("[HTTPS] Sertifikat self-signed dibuat: %s, %s", certFile, keyFile)
	return certFile, keyFile, nil
}
