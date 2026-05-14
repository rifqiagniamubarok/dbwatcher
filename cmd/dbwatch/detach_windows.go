//go:build windows

package main

import (
	"os/exec"

	"github.com/rifqiagniamubarok/dbwatcher/internal/daemon"
)

// applyDetachAttrs is a no-op stub on Windows. The current daemon design
// relies on POSIX session detach (Setsid), which Windows does not offer in
// the same form. Returning ErrUnsupportedPlatform here makes
// `dbwatch daemon start --detach` fail loudly instead of starting a child
// that we cannot manage.
func applyDetachAttrs(cmd *exec.Cmd) error {
	return daemon.ErrUnsupportedPlatform
}
