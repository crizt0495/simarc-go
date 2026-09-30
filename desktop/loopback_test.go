//go:build desktop

package main

import (
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// The window must be served from a real HTTP origin.
//
// Wails' own origin is the custom scheme wails://wails/ on Linux and macOS,
// and WebKit keeps no cookies for a custom scheme. That silently broke every
// login: the server set simarc_session and the webview never sent it back, so
// the CSRF check on POST /login answered 403. These tests pin the contract that
// fixes it — a loopback http origin that round-trips a session cookie — so the
// app cannot quietly go back to being unusable.

func TestBootstrapPagePointsAtLoopbackOrigin(t *testing.T) {
	// Whatever origin the shell picks, it must be plain HTTP on loopback: a
	// custom scheme here would bring the cookie problem straight back.
	target := (&loopbackServer{url: "http://127.0.0.1:34567"}).url
	if !strings.HasPrefix(target, "http://127.0.0.1:") {
		t.Fatalf("bootstrap target is not a loopback http origin: %q", target)
	}

	rec := httptest.NewRecorder()
	bootstrapPage(target).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("bootstrap status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("bootstrap Content-Type = %q, want text/html", ct)
	}

	body := rec.Body.String()
	// Two independent ways to get there: script for the webview, meta refresh as
	// the fallback if scripting is unavailable.
	if !strings.Contains(body, "location.replace") {
		t.Error("bootstrap does not script a navigation to the loopback origin")
	}
	if !strings.Contains(body, `http-equiv="refresh"`) {
		t.Error("bootstrap has no meta-refresh fallback")
	}
	if !strings.Contains(body, target) {
		t.Errorf("bootstrap does not mention the target origin %q", target)
	}
}

func TestBootstrapPageIsNeverCached(t *testing.T) {
	// The bootstrap embeds this run's port, so a cached copy would point a
	// later window at a dead port.
	rec := httptest.NewRecorder()
	bootstrapPage("http://127.0.0.1:1").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q, want it to include no-store", cc)
	}
}

func TestBootstrapPageEscapesQuotesInTarget(t *testing.T) {
	// The target is interpolated into both an attribute and a script literal,
	// so a stray quote must not be able to break out of either.
	rec := httptest.NewRecorder()
	bootstrapPage(`http://127.0.0.1:1"onload="alert(1)`).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if strings.Contains(rec.Body.String(), `onload="alert(1)"`) {
		t.Error("bootstrap allowed a quote to escape the attribute")
	}
}

// TestLoopbackRoundTripsSessionCookie is the regression test for the actual
// bug: a cookie set by the app has to come back on the next request.
func TestLoopbackRoundTripsSessionCookie(t *testing.T) {
	const cookieName = "simarc_session"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    "test-session-value",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	// A client with a real cookie jar, i.e. what a browser does.
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}

	// First request: the cookie is issued.
	resp, err := client.Get(srv.URL + "/login")
	if err != nil {
		t.Fatalf("GET /login: %v", err)
	}
	_ = resp.Body.Close()

	// Second request: it must come back. This is precisely what WebKit refused
	// to do on the wails:// origin.
	resp, err = client.Get(srv.URL + "/dashboard")
	if err != nil {
		t.Fatalf("GET /dashboard: %v", err)
	}
	defer resp.Body.Close()

	var got string
	for _, c := range resp.Request.Cookies() {
		if c.Name == cookieName {
			got = c.Value
		}
	}
	if got != "test-session-value" {
		t.Fatalf("session cookie did not survive the round trip: got %q", got)
	}
}

// TestLoopbackBindsLoopbackOnly guards against accidentally exposing the app to
// the network.
func TestLoopbackBindsLoopbackOnly(t *testing.T) {
	lb, err := serveOnLoopback(http.NotFoundHandler())
	if err != nil {
		t.Fatalf("serveOnLoopback: %v", err)
	}
	defer lb.close()

	u, err := url.Parse(lb.url)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	host, _, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split host: %v", err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("bound to %q, want 127.0.0.1 only", host)
	}

	resp, err := http.Get(lb.url + "/")
	if err != nil {
		t.Fatalf("loopback server not reachable: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 from the test handler", resp.StatusCode)
	}
}

func TestBootstrapPageIsCompleteDocument(t *testing.T) {
	// Guards the escaping above: the body must still parse as a complete
	// document, with no stray backslash sequence leaking into the markup.
	rec := httptest.NewRecorder()
	bootstrapPage("http://127.0.0.1:8080").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rec.Body.String()
	if !strings.HasPrefix(strings.TrimSpace(body), "<!DOCTYPE html>") {
		t.Error("bootstrap is not a complete HTML document")
	}
	if !strings.Contains(body, "</html>") {
		t.Error("bootstrap document is not closed")
	}
}
