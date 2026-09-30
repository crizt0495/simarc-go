package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// DataDir is the per-user directory the desktop app keeps its configuration
// and runtime files in:
//
//	Linux    $XDG_DATA_HOME/simarc  (default ~/.local/share/simarc)
//	macOS    ~/Library/Application Support/SIMARC
//	Windows  %APPDATA%\SIMARC
//
// The web build never calls it: the server keeps using its working directory,
// which is what development and the Vercel deployment already expect.
func DataDir() string {
	if dir := os.Getenv("SIMARC_DATA_DIR"); dir != "" {
		return dir
	}

	switch runtime.GOOS {
	case "windows":
		if base := os.Getenv("APPDATA"); base != "" {
			return filepath.Join(base, "SIMARC")
		}
	case "darwin":
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Library", "Application Support", "SIMARC")
		}
	default:
		if base := os.Getenv("XDG_DATA_HOME"); base != "" {
			return filepath.Join(base, "simarc")
		}
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".local", "share", "simarc")
		}
	}

	// Last resort: next to the binary, so the app still has a writable home.
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	return "."
}

// IsSourceCheckout reports whether dir looks like a development checkout — a
// directory holding the web/ tree. The desktop shell uses this to decide
// between reading templates from disk (so an edit shows up on restart) and
// falling back to the templates embedded in the binary (a real install).
func IsSourceCheckout(dir string) bool {
	if dir == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, "web", "templates"))
	return err == nil && info.IsDir()
}

// SourceRoot walks up from start looking for a development checkout, returning
// an empty string when there is none.
func SourceRoot(start string) string {
	dir := start
	for i := 0; i < 10; i++ {
		if IsSourceCheckout(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
