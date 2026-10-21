//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// createNoWindow mencegah Windows membuat jendela konsol untuk proses anak.
// Digabung dengan HideWindow, installer tidak pernah memunculkan CMD.
const createNoWindow = 0x08000000

func setHiddenWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
