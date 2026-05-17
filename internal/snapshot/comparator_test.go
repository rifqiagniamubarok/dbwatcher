package snapshot

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tableSchema builds a TableSchema from a column spec for terse tests.
func col(name, typ string, nullable bool) ColumnSchema {
	return ColumnSchema{Name: name, DataType: typ, Nullable: nullable}
}

func snap(label string, tables map[string]TableSchema, stats map[string]TableStatistics) *Snapshot {
	return &Snapshot{
		Label:      label,
		CapturedAt: time.Now(),
		Schema:     SchemaState{Tables: tables},
		Statistics: stats,
	}
}

func TestCompare_TableAdded(t *testing.T) {
	a := snap("before", map[string]TableSchema{}, nil)
	b := snap("after", map[string]TableSchema{
		"public.orders": {Schema: "public", Name: "orders"},
	}, nil)

	r := Compare(a, b)
	require.Len(t, r.SchemaChanges, 1)
	assert.Equal(t, ChangeAdded, r.SchemaChanges[0].Kind)
	assert.Equal(t, "public.orders", r.SchemaChanges[0].Table)
	assert.False(t, r.SchemaChanges[0].Breaking)
}

func TestCompare_TableRemovedIsBreaking(t *testing.T) {
	a := snap("before", map[string]TableSchema{
		"public.sessions": {Schema: "public", Name: "sessions"},
	}, nil)
	b := snap("after", map[string]TableSchema{}, nil)

	r := Compare(a, b)
	require.Len(t, r.SchemaChanges, 1)
	assert.Equal(t, ChangeRemoved, r.SchemaChanges[0].Kind)
	assert.True(t, r.SchemaChanges[0].Breaking)
	assert.Equal(t, 1, r.BreakingCount())
}

func TestCompare_ColumnAdded(t *testing.T) {
	a := snap("before", map[string]TableSchema{
		"public.users": {Schema: "public", Name: "users", Columns: []ColumnSchema{
			col("id", "int4", false),
		}},
	}, nil)
	b := snap("after", map[string]TableSchema{
		"public.users": {Schema: "public", Name: "users", Columns: []ColumnSchema{
			col("id", "int4", false),
			col("gender", "text", true),
		}},
	}, nil)

	r := Compare(a, b)
	require.Len(t, r.SchemaChanges, 1)
	c := r.SchemaChanges[0]
	assert.Equal(t, ChangeAdded, c.Kind)
	assert.Equal(t, "gender", c.Column)
	assert.Contains(t, c.Detail, "text")
	assert.False(t, c.Breaking)
}

func TestCompare_ColumnRemovedIsBreaking(t *testing.T) {
	a := snap("before", map[string]TableSchema{
		"public.users": {Schema: "public", Name: "users", Columns: []ColumnSchema{
			col("id", "int4", false),
			col("legacy", "text", true),
		}},
	}, nil)
	b := snap("after", map[string]TableSchema{
		"public.users": {Schema: "public", Name: "users", Columns: []ColumnSchema{
			col("id", "int4", false),
		}},
	}, nil)

	r := Compare(a, b)
	require.Len(t, r.SchemaChanges, 1)
	assert.Equal(t, ChangeRemoved, r.SchemaChanges[0].Kind)
	assert.True(t, r.SchemaChanges[0].Breaking)
}

func TestCompare_NullToNotNullIsBreaking(t *testing.T) {
	a := snap("before", map[string]TableSchema{
		"public.users": {Schema: "public", Name: "users", Columns: []ColumnSchema{
			col("phone", "text", true),
		}},
	}, nil)
	b := snap("after", map[string]TableSchema{
		"public.users": {Schema: "public", Name: "users", Columns: []ColumnSchema{
			col("phone", "text", false),
		}},
	}, nil)

	r := Compare(a, b)
	require.Len(t, r.SchemaChanges, 1)
	c := r.SchemaChanges[0]
	assert.Equal(t, ChangeModified, c.Kind)
	assert.Contains(t, c.Detail, "NOT NULL")
	assert.True(t, c.Breaking)
}

func TestCompare_TypeChangeIsBreaking(t *testing.T) {
	a := snap("before", map[string]TableSchema{
		"public.users": {Schema: "public", Name: "users", Columns: []ColumnSchema{
			col("age", "int4", true),
		}},
	}, nil)
	b := snap("after", map[string]TableSchema{
		"public.users": {Schema: "public", Name: "users", Columns: []ColumnSchema{
			col("age", "int8", true),
		}},
	}, nil)

	r := Compare(a, b)
	require.Len(t, r.SchemaChanges, 1)
	assert.True(t, r.SchemaChanges[0].Breaking)
	assert.Contains(t, r.SchemaChanges[0].Detail, "int4 → int8")
}

