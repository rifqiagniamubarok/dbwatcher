package ipc

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	defaultDaemonName = "default"
	runtimeDirMode    = 0o700
)

func normalizeDaemonName(name string) string {
	if name == "" {
		return defaultDaemonName
	}
	return name
}

// ResolveRuntimeDir returns the base directory used for daemon artifacts.
func ResolveRuntimeDir() (string, error) {
	if custom := os.Getenv("DBWATCH_SOCKET_DIR"); custom != "" {
		if err := os.MkdirAll(custom, runtimeDirMode); err != nil {
			return "", fmt.Errorf("create socket dir: %w", err)
		}
		return custom, nil
	}

	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		dir := filepath.Join(xdg, "dbwatch")
		if err := os.MkdirAll(dir, runtimeDirMode); err != nil {
			return "", fmt.Errorf("create runtime dir: %w", err)
		}
		return dir, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	dir := filepath.Join(home, ".dbwatch")
	if err := os.MkdirAll(dir, runtimeDirMode); err != nil {
		return "", fmt.Errorf("create fallback runtime dir: %w", err)
	}
	return dir, nil
}

// ResolveSocketPath resolves the daemon Unix socket path for a daemon name.
func ResolveSocketPath(name string) (string, error) {
	dir, err := ResolveRuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, normalizeDaemonName(name)+".sock"), nil
}

// ResolvePIDPath resolves the daemon PID file path for a daemon name.
func ResolvePIDPath(name string) (string, error) {
	dir, err := ResolveRuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, normalizeDaemonName(name)+".pid"), nil
}

// ResolveLogPath resolves the daemon log file path for a daemon name.
func ResolveLogPath(name string) (string, error) {
	dir, err := ResolveRuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, normalizeDaemonName(name)+".log"), nil
}
