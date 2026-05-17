//go:build integration

// Integration tests for the snapshot capturer. They require a running
// Postgres reachable via DBWATCH_TEST_DB_URL (default points at the dev
// container). Run with: go test -tags=integration ./internal/snapshot/...
package snapshot

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
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
	conn, err := pgx.Connect(context.Background(), testDBURL())
	require.NoError(t, err, "connect to test database")
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

func TestIntegration_CaptureAndCompare(t *testing.T) {
	ctx := context.Background()
	conn := connect(t)

	// Fresh table for a deterministic test.
	_, _ = conn.Exec(ctx, "DROP TABLE IF EXISTS snapshot_it")
	_, err := conn.Exec(ctx, `
		CREATE TABLE snapshot_it (
			id int PRIMARY KEY,
			phone text
		)`)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = conn.Exec(context.Background(), "DROP TABLE IF EXISTS snapshot_it") })

	_, err = conn.Exec(ctx, `INSERT INTO snapshot_it (id, phone) VALUES (1, NULL), (2, '123'), (3, NULL)`)
	require.NoError(t, err)

	capturer := NewCapturer(conn, Options{IncludeTables: []string{"snapshot_it"}})

	// First snapshot: phone is nullable, 2 NULL rows.
	before, err := capturer.Capture(ctx, "before")
	require.NoError(t, err)
	require.Contains(t, before.Schema.Tables, "public.snapshot_it")
	stat := before.Statistics["public.snapshot_it"]
	assert.Equal(t, int64(3), stat.RowCount)
	assert.Equal(t, int64(2), stat.Columns["phone"].NullCount)

	// Apply a migration: backfill NULLs and tighten the column.
	_, err = conn.Exec(ctx, `UPDATE snapshot_it SET phone = 'unknown' WHERE phone IS NULL`)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `ALTER TABLE snapshot_it ALTER COLUMN phone SET NOT NULL`)
	require.NoError(t, err)

	// Second snapshot.
	after, err := capturer.Capture(ctx, "after")
	require.NoError(t, err)

	report := Compare(before, after)

	// Schema: phone went NULL -> NOT NULL (breaking).
	require.GreaterOrEqual(t, len(report.SchemaChanges), 1)
	foundNotNull := false
	for _, c := range report.SchemaChanges {
		if c.Column == "phone" && c.Breaking {
			foundNotNull = true
		}
	}
	assert.True(t, foundNotNull, "expected a breaking NULL→NOT NULL change on phone")
	assert.Equal(t, 1, report.BreakingCount())

	// Data: the 2 NULLs disappeared — notable.
	require.Len(t, report.DataChanges, 1)
	dc := report.DataChanges[0]
	var phoneCol *ColumnDataChange
	for i := range dc.Columns {
		if dc.Columns[i].Column == "phone" {
			phoneCol = &dc.Columns[i]
		}
	}
	require.NotNil(t, phoneCol)
	assert.Equal(t, int64(2), phoneCol.NullFrom)
	assert.Equal(t, int64(0), phoneCol.NullTo)
	assert.True(t, phoneCol.Notable)
}

func TestIntegration_TableFilter(t *testing.T) {
	ctx := context.Background()
	conn := connect(t)

	_, _ = conn.Exec(ctx, "DROP TABLE IF EXISTS snapshot_it_a")
	_, _ = conn.Exec(ctx, "DROP TABLE IF EXISTS snapshot_it_b")
	_, err := conn.Exec(ctx, "CREATE TABLE snapshot_it_a (id int)")
	require.NoError(t, err)
	_, err = conn.Exec(ctx, "CREATE TABLE snapshot_it_b (id int)")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), "DROP TABLE IF EXISTS snapshot_it_a")
		_, _ = conn.Exec(context.Background(), "DROP TABLE IF EXISTS snapshot_it_b")
	})

	capturer := NewCapturer(conn, Options{IncludeTables: []string{"snapshot_it_a"}})
	snap, err := capturer.Capture(ctx, "filtered")
	require.NoError(t, err)

	assert.Contains(t, snap.Schema.Tables, "public.snapshot_it_a")
	assert.NotContains(t, snap.Schema.Tables, "public.snapshot_it_b")
}
