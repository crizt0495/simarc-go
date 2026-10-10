//go:build !windows

package main

import "syscall"

// browserProcAttr returns the process attributes used when launching an
// external browser. On Unix we start the browser in its own process group so
// that signals sent to the server (e.g. Ctrl+C) do not also kill the browser.
func browserProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
