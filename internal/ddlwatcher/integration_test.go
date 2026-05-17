//go:build integration

// Integration tests for the DDL watcher. They require a running Postgres
// reachable via DBWATCH_TEST_DB_URL (default points at the dev container).
// Run with: go test -tags=integration ./internal/ddlwatcher/...
package ddlwatcher

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testDBURL() string {
	if v := os.Getenv("DBWATCH_TEST_DB_URL"); v != "" {
		return v
	}
	return "postgres://test:test@localhost:5433/test?sslmode=disable"
}

func connect(t *testing.T) *pgx.Conn {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, stripReplicationParam(testDBURL()))
	require.NoError(t, err, "connect to test database")
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

func TestIntegration_InstallIsIdempotent(t *testing.T) {
	ctx := context.Background()
	conn := connect(t)

	// Clean slate.
	require.NoError(t, Uninstall(ctx, conn))

	installed, err := IsInstalled(ctx, conn)
	require.NoError(t, err)
	assert.False(t, installed)

	// First install.
	require.NoError(t, Install(ctx, conn))
	installed, err = IsInstalled(ctx, conn)
	require.NoError(t, err)
	assert.True(t, installed)

	// Second install must not error (idempotent).
	require.NoError(t, Install(ctx, conn))
	installed, err = IsInstalled(ctx, conn)
	require.NoError(t, err)
	assert.True(t, installed)

	// Uninstall, twice — also idempotent.
	require.NoError(t, Uninstall(ctx, conn))
	require.NoError(t, Uninstall(ctx, conn))
	installed, err = IsInstalled(ctx, conn)
	require.NoError(t, err)
	assert.False(t, installed)
}

func TestIntegration_DDLEventsCaptured(t *testing.T) {
	ctx := context.Background()
	setup := connect(t)

	require.NoError(t, Install(ctx, setup))
	t.Cleanup(func() { _ = Uninstall(context.Background(), setup) })

	events := make(chan store.Event, 16)
	listenerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	dl := New(testDBURL())
	listenerDone := make(chan error, 1)
	go func() {
		listenerDone <- dl.Start(listenerCtx, func(e store.Event) {
			select {
			case events <- e:
			default:
			}
		})
	}()

	// Give the listener a moment to LISTEN before issuing DDL.
	time.Sleep(300 * time.Millisecond)

	_, err := setup.Exec(ctx, "CREATE TABLE ddlwatcher_it (id int primary key)")
	require.NoError(t, err)
	_, err = setup.Exec(ctx, "ALTER TABLE ddlwatcher_it ADD COLUMN note text")
	require.NoError(t, err)
	_, err = setup.Exec(ctx, "DROP TABLE ddlwatcher_it")
	require.NoError(t, err)

	want := map[string]bool{"CREATE TABLE": false, "ALTER TABLE": false, "DROP TABLE": false}
	deadline := time.After(5 * time.Second)
	for remaining := len(want); remaining > 0; {
		select {
		case e := <-events:
			assert.Equal(t, store.KindDDL, e.Kind)
			if seen, tracked := want[e.CommandTag]; tracked && !seen {
				want[e.CommandTag] = true
				remaining--
			}
		case <-deadline:
			t.Fatalf("timed out waiting for DDL events; got %v", want)
		}
	}

	cancel()
	select {
	case <-listenerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("ddl listener did not stop")
	}
}
