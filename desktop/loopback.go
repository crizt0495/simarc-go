//go:build desktop

package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// The webview needs a real HTTP origin, not Wails' built-in one.
//
// Wails starts the window at `wails://wails/` on Linux and macOS (and only
// uses `http://wails.localhost/` on Windows). WebKit does not keep cookies for
// a custom URI scheme, so on Linux and macOS the session cookie was silently
// dropped: the server sent `Set-Cookie: simarc_session=…` and the webview never
// sent it back. Every login then failed CSRF validation with a 403, and the
// page after a successful-looking sign-in came up blank because no request
// carried a session.
//
// The fix is to give the window an ordinary loopback origin. We bind the same
// engine to 127.0.0.1 on an ephemeral port and hand the window a tiny page
// that navigates there; from that point on everything — pages, assets, forms,
// downloads — is plain HTTP, and cookies, sessions and CSRF behave exactly as
// they do in the browser build. Nothing is exposed beyond the loopback
// interface, and the port is chosen by the kernel per run.

// loopbackServer serves the application over http://127.0.0.1:<port>.
type loopbackServer struct {
	listener net.Listener
	server   *http.Server
	url      string
}

// serveOnLoopback binds the handler to a free port on 127.0.0.1 and serves it
// in the background.
func serveOnLoopback(handler http.Handler) (*loopbackServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("tidak bisa membuka port loopback: %w", err)
	}

	// A desktop window is long-lived, but a hung request must never wedge the
	// UI thread, so bound how long a single request may take.
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 15 * time.Second,
	}

	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("[desktop] server loopback berhenti: %v", err)
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	return &loopbackServer{
		listener: ln,
		server:   srv,
		url:      fmt.Sprintf("http://127.0.0.1:%d", addr.Port),
	}, nil
}

// close shuts the loopback listener down.
func (s *loopbackServer) close() {
	if s == nil || s.server == nil {
		return
	}
	_ = s.server.Close()
}

// bootstrapPage is the only thing the Wails asset server ever returns.
//
// It exists purely to move the window off the `wails://` origin and onto the
// loopback origin, after which the asset server is no longer involved at all.
func bootstrapPage(target string) http.Handler {
	const page = `<!DOCTYPE html>
<html lang="id">
<head>
<meta charset="utf-8">
<title>SIMARC</title>
<meta http-equiv="refresh" content="0; url=%[1]s">
<style>
  html,body{height:100%%;margin:0}
  body{display:flex;align-items:center;justify-content:center;
       font:15px/1.5 system-ui,sans-serif;color:#5b6472;background:#fff}
</style>
</head>
<body>
  <p>Memuat SIMARC&hellip;</p>
  <noscript>
    <p>SIMARC memerlukan JavaScript. Buka
       <a href="%[1]s">%[1]s</a> secara manual.</p>
  </noscript>
  <script>location.replace(%[2]q);</script>
</body>
</html>
`
	safe := strings.ReplaceAll(target, `"`, "%22")
	html := fmt.Sprintf(page, safe, safe)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// The bootstrap is generated per run; never let it be cached.
		w.Header().Set("Cache-Control", "no-store, max-age=0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	})
}