func TestCompare_NotNullToNullNotBreaking(t *testing.T) {
	a := snap("before", map[string]TableSchema{
		"public.users": {Schema: "public", Name: "users", Columns: []ColumnSchema{
			col("phone", "text", false),
		}},
	}, nil)
	b := snap("after", map[string]TableSchema{
		"public.users": {Schema: "public", Name: "users", Columns: []ColumnSchema{
			col("phone", "text", true),
		}},
	}, nil)

	r := Compare(a, b)
	require.Len(t, r.SchemaChanges, 1)
	assert.False(t, r.SchemaChanges[0].Breaking)
}

func TestCompare_IdenticalSchemaNoChanges(t *testing.T) {
	tables := map[string]TableSchema{
		"public.users": {Schema: "public", Name: "users", Columns: []ColumnSchema{
			col("id", "int4", false),
		}},
	}
	r := Compare(snap("a", tables, nil), snap("b", tables, nil))
	assert.Empty(t, r.SchemaChanges)
}

func TestCompare_DataChange_NullsDisappearIsNotable(t *testing.T) {
	statsA := map[string]TableStatistics{
		"public.users": {Table: "public.users", RowCount: 100, Columns: map[string]ColumnStats{
			"phone": {NonNullCount: 97, NullCount: 3, DistinctCount: 97},
		}},
	}
	statsB := map[string]TableStatistics{
		"public.users": {Table: "public.users", RowCount: 100, Columns: map[string]ColumnStats{
			"phone": {NonNullCount: 100, NullCount: 0, DistinctCount: 98},
		}},
	}
	a := snap("before", map[string]TableSchema{"public.users": {}}, statsA)
	b := snap("after", map[string]TableSchema{"public.users": {}}, statsB)

	r := Compare(a, b)
	require.Len(t, r.DataChanges, 1)
	dc := r.DataChanges[0]
	require.Len(t, dc.Columns, 1)
	assert.True(t, dc.Columns[0].Notable)
	assert.Equal(t, int64(3), dc.Columns[0].NullFrom)
	assert.Equal(t, int64(0), dc.Columns[0].NullTo)
	assert.Equal(t, 1, r.NotableDataCount())
}

func TestCompare_DataChange_RowCountOnly(t *testing.T) {
	statsA := map[string]TableStatistics{
		"public.orders": {Table: "public.orders", RowCount: 250, Columns: map[string]ColumnStats{}},
	}
	statsB := map[string]TableStatistics{
		"public.orders": {Table: "public.orders", RowCount: 260, Columns: map[string]ColumnStats{}},
	}
	r := Compare(
		snap("a", map[string]TableSchema{"public.orders": {}}, statsA),
		snap("b", map[string]TableSchema{"public.orders": {}}, statsB),
	)
	require.Len(t, r.DataChanges, 1)
	assert.Equal(t, int64(250), r.DataChanges[0].RowCountFrom)
	assert.Equal(t, int64(260), r.DataChanges[0].RowCountTo)
}

func TestCompare_DataChange_SkippedTable(t *testing.T) {
	statsA := map[string]TableStatistics{
		"public.big": {Table: "public.big", RowCount: 50_000_000, Skipped: true, SkippedReason: "large table"},
	}
	statsB := map[string]TableStatistics{
		"public.big": {Table: "public.big", RowCount: 50_000_001, Skipped: true, SkippedReason: "large table"},
	}
	r := Compare(
		snap("a", map[string]TableSchema{"public.big": {}}, statsA),
		snap("b", map[string]TableSchema{"public.big": {}}, statsB),
	)
	require.Len(t, r.DataChanges, 1)
	assert.Contains(t, r.DataChanges[0].Note, "skipped")
	assert.Empty(t, r.DataChanges[0].Columns)
}

func TestCompare_DefaultChange(t *testing.T) {
	a := snap("before", map[string]TableSchema{
		"public.t": {Columns: []ColumnSchema{{Name: "status", DataType: "text", Default: ""}}},
	}, nil)
	b := snap("after", map[string]TableSchema{
		"public.t": {Columns: []ColumnSchema{{Name: "status", DataType: "text", Default: "'active'"}}},
	}, nil)

	r := Compare(a, b)
	require.Len(t, r.SchemaChanges, 1)
	assert.Equal(t, ChangeModified, r.SchemaChanges[0].Kind)
	assert.Contains(t, r.SchemaChanges[0].Detail, "default changed")
	assert.False(t, r.SchemaChanges[0].Breaking)
}
