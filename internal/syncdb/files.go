package syncdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"arsippro/internal/config"
)

// Archive-file synchronisation.
//
// The database sync (engine.go) only carries rows; archive files under
// storage/app/uploads live on each computer's disk. They are exchanged
// through the hub application over HTTP, using token-protected endpoints
// (handlers/syncfiles.go):
//
//   GET  /api/sync/files/list          -> []FileEntry (path, size, mtime)
//   GET  /api/sync/files/get?path=...  -> file bytes
//   POST /api/sync/files/put?path=...&mtime=... -> store file bytes
//
// Freshness uses (size, mtime) as in rsync's default quick check: a file is
// copied when it is missing on one side or clearly newer on the other. Changes
// within a two-second window are treated as equal so small clock differences
// between computers cannot cause endless copies.

const syncTokenHeader = "X-Sync-Token"

// mtimeTolerance absorbs clock skew when comparing file timestamps.
const mtimeTolerance = int64(2 * time.Second)

// FileEntry describes one archive file, identified by its path relative to the
// uploads directory (always slash separated).
type FileEntry struct {
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	Mtime int64  `json:"mtime"` // Unix nanoseconds
}

// FileSyncResult summarises one archive-file sync pass.
type FileSyncResult struct {
	Pushed   int   `json:"pushed"`
	Pulled   int   `json:"pulled"`
	BytesOut int64 `json:"bytes_out"`
	BytesIn  int64 `json:"bytes_in"`
}

// SyncFiles copies archive files between this computer and the hub: files that
// are new or newer locally are uploaded, files new or newer on the hub are
// downloaded. It is safe to run when the hub is unreachable (returns an error
// and changes nothing).
func SyncFiles(ctx context.Context, cfg config.SyncConfig, logf func(string, ...any)) (FileSyncResult, error) {
	root, err := filepath.Abs(config.UploadDir())
	if err != nil {
		return FileSyncResult{}, err
	}
	return syncFilesAt(ctx, cfg, root, logf)
}

// syncFilesAt is SyncFiles against an explicit uploads root (used by tests).
func syncFilesAt(ctx context.Context, cfg config.SyncConfig, root string, logf func(string, ...any)) (FileSyncResult, error) {
	var res FileSyncResult
	if logf == nil {
		logf = func(string, ...any) {}
	}
	if !cfg.FilesReady() {
		return res, fmt.Errorf("sinkronisasi berkas belum dikonfigurasi (SYNC_HUB_URL / SYNC_HUB_TOKEN)")
	}

	maxBytes := cfg.FilesMaxBytes()

	local, err := localFiles(root, cfg, maxBytes)
	if err != nil {
		return res, err
	}
	hubList, err := listHubFiles(ctx, cfg)
	if err != nil {
		return res, err
	}
	hub := make(map[string]FileEntry, len(hubList))
	for _, e := range hubList {
		if !validRel(e.Path) || cfg.ExcludesUpload(e.Path) || e.Size > maxBytes {
			continue
		}
		hub[e.Path] = e
	}

	client := &http.Client{Timeout: 30 * time.Minute}

	// Pull: anything newer/only on the hub.
	for rel, he := range hub {
		le, ok := local[rel]
		if ok && sameFile(le, he) {
			continue
		}
		if ok && le.Mtime > he.Mtime {
			continue // local copy is newer; it will be pushed below
		}
		if err := downloadFile(ctx, client, cfg, root, rel, he); err != nil {
			logf("[sync] berkas tarik %s: %v", rel, err)
			continue
		}
		res.Pulled++
		res.BytesIn += he.Size
		local[rel] = he
	}

	// Push: anything newer/only on this computer.
	for rel, le := range local {
		if cfg.ExcludesUpload(rel) {
			continue
		}
		he, ok := hub[rel]
		if ok && sameFile(le, he) {
			continue
		}
		if ok && he.Mtime > le.Mtime {
			continue // hub copy is newer; already pulled above
		}
		if err := uploadFile(ctx, client, cfg, root, rel, le); err != nil {
			logf("[sync] berkas dorong %s: %v", rel, err)
			continue
		}
		res.Pushed++
		res.BytesOut += le.Size
	}
	return res, nil
}

// localFiles walks the uploads directory and returns the syncable files.
func localFiles(root string, cfg config.SyncConfig, maxBytes int64) (map[string]FileEntry, error) {
	out := make(map[string]FileEntry)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable entries rather than aborting
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
				return fs.SkipDir
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
		out[rel] = FileEntry{Path: rel, Size: info.Size(), Mtime: info.ModTime().UnixNano()}
		return nil
	})
	if os.IsNotExist(err) {
		return out, nil
	}
	return out, err
}

// sameFile reports whether two entries describe the same content for sync
// purposes (same size, timestamps within tolerance).
func sameFile(a, b FileEntry) bool {
	if a.Size != b.Size {
		return false
	}
	d := a.Mtime - b.Mtime
	if d < 0 {
		d = -d
	}
	return d <= mtimeTolerance
}

// validRel rejects absolute paths and traversal outside the uploads root.
func validRel(rel string) bool {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, "\\") {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") || strings.HasSuffix(rel, "/..") {
		return false
	}
	return true
}

func listHubFiles(ctx context.Context, cfg config.SyncConfig) ([]FileEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.HubURL+"/api/sync/files/list", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(syncTokenHeader, cfg.HubToken)

	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("hub menolak daftar berkas (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var list []FileEntry
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}
	return list, nil
}

func downloadFile(ctx context.Context, client *http.Client, cfg config.SyncConfig, root, rel string, entry FileEntry) error {
	u := cfg.HubURL + "/api/sync/files/get?path=" + url.QueryEscape(rel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set(syncTokenHeader, cfg.HubToken)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(full), ".sync-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, full); err != nil {
		os.Remove(tmpName)
		return err
	}
	// Preserve the source timestamp so both sides agree on freshness.
	if entry.Mtime > 0 {
		mt := time.Unix(0, entry.Mtime)
		_ = os.Chtimes(full, mt, mt)
	}
	return nil
}

func uploadFile(ctx context.Context, client *http.Client, cfg config.SyncConfig, root, rel string, entry FileEntry) error {
	full := filepath.Join(root, filepath.FromSlash(rel))
	f, err := os.Open(full)
	if err != nil {
		return err
	}
	defer f.Close()

	u := fmt.Sprintf("%s/api/sync/files/put?path=%s&mtime=%d",
		cfg.HubURL, url.QueryEscape(rel), entry.Mtime)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, f)
	if err != nil {
		return err
	}
	req.Header.Set(syncTokenHeader, cfg.HubToken)
	req.Header.Set("Content-Type", "application/octet-stream")
	if info, err := f.Stat(); err == nil {
		req.ContentLength = info.Size()
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}
