//go:build windows

package main

import "syscall"

// browserProcAttr returns the process attributes used when launching an
// external browser. On Windows we detach the browser into its own process
// group (CREATE_NEW_PROCESS_GROUP) so it is not tied to the server console.
// The Unix-only Setpgid field is not available here, which is why this lives
// in a platform-specific file.
func browserProcAttr() *syscall.SysProcAttr {
	const createNewProcessGroup = 0x00000200
	return &syscall.SysProcAttr{CreationFlags: createNewProcessGroup}
}
