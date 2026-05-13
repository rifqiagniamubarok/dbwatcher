package listener

import (
	"testing"

	"github.com/jackc/pglogrepl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaCache_UpdateAndGet(t *testing.T) {
	cache := NewSchemaCache()

	rel := &pglogrepl.RelationMessage{
		RelationID:  42,
		Namespace:   "public",
		RelationName: "users",
		Columns: []*pglogrepl.RelationMessageColumn{
			{Name: "id", DataType: 23, Flags: 1},   // int4, key
			{Name: "name", DataType: 25, Flags: 0},  // text
			{Name: "email", DataType: 25, Flags: 0}, // text
		},
	}

	cache.Update(rel)

	meta, ok := cache.Get(42)
	require.True(t, ok)
	assert.Equal(t, "public", meta.Schema)
	assert.Equal(t, "users", meta.Table)
	assert.Len(t, meta.Columns, 3)
	assert.Equal(t, "id", meta.Columns[0].Name)
	assert.True(t, meta.Columns[0].IsKey)
	assert.Equal(t, "name", meta.Columns[1].Name)
	assert.False(t, meta.Columns[1].IsKey)
}

func TestSchemaCache_GetMissing(t *testing.T) {
	cache := NewSchemaCache()
	_, ok := cache.Get(999)
	assert.False(t, ok)
}

func TestSchemaCache_UpdateOverwrite(t *testing.T) {
	cache := NewSchemaCache()

	rel1 := &pglogrepl.RelationMessage{
		RelationID:   10,
		Namespace:    "public",
		RelationName: "orders",
		Columns: []*pglogrepl.RelationMessageColumn{
			{Name: "id", DataType: 23, Flags: 1},
		},
	}
	cache.Update(rel1)

	// Same OID, different columns (schema change at runtime)
	rel2 := &pglogrepl.RelationMessage{
		RelationID:   10,
		Namespace:    "public",
		RelationName: "orders",
		Columns: []*pglogrepl.RelationMessageColumn{
			{Name: "id", DataType: 23, Flags: 1},
			{Name: "total", DataType: 701, Flags: 0}, // float8
		},
	}
	cache.Update(rel2)

	meta, ok := cache.Get(10)
	require.True(t, ok)
	assert.Len(t, meta.Columns, 2)
	assert.Equal(t, "total", meta.Columns[1].Name)
}

func TestSchemaCache_DataTypeNames(t *testing.T) {
	cache := NewSchemaCache()

	rel := &pglogrepl.RelationMessage{
		RelationID:   1,
		Namespace:    "public",
		RelationName: "test",
		Columns: []*pglogrepl.RelationMessageColumn{
			{Name: "a", DataType: 23},  // int4
			{Name: "b", DataType: 25},  // text
			{Name: "c", DataType: 16},  // bool
			{Name: "d", DataType: 701}, // float8
			{Name: "e", DataType: 1114}, // timestamp
			{Name: "f", DataType: 114}, // json
			{Name: "g", DataType: 3802}, // jsonb
			{Name: "h", DataType: 9999}, // unknown OID
		},
	}
	cache.Update(rel)

	meta, ok := cache.Get(1)
	require.True(t, ok)
	assert.Equal(t, "int4", meta.Columns[0].DataType)
	assert.Equal(t, "text", meta.Columns[1].DataType)
	assert.Equal(t, "bool", meta.Columns[2].DataType)
	assert.Equal(t, "float8", meta.Columns[3].DataType)
	assert.Equal(t, "timestamp", meta.Columns[4].DataType)
	assert.Equal(t, "json", meta.Columns[5].DataType)
	assert.Equal(t, "jsonb", meta.Columns[6].DataType)
	assert.Equal(t, "unknown", meta.Columns[7].DataType)
}
