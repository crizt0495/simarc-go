package handlers

import (
	"crypto/subtle"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"arsippro/internal/config"

	"github.com/gin-gonic/gin"
)

// Archive-file sync endpoints served by the hub application.
//
// Nodes exchange files under storage/app/uploads through these endpoints using
// a shared secret (SYNC_HUB_TOKEN) sent in the X-Sync-Token header. They live
// outside the login-protected routes so a headless node can reach them, but
// they refuse to serve anything unless the hub has a token configured and the
// caller presents it.

const syncTokenHeader = "X-Sync-Token"

// SyncTokenRequired rejects requests whose X-Sync-Token header does not match
// the configured shared secret. When no token is configured the endpoints are
// disabled entirely (401), so files are never exposed unintentionally.
func SyncTokenRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		want := config.LoadSync().HubToken
		got := c.GetHeader(syncTokenHeader)
		if want == "" || got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token sinkronisasi tidak valid"})
			return
		}
		c.Next()
	}
}

// syncUploadEntry mirrors syncdb.FileEntry for the JSON list endpoint.
type syncUploadEntry struct {
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	Mtime int64  `json:"mtime"`
}

// syncUploadsOverride, when non-empty, overrides the uploads root (test only).
var syncUploadsOverride string

// syncUploadsRoot returns the absolute path of the uploads directory.
func syncUploadsRoot() (string, error) {
	if syncUploadsOverride != "" {
		return syncUploadsOverride, nil
	}
	return filepath.Abs(config.UploadDir())
}

// safeUploadPath validates a client-supplied relative path and joins it under
// root. It rejects absolute paths, "..", and anything that escapes root.
func safeUploadPath(root, rel string) (string, bool) {
	rel = strings.TrimSpace(strings.ReplaceAll(rel, "\\", "/"))
	if rel == "" || strings.HasPrefix(rel, "/") {
		return "", false
	}
	clean := path.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") || strings.HasSuffix(clean, "/..") {
		return "", false
	}
	full := filepath.Join(root, filepath.FromSlash(clean))
	if full != root && !strings.HasPrefix(full, root+string(os.PathSeparator)) {
		return "", false
	}
	return full, true
}

// SyncFilesList returns every syncable archive file as JSON.
func SyncFilesList(c *gin.Context) {
	root, err := syncUploadsRoot()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	cfg := config.LoadSync()
	maxBytes := cfg.FilesMaxBytes()

	entries := make([]syncUploadEntry, 0, 64)
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if cfg.ExcludesUpload(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil || !info.Mode().IsRegular() || info.Size() > maxBytes {
			return nil
		}
		entries = append(entries, syncUploadEntry{Path: rel, Size: info.Size(), Mtime: info.ModTime().UnixNano()})
		return nil
	})
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	c.JSON(http.StatusOK, entries)
}

// SyncFileGet streams one archive file to the caller.
func SyncFileGet(c *gin.Context) {
	root, err := syncUploadsRoot()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	full, ok := safeUploadPath(root, c.Query("path"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path tidak valid"})
		return
	}
	info, err := os.Stat(full)
	if err != nil || !info.Mode().IsRegular() {
		c.JSON(http.StatusNotFound, gin.H{"error": "berkas tidak ditemukan"})
		return
	}
	c.File(full)
}

// SyncFilePut stores one archive file sent by a node.
func SyncFilePut(c *gin.Context) {
	root, err := syncUploadsRoot()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	full, ok := safeUploadPath(root, c.Query("path"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path tidak valid"})
		return
	}
	maxBytes := config.LoadSync().FilesMaxBytes()
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)

	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(full), ".sync-*.tmp")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	tmpName := tmp.Name()
	written, err := io.Copy(tmp, c.Request.Body)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmpName)
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": err.Error()})
		return
	}
	if err := os.Rename(tmpName, full); err != nil {
		os.Remove(tmpName)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if mt, err := strconv.ParseInt(c.Query("mtime"), 10, 64); err == nil && mt > 0 {
		t := time.Unix(0, mt)
		_ = os.Chtimes(full, t, t)
	}
	c.JSON(http.StatusCreated, gin.H{"stored": written})
}
