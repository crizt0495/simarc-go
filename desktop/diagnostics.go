//go:build desktop

package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Diagnostics for the desktop shell.
//
// The window talks to the gin engine over an in-process transport, so when
// something misbehaves on a user's machine there is no port to curl and no
// access log to read. These helpers make the shell observable, and they are
// entirely opt-in — an ordinary session logs nothing and writes no files.
//
//	SIMARC_DESKTOP_LOG=1              one line per request the window makes
//	SIMARC_DESKTOP_DUMP=/path/file    save the first HTML response to that file
//
// The dump is what makes the shell verifiable from outside the GUI: it is the
// exact markup the webview renders, so checking it for the top navigation
// proves what the window shows without needing a screenshot.
var (
	dumpMu      sync.Mutex
	dumpWritten bool
)

// instrument wraps the engine with the enabled diagnostics.
func instrument(next http.Handler) http.Handler {
	logging := os.Getenv("SIMARC_DESKTOP_LOG") != ""
	dump := os.Getenv("SIMARC_DESKTOP_DUMP")
	if !logging && dump == "" {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		rec := &statusRecorder{
			ResponseWriter: w,
			status:         http.StatusOK,
			header:         http.Header{},
		}

		// Only the first HTML response is buffered; capturing every page would
		// hold whole documents in memory for the life of the process.
		var capture *bodyCapture
		if dump != "" && isDocumentRequest(req) {
			dumpMu.Lock()
			want := !dumpWritten
			dumpMu.Unlock()
			if want {
				capture = &bodyCapture{ResponseWriter: w}
				rec.ResponseWriter = capture
			}
		}

		next.ServeHTTP(rec, req)

		if logging {
			log.Printf("[desktop] %s %s -> %d %s",
				req.Method, req.URL.Path, rec.status,
				time.Since(start).Round(time.Millisecond))
		}

		if capture != nil {
			writeDump(dump, rec, capture.body)
		}
	})
}

// isDocumentRequest reports whether this request is for a page (not a
// stylesheet, font, image or script), i.e. the markup worth dumping.
func isDocumentRequest(req *http.Request) bool {
	if req.Method != http.MethodGet && req.Method != http.MethodPost {
		return false
	}
	ext := req.URL.Path
	for _, dot := range []string{".css", ".js", ".png", ".jpg", ".jpeg", ".gif",
		".svg", ".ico", ".woff", ".woff2", ".ttf", ".eot", ".map"} {
		if len(ext) >= len(dot) && ext[len(ext)-len(dot):] == dot {
			return false
		}
	}
	return true
}

func writeDump(path string, rec *statusRecorder, body []byte) {
	dumpMu.Lock()
	if dumpWritten {
		dumpMu.Unlock()
		return
	}
	dumpWritten = true
	dumpMu.Unlock()

	if ct := rec.header.Get("Content-Type"); ct != "" &&
		!strings.Contains(ct, "text/html") {
		return
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		log.Printf("[desktop] tidak bisa menulis dump HTML: %v", err)
		return
	}
	log.Printf("[desktop] HTML awal ditulis ke %s (%d byte)", path, len(body))
}

// statusRecorder remembers the status code and the headers so they can be
// reported after the handler chain has run. Wails' own recorder is not
// reachable from application code.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
	header  http.Header
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.written {
		s.status = code
		s.written = true
		s.snapshotHeader()
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.written {
		s.written = true
		s.snapshotHeader()
	}
	return s.ResponseWriter.Write(b)
}

// snapshotHeader copies the live header map before the handler may mutate it.
func (s *statusRecorder) snapshotHeader() {
	for k, v := range s.ResponseWriter.Header() {
		s.header[k] = v
	}
}

// bodyCapture tees a response body so it can be written to disk while still
// reaching the webview unchanged.
type bodyCapture struct {
	http.ResponseWriter
	body []byte
}

func (b *bodyCapture) Write(p []byte) (int, error) {
	b.body = append(b.body, p...)
	return b.ResponseWriter.Write(p)
}
