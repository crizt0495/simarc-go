//go:build !windows

package main

// createShortcuts tidak berlaku di luar Windows.
func createShortcuts(installDir, runBat, iconPath string, noDesktop bool) error {
	return nil
}
