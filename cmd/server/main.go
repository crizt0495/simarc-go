// Command server runs SIMARC as a standalone HTTP server (local / VPS).
// The server binds to all interfaces so any client on the same WiFi/LAN can
// reach it; the LAN IP printed in the banner is auto-detected (or overridden
// via the LAN_IP environment variable).
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"arsippro/internal/app"
	"arsippro/internal/config"
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
	fmt.Println(bannerLine(fmt.Sprintf("Database  : MySQL  (%s:%s/%s)", config.App.DBHost, config.App.DBPort, config.App.DBName), w))
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
	// Mode jendela: normal (default) | maximized | fullscreen | kiosk
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SIMARC_WINDOW"))) {
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

	r, err := app.Init()
	if err != nil {
		log.Fatalf("Gagal menginisialisasi aplikasi: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = config.App.AppPort
	}
	addr := ":" + port
	lanIP := getLANIP()

	var appCmd *exec.Cmd
	if config.IsVercel() {
		log.Printf("Vercel deployment detected, listening on :%s", port)
	} else {
		printBanner(port, lanIP)
		// Open the local app window on the loopback address (most reliable);
		// client machines still use the LAN URL shown in the banner.
		appCmd = openBrowser(fmt.Sprintf("http://127.0.0.1:%s", port))
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		log.Printf("SIMARC starting on %s (LAN: http://%s:%s)", addr, lanIP, port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

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
