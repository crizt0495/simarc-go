package syncdb

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"arsippro/internal/config"
)

// newRefHub is a minimal implementation of the hub file endpoints used to
// exercise the client-side sync logic (the real handlers are tested separately
// in the handlers package).
func newRefHub(t *testing.T, root, token string) *httptest.Server {
	t.Helper()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if token != "" && r.Header.Get(syncTokenHeader) != token {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			h(w, r)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/files/list", auth(func(w http.ResponseWriter, r *http.Request) {
		type entry struct {
			Path  string `json:"path"`
			Size  int64  `json:"size"`
			Mtime int64  `json:"mtime"`
		}
		out := []entry{}
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return nil
			}
			info, ierr := d.Info()
			if ierr != nil {
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			out = append(out, entry{filepath.ToSlash(rel), info.Size(), info.ModTime().UnixNano()})
			return nil
		})
		_ = json.NewEncoder(w).Encode(out)
	}))
	mux.HandleFunc("/api/sync/files/get", auth(func(w http.ResponseWriter, r *http.Request) {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(r.URL.Query().Get("path"))))
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(b)
	}))
	mux.HandleFunc("/api/sync/files/put", auth(func(w http.ResponseWriter, r *http.Request) {
		full := filepath.Join(root, filepath.FromSlash(r.URL.Query().Get("path")))
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		b, _ := io.ReadAll(r.Body)
		if err := os.WriteFile(full, b, 0o644); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if mt, _ := strconv.ParseInt(r.URL.Query().Get("mtime"), 10, 64); mt > 0 {
			t := time.Unix(0, mt)
			_ = os.Chtimes(full, t, t)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	return httptest.NewServer(mux)
}

func writeFileAt(t *testing.T, path, content string, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if !mtime.IsZero() {
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
}

func readFileAt(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSyncFilesConverges(t *testing.T) {
	node := t.TempDir()
	hubDir := t.TempDir()
	srv := newRefHub(t, hubDir, "secret")
	defer srv.Close()

	cfg := config.SyncConfig{
		HubURL: srv.URL, HubToken: "secret", FilesEnabled: true,
		FilesMaxMB: 8, FilesExclude: []string{"tmp"},
	}
	ctx := context.Background()
	base := time.Now().Truncate(time.Second)

	// Node has an archive file and an excluded scratch file; hub has its own.
	writeFileAt(t, filepath.Join(node, "arsip", "a.pdf"), "AAA", base)
	writeFileAt(t, filepath.Join(node, "tmp", "scratch.bin"), "XXX", base)
	writeFileAt(t, filepath.Join(hubDir, "arsip", "b.pdf"), "BBB", base.Add(time.Second))

	res, err := syncFilesAt(ctx, cfg, node, nil)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Pushed != 1 {
		t.Errorf("pushed = %d, want 1", res.Pushed)
	}
	if res.Pulled != 1 {
		t.Errorf("pulled = %d, want 1", res.Pulled)
	}
	if got := readFileAt(t, filepath.Join(node, "arsip", "b.pdf")); got != "BBB" {
		t.Errorf("node b.pdf = %q, want BBB", got)
	}
	if got := readFileAt(t, filepath.Join(hubDir, "arsip", "a.pdf")); got != "AAA" {
		t.Errorf("hub a.pdf = %q, want AAA", got)
	}
	if _, err := os.Stat(filepath.Join(hubDir, "tmp", "scratch.bin")); !os.IsNotExist(err) {
		t.Errorf("excluded tmp/scratch.bin must not be pushed to hub")
	}

	// A newer edit on the node must overwrite the hub copy.
	writeFileAt(t, filepath.Join(node, "arsip", "a.pdf"), "AAAA", base.Add(5*time.Second))
	res, err = syncFilesAt(ctx, cfg, node, nil)
	if err != nil {
		t.Fatalf("sync2: %v", err)
	}
	if res.Pushed != 1 {
		t.Errorf("pushed = %d, want 1 (newer node file)", res.Pushed)
	}
	if got := readFileAt(t, filepath.Join(hubDir, "arsip", "a.pdf")); got != "AAAA" {
		t.Errorf("hub a.pdf = %q, want AAAA (newer wins)", got)
	}

	// A newer edit on the hub must overwrite the node copy.
	writeFileAt(t, filepath.Join(hubDir, "arsip", "b.pdf"), "BBBB", base.Add(10*time.Second))
	res, err = syncFilesAt(ctx, cfg, node, nil)
	if err != nil {
		t.Fatalf("sync3: %v", err)
	}
	if res.Pulled != 1 {
		t.Errorf("pulled = %d, want 1 (newer hub file)", res.Pulled)
	}
	if got := readFileAt(t, filepath.Join(node, "arsip", "b.pdf")); got != "BBBB" {
		t.Errorf("node b.pdf = %q, want BBBB (newer hub wins)", got)
	}

	// A steady state should be a no-op.
	res, err = syncFilesAt(ctx, cfg, node, nil)
	if err != nil {
		t.Fatalf("sync4: %v", err)
	}
	if res.Pushed != 0 || res.Pulled != 0 {
		t.Errorf("steady state pushed=%d pulled=%d, want 0/0", res.Pushed, res.Pulled)
	}
}
