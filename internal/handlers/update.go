// Package handlers — auto-update helpers.
//
// The hub (this server) can publish its own binary and a version string so
// that other computers on the LAN pick up code updates automatically, without
// anyone having to run the installer by hand. Clients check the hub on startup
// and periodically; when a newer version is advertised they download the
// binary into tmp/simarc-server.exe.new and apply it on the next restart
// (see run.bat).
package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gin-gonic/gin"
)

// UpdateVersion returns the running application's version. It is intentionally
// unauthenticated so a freshly installed (not yet logged-in) client can check
// for updates; it leaks only the version string.
func UpdateVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version": AppVersion(),
		"os":      runtime.GOOS,
		"arch":    runtime.GOARCH,
	})
}

// UpdateBinary serves the application binary that clients should download.
// On the hub this is its own executable; the path can be overridden with the
// SIMARC_UPDATE_BINARY environment variable (useful when the hub stores a
// Windows build for Windows clients).
func UpdateBinary(c *gin.Context) {
	path := os.Getenv("SIMARC_UPDATE_BINARY")
	if path == "" {
		// Default: the running executable itself.
		if exe, err := os.Executable(); err == nil {
			path = exe
		}
	}
	if path == "" {
		c.String(http.StatusNotFound, "binary not available")
		return
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		c.String(http.StatusNotFound, "binary not available")
		return
	}
	c.Header("Content-Disposition", "attachment; filename="+filepath.Base(path))
	c.File(path)
}

// AppVersion returns the running application's version. It prefers a VERSION
// file next to the executable (written by the installer), then the build-time
// value injected via -ldflags, then "dev".
func AppVersion() string {
	if v := strings.TrimSpace(os.Getenv("SIMARC_APP_VERSION")); v != "" {
		return v
	}
	// VERSION file beside the executable (installer writes this).
	if exe, err := os.Executable(); err == nil {
		if data, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "VERSION")); err == nil {
			if v := strings.TrimSpace(string(data)); v != "" {
				return v
			}
		}
	}
	// Working directory fallback (dev runs).
	if data, err := os.ReadFile("VERSION"); err == nil {
		if v := strings.TrimSpace(string(data)); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(buildVersion); v != "" && v != "dev" {
		return v
	}
	return "dev"
}

// buildVersion is injected at build time via -ldflags "-X .../handlers.buildVersion=...".
// Defaults to "dev" for untagged builds.
var buildVersion = "dev"

// Ensure fmt is used (kept for future expansion).
var _ = fmt.Sprintf
