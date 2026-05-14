//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// applyDetachAttrs configures the child process so it survives the parent's
// exit. On POSIX systems that means starting a new session (Setsid) so the
// child detaches from the controlling terminal and the parent's process group.
func applyDetachAttrs(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return nil
}
