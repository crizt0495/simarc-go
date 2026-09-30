//go:build desktop

package main

import (
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// These tests cover the desktop shell's only contract with the rest of the
// application: the window is served the ordinary engine, rendered in
// top-navigation mode. They drive the real engine over real HTTP — including a
// real login against the real session store — so what they assert is exactly
// what the webview receives.
//
// Like the rest of the Go suite they need the MySQL server configured in .env.
// Credentials default to the seeded administrator and can be overridden with
// SIMARC_TEST_USER / SIMARC_TEST_PASS.
func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	if err := os.Chdir(projectRoot()); err != nil {
		log.Printf("[TEST] gagal pindah ke root proyek: %v", err)
	}
	os.Exit(m.Run())
}

// projectRoot walks up from the test's working directory until it finds the
// web/ tree, the same way the handlers suite locates it.
func projectRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return wd
	}
	dir := wd
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "web", "templates")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return wd
}

// newTestServer starts the shell's engine behind a real HTTP listener, which is
// what the Wails asset server does inside the window.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	r, err := newEngine()
	if err != nil {
		t.Fatalf("engine tidak bisa dibangun: %v", err)
	}
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

// newLoggedInClient returns a client holding a live session, having logged in
// over the same form the window uses — including the CSRF token the login page
// hands out.
func newLoggedInClient(t *testing.T, srv *httptest.Server) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("gagal membuat cookie jar: %v", err)
	}

	// Do not follow the post-login redirect; we only want the session cookie.
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// The login form is CSRF-protected: read the token from the page first.
	code, page := get(t, client, srv, "/login")
	if code != http.StatusOK {
		t.Fatalf("GET /login mengembalikan %d", code)
	}
	token := csrfToken(page)
	if token == "" {
		t.Fatal("token CSRF tidak ditemukan di halaman login")
	}

	user := envOr("SIMARC_TEST_USER", "admin")
	pass := envOr("SIMARC_TEST_PASS", "admin")

	resp, err := client.PostForm(srv.URL+"/login", url.Values{
		"_token":   {token},
		"username": {user},
		"password": {pass},
	})
	if err != nil {
		t.Fatalf("gagal POST /login: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("login %s/%s gagal (status %d, ingin 302) — set SIMARC_TEST_USER/SIMARC_TEST_PASS:\n%s",
			user, pass, resp.StatusCode, truncate(string(body), 300))
	}
	if len(jar.Cookies(resp.Request.URL)) == 0 {
		t.Fatal("login berhasil tetapi tidak menghasilkan cookie sesi")
	}

	// From here on redirects are fine.
	client.CheckRedirect = nil
	return client
}

// csrfTokenRE matches the hidden _token input in either attribute order.
var csrfTokenRE = regexp.MustCompile(
	`name="_token"\s+value="([^"]+)"` + `|value="([^"]+)"\s+name="_token"`)

