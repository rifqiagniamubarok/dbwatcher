package daemon

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// WritePIDFile, ReadPIDFile, and TruncateIfLarge are portable. Process
// inspection and stopping (IsProcessRunning, StopProcess) live in
// lifecycle_unix.go and lifecycle_windows.go because they need POSIX signals.

func WritePIDFile(path string, pid int) error {
	return os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o600)
}

func ReadPIDFile(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0, fmt.Errorf("invalid pid file %q: %w", path, err)
	}
	return pid, nil
}

func TruncateIfLarge(path string, maxBytes int64) error {
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if fi.Size() <= maxBytes {
		return nil
	}
	return os.Truncate(path, 0)
}
