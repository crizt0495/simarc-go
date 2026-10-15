//go:build !windows

package main

import "os/exec"

func setHiddenWindow(cmd *exec.Cmd) {}
