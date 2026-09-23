//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// detach puts the viewer in its own process group so signals aimed at Chrome's
// native-host process group do not reach it.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
