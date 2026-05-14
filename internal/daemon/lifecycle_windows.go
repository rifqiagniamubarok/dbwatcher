//go:build windows

package daemon

import (
	"errors"
	"time"
)

// ErrUnsupportedPlatform is returned by Windows builds when something tries
// to inspect or stop a daemon process. Windows daemon mode is not yet
// implemented — see README.md and CHANGELOG.md "Known limitations".
var ErrUnsupportedPlatform = errors.New("daemon process management is not supported on Windows; use 'dbwatch tail' instead")

func IsProcessRunning(pid int) bool {
	// Without POSIX signals we can't cheaply probe a foreign PID. Returning
	// false means daemon-status / daemon-stop will treat any PID file on
	// Windows as stale and refuse to operate, which is correct behavior
	// until proper Windows service support lands.
	return false
}

func StopProcess(pid int, timeout time.Duration) error {
	return ErrUnsupportedPlatform
}