// csrfToken pulls the hidden _token value out of a rendered login form.
func csrfToken(page string) string {
	m := csrfTokenRE.FindStringSubmatch(page)
	if m == nil {
		return ""
	}
	if m[1] != "" {
		return m[1]
	}
	return m[2]
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// get performs an authenticated GET and returns the status and body.
func get(t *testing.T, client *http.Client, srv *httptest.Server, path string) (int, string) {
	t.Helper()
	resp, err := client.Get(srv.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("baca body %s: %v", path, err)
	}
	return resp.StatusCode, string(body)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// styleBlock and scriptBlock match the contents of a <style>/<script> element.
// Their text is not markup, so a selector or a string mentioning "sidebar" in
// there must not count as a rendered sidebar. (The laporan/arsip print
// stylesheet legitimately lists .sidebar-overlay among the rules it hides.)
var (
	styleBlock  = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	scriptBlock = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
)

// markup strips style and script bodies so assertions describe what the page
// actually renders.
func markup(page string) string {
	page = styleBlock.ReplaceAllString(page, "<style></style>")
	return scriptBlock.ReplaceAllString(page, "<script></script>")
}

// TestDesktopShellServesTopNavigation is the central guarantee of this file:
// every page the window loads carries the horizontal top bar and never the
// left sidebar.
func TestDesktopShellServesTopNavigation(t *testing.T) {
	srv := newTestServer(t)
	client := newLoggedInClient(t, srv)

	// One or two real destinations from each of the six top-bar groups, so a
	// regression in any group's dropdown is caught.
	pages := []string{
		"/dashboard",          // Dashboard (direct link)
		"/arsip",              // Manajemen Arsip
		"/pemberkasan",        //
		"/arsip/pemindahan",   //
		"/peminjaman",         // Layanan
		"/unit-kerja",         // Master Data
		"/lokasi-arsip",       //
		"/laporan",            // Laporan
		"/laporan/arsip",      //
		"/monitoring/retensi", // Lainnya
		"/jadwal-retensi",     //
		"/roles",              //
		"/pengaturan",         //
	}

	for _, path := range pages {
		t.Run(path, func(t *testing.T) {
			code, body := get(t, client, srv, path)
			if code != http.StatusOK {
				t.Fatalf("halaman %s mengembalikan %d", path, code)
			}
			doc := markup(body)

			if !strings.Contains(doc, "layout-topbar") {
				t.Errorf("%s tidak memakai shell topbar (layout-topbar tidak ada)", path)
			}
			if !strings.Contains(doc, "top-navbar") {
				t.Errorf("%s tidak memuat top-navbar", path)
			}
			if !strings.Contains(doc, "topnav-menu") {
				t.Errorf("%s tidak memuat menu topnav", path)
			}
			// The sidebar and its mobile overlay belong to the web shell only.
			if strings.Contains(doc, `class="sidebar"`) {
				t.Errorf("%s masih merender sidebar", path)
			}
			if strings.Contains(doc, "sidebar-overlay") {
				t.Errorf("%s masih merender overlay sidebar", path)
			}
			// The shared header controls must survive the switch.
			if !strings.Contains(doc, "themeToggle") {
				t.Errorf("%s kehilangan tombol tema", path)
			}
			if !strings.Contains(doc, "nav-user") {
				t.Errorf("%s kehilangan menu user", path)
			}
		})
	}
}

// TestDesktopShellExposesEveryTopNavGroup checks all five primary groups plus
// the overflow, so a broken dropdown is caught before it ships.
func TestDesktopShellExposesEveryTopNavGroup(t *testing.T) {
	srv := newTestServer(t)
	client := newLoggedInClient(t, srv)

	code, body := get(t, client, srv, "/dashboard")
	if code != http.StatusOK {
		t.Fatalf("GET /dashboard mengembalikan %d", code)
	}
	doc := markup(body)

	groups := []string{
		"Dashboard", "Manajemen Arsip", "Layanan", "Master Data", "Laporan", "Lainnya",
	}
	for _, g := range groups {
		if !strings.Contains(doc, g) {
			t.Errorf("grup topnav %q tidak ada di /dashboard", g)
		}
	}
}

// TestDesktopShellLoginPageIsServed confirms the window's very first request
// resolves: Wails loads "/" and needs a 200 HTML document from it, not a
// redirect. Wails discards a redirect's body, and the window then never paints.
func TestDesktopShellLoginPageIsServed(t *testing.T) {
	srv := newTestServer(t)

	code, body := get(t, &http.Client{}, srv, "/")
	if code != http.StatusOK {
		t.Fatalf("GET / mengembalikan %d, ingin 200 (Wails butuh dokumen, bukan redirect)", code)
	}
	doc := markup(body)
	if !strings.Contains(doc, "<form") {
		t.Error("halaman / tidak memuat form login")
	}
	if !strings.Contains(doc, "password") {
		t.Error("halaman / tidak memuat input password")
	}
}

// TestInstrumentIsOptIn makes sure the diagnostics cannot slow down or alter a
// normal session: with nothing enabled the handler is passed straight through,
// and with logging on the response still comes through unchanged.
func TestInstrumentIsOptIn(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	t.Run("disabled passes the handler through untouched", func(t *testing.T) {
		t.Setenv("SIMARC_DESKTOP_LOG", "")
		t.Setenv("SIMARC_DESKTOP_DUMP", "")

		got := instrument(inner)
		if reflect.ValueOf(got).Pointer() != reflect.ValueOf(inner).Pointer() {
			t.Error("instrument harus mengembalikan handler yang sama saat diagnostics mati")
		}
	})

	t.Run("enabled still serves the body unchanged", func(t *testing.T) {
		t.Setenv("SIMARC_DESKTOP_LOG", "1")
		t.Setenv("SIMARC_DESKTOP_DUMP", "")

		rec := httptest.NewRecorder()
		instrument(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

		if rec.Body.String() != "ok" {
			t.Errorf("body berubah saat logging aktif: %q", rec.Body.String())
		}
		if rec.Code != http.StatusOK {
			t.Errorf("status berubah saat logging aktif: %d", rec.Code)
		}
	})

	t.Run("dump writes the first document only", func(t *testing.T) {
		dump := filepath.Join(t.TempDir(), "dom.html")
		t.Setenv("SIMARC_DESKTOP_LOG", "")
		t.Setenv("SIMARC_DESKTOP_DUMP", dump)

		page := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte("<html>" + r.URL.Path + "</html>"))
		})

		h := instrument(page)
		for _, path := range []string{"/satu", "/dua"} {
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
		}

		data, err := os.ReadFile(dump)
		if err != nil {
			t.Fatalf("dump tidak ditulis: %v", err)
		}
		if !strings.Contains(string(data), "/satu") {
			t.Errorf("dump harus berisi halaman pertama, isinya: %q", data)
		}
		if strings.Contains(string(data), "/dua") {
			t.Error("dump hanya boleh berisi satu halaman")
		}
	})
}
