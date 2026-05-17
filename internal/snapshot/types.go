// Package snapshot captures the schema and per-table statistics of a Postgres
// database at a point in time, and compares two such snapshots to surface
// schema and data changes — aimed at migration-safety review.
//
// By default a snapshot stores only schema metadata and aggregate statistics
// (counts, distinct counts, min/max), never row contents.
package snapshot

import "time"

// Snapshot is the schema + statistics of a database at one point in time.
type Snapshot struct {
	ID         string                     `json:"id"`
	Label      string                     `json:"label"`
	CapturedAt time.Time                  `json:"captured_at"`
	Schema     SchemaState                `json:"schema"`
	Statistics map[string]TableStatistics `json:"statistics"` // keyed by "schema.table"
}

// SchemaState is the structural state of every captured table.
type SchemaState struct {
	Tables map[string]TableSchema `json:"tables"` // keyed by "schema.table"
}

// TableSchema describes one table's structure.
type TableSchema struct {
	Schema  string         `json:"schema"`
	Name    string         `json:"name"`
	Columns []ColumnSchema `json:"columns"`
	Indexes []string       `json:"indexes"`
}

// ColumnSchema describes one column's structure.
type ColumnSchema struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
	Nullable bool   `json:"nullable"`
	Default  string `json:"default,omitempty"`
	Position int    `json:"position"`
}

// TableStatistics holds aggregate statistics for one table.
type TableStatistics struct {
	Table    string                 `json:"table"`
	RowCount int64                  `json:"row_count"`
	Columns  map[string]ColumnStats `json:"columns"`
	// Skipped is set when statistics were not collected (large table or a
	// per-table timeout). RowCount may still be present.
	Skipped       bool   `json:"skipped,omitempty"`
	SkippedReason string `json:"skipped_reason,omitempty"`
}

// ColumnStats holds per-column aggregate statistics. Min/Max are only set for
// orderable types and are stored as strings to stay type-agnostic.
type ColumnStats struct {
	NonNullCount  int64  `json:"non_null_count"`
	NullCount     int64  `json:"null_count"`
	DistinctCount int64  `json:"distinct_count"`
	Min           string `json:"min,omitempty"`
	Max           string `json:"max,omitempty"`
}

// qualifiedName returns the "schema.table" key used in the maps above.
func qualifiedName(schema, table string) string {
	return schema + "." + table
}
