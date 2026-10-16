//go:build windows

package main

import (
	"os"
	"path/filepath"
	"runtime"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows/registry"
)

// createShortcuts membuat shortcut Start Menu (selalu) dan Desktop (opsional)
// memakai COM WScript.Shell, setara dengan jalur PowerShell. Ini memastikan
// aplikasi selalu bisa dibuka walau PowerShell diblokir.
func createShortcuts(installDir, runBat, iconPath string, noDesktop bool) error {
	// COM bersifat per-thread (STA): kunci goroutine ini ke OS thread agar
	// CoInitializeEx dan CreateObject berjalan pada thread yang sama.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		// Terima RPC_E_CHANGED_MODE (COM sudah terinisialisasi dengan mode lain).
		if oleErr, ok := err.(*ole.OleError); !ok || oleErr.Code() != 0x80010106 {
			return err
		}
	}
	defer ole.CoUninitialize()

	shellObj, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return err
	}
	defer shellObj.Release()

	wshell, err := shellObj.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return err
	}
	defer wshell.Release()

	saveLnk := func(lnkPath string) error {
		if err := os.MkdirAll(filepath.Dir(lnkPath), 0o755); err != nil {
			return err
		}
		cs, err := oleutil.CallMethod(wshell, "CreateShortcut", lnkPath)
		if err != nil {
			return err
		}
		sc := cs.ToIDispatch()
		defer sc.Release()
		_, _ = oleutil.PutProperty(sc, "TargetPath", runBat)
		_, _ = oleutil.PutProperty(sc, "WorkingDirectory", installDir)
		_, _ = oleutil.PutProperty(sc, "Description", "SIMARC - Arsip Record Center")
		if iconPath != "" {
			if _, err := os.Stat(iconPath); err == nil {
				_, _ = oleutil.PutProperty(sc, "IconLocation", iconPath)
			}
		}
		_, err = oleutil.CallMethod(sc, "Save")
		return err
	}

	// ── Start Menu ────────────────────────────────────────────────────────────
	var firstErr error
	startMenu := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "SIMARC")
	if err := saveLnk(filepath.Join(startMenu, "SIMARC.lnk")); err != nil {
		firstErr = err
	}

	// ── Desktop (kecuali diminta tidak) ──────────────────────────────────────
	if !noDesktop {
		if desk := desktopDir(); desk != "" {
			if err := saveLnk(filepath.Join(desk, "SIMARC.lnk")); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// desktopDir mengembalikan folder Desktop pengguna. Membaca dari registry
// (mendukung Desktop yang dialihkan ke OneDrive), lalu jatuh ke %USERPROFILE%\Desktop.
func desktopDir() string {
	const key = `Software\Microsoft\Windows\CurrentVersion\Explorer\Shell Folders`
	if k, err := registry.OpenKey(registry.CURRENT_USER, key, registry.QUERY_VALUE); err == nil {
		defer k.Close()
		if v, _, err := k.GetStringValue("Desktop"); err == nil && v != "" {
			return v
		}
	}
	if up := os.Getenv("USERPROFILE"); up != "" {
		return filepath.Join(up, "Desktop")
	}
	return ""
}
