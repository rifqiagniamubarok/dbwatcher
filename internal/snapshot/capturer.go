package snapshot

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Default safeguard values (task 8.8). All overridable via Options.
const (
	defaultPerTableTimeout = 30 * time.Second
	// largeTableThreshold: above this row count, per-column statistics are
	// skipped (a full table scan for COUNT(DISTINCT) would be too costly).
	defaultLargeTableThreshold = 10_000_000
)

// Options tunes a Capture run.
type Options struct {
	// PerTableTimeout caps how long statistics for a single table may take.
	// Zero means the default.
	PerTableTimeout time.Duration
	// LargeTableThreshold is the row count above which per-column statistics
	// are skipped. Zero means the default.
	LargeTableThreshold int64
	// IncludeTables, when non-empty, restricts the capture to these tables
	// (bare names, e.g. "users"). Mutually exclusive with ExcludeTables.
	IncludeTables []string
	// ExcludeTables omits these tables from the capture.
	ExcludeTables []string
}

func (o Options) perTableTimeout() time.Duration {
	if o.PerTableTimeout > 0 {
		return o.PerTableTimeout
	}
	return defaultPerTableTimeout
}

func (o Options) largeTableThreshold() int64 {
	if o.LargeTableThreshold > 0 {
		return o.LargeTableThreshold
	}
	return defaultLargeTableThreshold
}

// Capturer captures snapshots from a Postgres connection.
type Capturer struct {
	conn *pgx.Conn
	opts Options
}

// NewCapturer wraps a connection. The connection is owned by the caller.
func NewCapturer(conn *pgx.Conn, opts Options) *Capturer {
	return &Capturer{conn: conn, opts: opts}
}

// Capture builds a Snapshot: schema metadata for every table in the public
// schema, plus aggregate statistics. Statistics for very large tables are
// reduced to a row count (see LargeTableThreshold).
func (c *Capturer) Capture(ctx context.Context, label string) (*Snapshot, error) {
	if label == "" {
		label = "snapshot-" + time.Now().Format("20060102-150405")
	}

	snap := &Snapshot{
		ID:         "snap_" + uuid.NewString()[:8],
		Label:      label,
		CapturedAt: time.Now(),
		Schema:     SchemaState{Tables: map[string]TableSchema{}},
		Statistics: map[string]TableStatistics{},
	}

	tables, err := c.listTables(ctx)
	if err != nil {
		return nil, err
	}

	for _, tbl := range tables {
		if !c.included(tbl.Name) {
			continue
		}
		key := qualifiedName(tbl.Schema, tbl.Name)

		schema, err := c.captureSchema(ctx, tbl.Schema, tbl.Name)
		if err != nil {
			return nil, fmt.Errorf("capture schema for %s: %w", key, err)
		}
		snap.Schema.Tables[key] = schema

		stats := c.captureStatistics(ctx, tbl.Schema, tbl.Name, schema)
		snap.Statistics[key] = stats
	}

	return snap, nil
}

type tableRef struct {
	Schema string
	Name   string
}

