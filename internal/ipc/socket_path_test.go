package ipc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveSocketPath_UsesCustomDir(t *testing.T) {
	t.Setenv("DBWATCH_SOCKET_DIR", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", "")

	p, err := ResolveSocketPath("staging")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(os.Getenv("DBWATCH_SOCKET_DIR"), "staging.sock"), p)
}

func TestResolveSocketPath_DefaultName(t *testing.T) {
	t.Setenv("DBWATCH_SOCKET_DIR", t.TempDir())
	p, err := ResolveSocketPath("")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(os.Getenv("DBWATCH_SOCKET_DIR"), "default.sock"), p)
}
