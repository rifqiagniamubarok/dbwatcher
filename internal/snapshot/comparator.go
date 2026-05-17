package snapshot

import "sort"

// ChangeKind classifies a single change in a CompareReport.
type ChangeKind string

const (
	ChangeAdded    ChangeKind = "added"
	ChangeRemoved  ChangeKind = "removed"
	ChangeModified ChangeKind = "modified"
)

// SchemaChange is one structural difference between two snapshots.
type SchemaChange struct {
	Kind     ChangeKind `json:"kind"`
	Table    string     `json:"table"`
	Column   string     `json:"column,omitempty"` // empty when the change is table-level
	Detail   string     `json:"detail"`           // human-readable description
	Breaking bool       `json:"breaking"`         // true for changes that can break callers
}

// DataChange summarizes statistical drift for one table.
type DataChange struct {
	Table      string             `json:"table"`
	RowCountFrom int64            `json:"row_count_from"`
	RowCountTo   int64            `json:"row_count_to"`
	Columns    []ColumnDataChange `json:"columns,omitempty"`
	Note       string             `json:"note,omitempty"` // e.g. "table removed, comparison skipped"
}

// ColumnDataChange is statistical drift for one column.
type ColumnDataChange struct {
	Column        string `json:"column"`
	NullFrom      int64  `json:"null_from"`
	NullTo        int64  `json:"null_to"`
	NonNullFrom   int64  `json:"non_null_from"`
	NonNullTo     int64  `json:"non_null_to"`
	DistinctFrom  int64  `json:"distinct_from"`
	DistinctTo    int64  `json:"distinct_to"`
	Notable       bool   `json:"notable"` // worth flagging (e.g. NULLs disappeared)
}

// CompareReport is the full diff between two snapshots.
type CompareReport struct {
	FromLabel     string         `json:"from_label"`
	ToLabel       string         `json:"to_label"`
	SchemaChanges []SchemaChange `json:"schema_changes"`
	DataChanges   []DataChange   `json:"data_changes"`
}

// BreakingCount returns how many schema changes are breaking.
func (r *CompareReport) BreakingCount() int {
	n := 0
	for _, c := range r.SchemaChanges {
		if c.Breaking {
			n++
		}
	}
	return n
}

// NotableDataCount returns how many column data changes are notable.
func (r *CompareReport) NotableDataCount() int {
	n := 0
	for _, dc := range r.DataChanges {
		for _, cc := range dc.Columns {
			if cc.Notable {
				n++
			}
		}
	}
	return n
}

// Compare diffs two snapshots: `a` is the earlier/baseline, `b` is the later
// state (often "now"). The report describes how to get from a to b.
func Compare(a, b *Snapshot) *CompareReport {
	report := &CompareReport{
		FromLabel: a.Label,
		ToLabel:   b.Label,
	}
	report.SchemaChanges = compareSchema(a.Schema, b.Schema)
	report.DataChanges = compareStatistics(a, b)
	return report
}

func compareSchema(a, b SchemaState) []SchemaChange {
	var changes []SchemaChange

	for _, name := range sortedKeys(a.Tables, b.Tables) {
		ta, inA := a.Tables[name]
		tb, inB := b.Tables[name]

		switch {
		case !inA && inB:
			changes = append(changes, SchemaChange{
				Kind: ChangeAdded, Table: name,
				Detail: "table added",
			})
		case inA && !inB:
			changes = append(changes, SchemaChange{
				Kind: ChangeRemoved, Table: name,
				Detail: "table removed", Breaking: true,
			})
		default:
			changes = append(changes, compareColumns(name, ta, tb)...)
		}
	}
	return changes
}

func compareColumns(table string, a, b TableSchema) []SchemaChange {
	var changes []SchemaChange

	colsA := indexColumns(a.Columns)
	colsB := indexColumns(b.Columns)

	for _, name := range sortedColumnNames(colsA, colsB) {
		ca, inA := colsA[name]
		cb, inB := colsB[name]

		switch {
		case !inA && inB:
			changes = append(changes, SchemaChange{
				Kind: ChangeAdded, Table: table, Column: name,
				Detail: "column added (" + cb.DataType + describeNullable(cb.Nullable) + ")",
			})
		case inA && !inB:
			changes = append(changes, SchemaChange{
				Kind: ChangeRemoved, Table: table, Column: name,
				Detail: "column removed", Breaking: true,
			})
		default:
			if c := diffColumn(table, ca, cb); c != nil {
				changes = append(changes, *c)
			}
		}
	}
	return changes
}

