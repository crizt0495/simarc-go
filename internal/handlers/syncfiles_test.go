package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestSyncFileEndpoints exercises the real hub-side handlers: token auth, list,
// download, upload, and path-traversal rejection.
func TestSyncFileEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := t.TempDir()
	syncUploadsOverride = root
	defer func() { syncUploadsOverride = "" }()

	t.Setenv("SYNC_HUB_TOKEN", "secret")
	t.Setenv("SYNC_FILES_MAX_MB", "8")

	if err := os.MkdirAll(filepath.Join(root, "arsip"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "arsip", "x.pdf"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	g := r.Group("/api/sync/files")
	g.Use(SyncTokenRequired())
	g.GET("/list", SyncFilesList)
	g.GET("/get", SyncFileGet)
	g.POST("/put", SyncFilePut)
	srv := httptest.NewServer(r)
	defer srv.Close()

	do := func(method, path, token, body string) *http.Response {
		req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set(syncTokenHeader, token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	// No token -> 401.
	resp := do(http.MethodGet, "/api/sync/files/list", "", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("list without token = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()

	// List with token.
	resp = do(http.MethodGet, "/api/sync/files/list", "secret", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list = %d, want 200", resp.StatusCode)
	}
	var list []syncUploadEntry
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(list) != 1 || list[0].Path != "arsip/x.pdf" {
		t.Errorf("list = %+v, want one entry arsip/x.pdf", list)
	}

	// Get with token.
	resp = do(http.MethodGet, "/api/sync/files/get?path=arsip/x.pdf", "secret", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "hello" {
		t.Errorf("get body = %q, want hello", body)
	}

	// Traversal must be rejected.
	resp = do(http.MethodGet, "/api/sync/files/get?path=../secret", "secret", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("traversal = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// Put with token.
	resp = do(http.MethodPost, "/api/sync/files/put?path=arsip/y.pdf", "secret", "world")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("put = %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()
	if b, err := os.ReadFile(filepath.Join(root, "arsip", "y.pdf")); err != nil || string(b) != "world" {
		t.Errorf("stored file = %q, err=%v; want world", b, err)
	}

	// Put without token.
	resp = do(http.MethodPost, "/api/sync/files/put?path=arsip/z.pdf", "", "x")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("put without token = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()
}