func (c *Capturer) listTables(ctx context.Context) ([]tableRef, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT table_schema, table_name
		FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
		ORDER BY table_name`)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	defer rows.Close()

	var out []tableRef
	for rows.Next() {
		var t tableRef
		if err := rows.Scan(&t.Schema, &t.Name); err != nil {
			return nil, fmt.Errorf("scan table row: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (c *Capturer) captureSchema(ctx context.Context, schema, table string) (TableSchema, error) {
	ts := TableSchema{Schema: schema, Name: table}

	rows, err := c.conn.Query(ctx, `
		SELECT column_name, data_type, is_nullable, COALESCE(column_default, ''), ordinal_position
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position`, schema, table)
	if err != nil {
		return ts, fmt.Errorf("query columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			col      ColumnSchema
			nullable string
		)
		if err := rows.Scan(&col.Name, &col.DataType, &nullable, &col.Default, &col.Position); err != nil {
			return ts, fmt.Errorf("scan column: %w", err)
		}
		col.Nullable = nullable == "YES"
		ts.Columns = append(ts.Columns, col)
	}
	if err := rows.Err(); err != nil {
		return ts, err
	}

	idxRows, err := c.conn.Query(ctx, `
		SELECT indexname FROM pg_indexes
		WHERE schemaname = $1 AND tablename = $2
		ORDER BY indexname`, schema, table)
	if err != nil {
		return ts, fmt.Errorf("query indexes: %w", err)
	}
	defer idxRows.Close()
	for idxRows.Next() {
		var name string
		if err := idxRows.Scan(&name); err != nil {
			return ts, fmt.Errorf("scan index: %w", err)
		}
		ts.Indexes = append(ts.Indexes, name)
	}
	return ts, idxRows.Err()
}

// captureStatistics collects aggregate stats for one table. It never returns
// an error — a failure (timeout, permission) downgrades to a Skipped result
// so one bad table doesn't abort the whole snapshot.
func (c *Capturer) captureStatistics(ctx context.Context, schema, table string, ts TableSchema) TableStatistics {
	key := qualifiedName(schema, table)
	stats := TableStatistics{Table: key, Columns: map[string]ColumnStats{}}

	tctx, cancel := context.WithTimeout(ctx, c.opts.perTableTimeout())
	defer cancel()

	ident := pgx.Identifier{schema, table}.Sanitize()

	rowCount, err := c.queryRowCount(tctx, ident)
	if err != nil {
		stats.Skipped = true
		stats.SkippedReason = "row count failed: " + err.Error()
		slog.Warn("snapshot: row count failed", "table", key, "err", err)
		return stats
	}
	stats.RowCount = rowCount

	if rowCount > c.opts.largeTableThreshold() {
		stats.Skipped = true
		stats.SkippedReason = fmt.Sprintf("large table (%d rows) — per-column stats skipped", rowCount)
		return stats
	}

	for _, col := range ts.Columns {
		cs, err := c.queryColumnStats(tctx, ident, col.Name)
		if err != nil {
			// Partial failure: keep what we have, mark the table skipped.
			stats.Skipped = true
			stats.SkippedReason = "column stats failed for " + col.Name + ": " + err.Error()
			slog.Warn("snapshot: column stats failed", "table", key, "column", col.Name, "err", err)
			return stats
		}
		cs.NullCount = rowCount - cs.NonNullCount
		stats.Columns[col.Name] = cs
	}
	return stats
}

func (c *Capturer) queryRowCount(ctx context.Context, ident string) (int64, error) {
	var n int64
	err := c.conn.QueryRow(ctx, "SELECT COUNT(*) FROM "+ident).Scan(&n)
	return n, err
}

func (c *Capturer) queryColumnStats(ctx context.Context, ident, column string) (ColumnStats, error) {
	colIdent := pgx.Identifier{column}.Sanitize()
	var (
		cs       ColumnStats
		min, max *string
	)
	err := c.conn.QueryRow(ctx, fmt.Sprintf(
		`SELECT COUNT(%[1]s), COUNT(DISTINCT %[1]s),
		        MIN(%[1]s)::text, MAX(%[1]s)::text
		 FROM %[2]s`, colIdent, ident),
	).Scan(&cs.NonNullCount, &cs.DistinctCount, &min, &max)
	if err != nil {
		return ColumnStats{}, err
	}
	if min != nil {
		cs.Min = *min
	}
	if max != nil {
		cs.Max = *max
	}
	return cs, nil
}

func (c *Capturer) included(table string) bool {
	if len(c.opts.IncludeTables) > 0 {
		return contains(c.opts.IncludeTables, table)
	}
	if len(c.opts.ExcludeTables) > 0 {
		return !contains(c.opts.ExcludeTables, table)
	}
	return true
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