// diffColumn reports a single modified-column change, or nil if unchanged.
func diffColumn(table string, a, b ColumnSchema) *SchemaChange {
	switch {
	case a.DataType != b.DataType:
		return &SchemaChange{
			Kind: ChangeModified, Table: table, Column: b.Name,
			Detail:   "type changed: " + a.DataType + " → " + b.DataType,
			Breaking: true,
		}
	case a.Nullable && !b.Nullable:
		return &SchemaChange{
			Kind: ChangeModified, Table: table, Column: b.Name,
			Detail:   "NULL → NOT NULL",
			Breaking: true,
		}
	case !a.Nullable && b.Nullable:
		return &SchemaChange{
			Kind: ChangeModified, Table: table, Column: b.Name,
			Detail: "NOT NULL → NULL",
		}
	case a.Default != b.Default:
		return &SchemaChange{
			Kind: ChangeModified, Table: table, Column: b.Name,
			Detail: "default changed: " + emptyAsNone(a.Default) + " → " + emptyAsNone(b.Default),
		}
	}
	return nil
}

func compareStatistics(a, b *Snapshot) []DataChange {
	var changes []DataChange

	for _, name := range sortedKeys(a.Statistics, b.Statistics) {
		sa, inA := a.Statistics[name]
		sb, inB := b.Statistics[name]

		switch {
		case !inA && inB:
			changes = append(changes, DataChange{
				Table: name, RowCountTo: sb.RowCount,
				Note: "new table",
			})
			continue
		case inA && !inB:
			changes = append(changes, DataChange{
				Table: name, RowCountFrom: sa.RowCount,
				Note: "table removed, comparison skipped",
			})
			continue
		}

		dc := DataChange{
			Table:        name,
			RowCountFrom: sa.RowCount,
			RowCountTo:   sb.RowCount,
		}
		if sa.Skipped || sb.Skipped {
			dc.Note = "statistics skipped for a large table"
		} else {
			dc.Columns = compareColumnStats(sa, sb)
		}
		changes = append(changes, dc)
	}
	return changes
}

func compareColumnStats(a, b TableStatistics) []ColumnDataChange {
	var changes []ColumnDataChange
	for _, col := range sortedColumnStatNames(a.Columns, b.Columns) {
		ca, inA := a.Columns[col]
		cb, inB := b.Columns[col]
		if !inA || !inB {
			continue // column added/removed — already covered by schema diff
		}
		if ca == cb {
			continue
		}
		cc := ColumnDataChange{
			Column:       col,
			NullFrom:     ca.NullCount,
			NullTo:       cb.NullCount,
			NonNullFrom:  ca.NonNullCount,
			NonNullTo:    cb.NonNullCount,
			DistinctFrom: ca.DistinctCount,
			DistinctTo:   cb.DistinctCount,
		}
		// Notable: NULLs that previously existed are now gone — a classic
		// "rows were silently backfilled" signal during a migration.
		if ca.NullCount > 0 && cb.NullCount < ca.NullCount {
			cc.Notable = true
		}
		changes = append(changes, cc)
	}
	return changes
}

// --- helpers ---

func indexColumns(cols []ColumnSchema) map[string]ColumnSchema {
	m := make(map[string]ColumnSchema, len(cols))
	for _, c := range cols {
		m[c.Name] = c
	}
	return m
}

func describeNullable(nullable bool) string {
	if nullable {
		return ", nullable"
	}
	return ", NOT NULL"
}

func emptyAsNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func sortedKeys[V any](a, b map[string]V) []string {
	seen := make(map[string]struct{})
	for k := range a {
		seen[k] = struct{}{}
	}
	for k := range b {
		seen[k] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedColumnNames(a, b map[string]ColumnSchema) []string {
	return sortedKeys(a, b)
}

func sortedColumnStatNames(a, b map[string]ColumnStats) []string {
	return sortedKeys(a, b)
}
