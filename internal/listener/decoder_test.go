package listener

import (
	"testing"

	"github.com/jackc/pglogrepl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
)

func makeCache() *SchemaCache {
	cache := NewSchemaCache()
	cache.Update(&pglogrepl.RelationMessage{
		RelationID:   1,
		Namespace:    "public",
		RelationName: "users",
		Columns: []*pglogrepl.RelationMessageColumn{
			{Name: "id", DataType: 23, Flags: 1},
			{Name: "name", DataType: 25, Flags: 0},
			{Name: "email", DataType: 25, Flags: 0},
		},
	})
	return cache
}

func tupleData(values ...*pglogrepl.TupleDataColumn) *pglogrepl.TupleData {
	cols := make([]*pglogrepl.TupleDataColumn, len(values))
	copy(cols, values)
	return &pglogrepl.TupleData{Columns: cols}
}

func textCol(v string) *pglogrepl.TupleDataColumn {
	return &pglogrepl.TupleDataColumn{DataType: 't', Data: []byte(v)}
}

func nullCol() *pglogrepl.TupleDataColumn {
	return &pglogrepl.TupleDataColumn{DataType: 'n'}
}

func toastCol() *pglogrepl.TupleDataColumn {
	return &pglogrepl.TupleDataColumn{DataType: 'u'}
}

func TestDecodeMessage_Insert(t *testing.T) {
	cache := makeCache()
	msg := &pglogrepl.InsertMessage{
		RelationID: 1,
		Tuple: tupleData(
			textCol("1"),
			textCol("alice"),
			textCol("alice@example.com"),
		),
	}

	event, err := DecodeMessage(msg, cache)
	require.NoError(t, err)
	require.NotNil(t, event)

	assert.Equal(t, store.EventInsert, event.Type)
	assert.Equal(t, "public", event.Schema)
	assert.Equal(t, "users", event.Table)
	assert.Equal(t, "1", event.NewValues["id"])
	assert.Equal(t, "alice", event.NewValues["name"])
	assert.Equal(t, "alice@example.com", event.NewValues["email"])
	assert.Nil(t, event.OldValues)
}

func TestDecodeMessage_Update(t *testing.T) {
	cache := makeCache()
	msg := &pglogrepl.UpdateMessage{
		RelationID: 1,
		OldTuple: tupleData(
			textCol("1"),
			textCol("alice"),
			textCol("alice@example.com"),
		),
		NewTuple: tupleData(
			textCol("1"),
			textCol("alice"),
			textCol("alice2@example.com"),
		),
	}

	event, err := DecodeMessage(msg, cache)
	require.NoError(t, err)
	require.NotNil(t, event)

	assert.Equal(t, store.EventUpdate, event.Type)
	assert.Equal(t, "alice@example.com", event.OldValues["email"])
	assert.Equal(t, "alice2@example.com", event.NewValues["email"])
}

func TestDecodeMessage_UpdateNoOldTuple(t *testing.T) {
	cache := makeCache()
	msg := &pglogrepl.UpdateMessage{
		RelationID: 1,
		OldTuple:   nil,
		NewTuple: tupleData(
			textCol("1"),
			textCol("bob"),
			textCol("bob@example.com"),
		),
	}

	event, err := DecodeMessage(msg, cache)
	require.NoError(t, err)
	require.NotNil(t, event)

	assert.Equal(t, store.EventUpdate, event.Type)
	assert.Nil(t, event.OldValues)
	assert.Equal(t, "bob", event.NewValues["name"])
}

func TestDecodeMessage_Delete(t *testing.T) {
	cache := makeCache()
	msg := &pglogrepl.DeleteMessage{
		RelationID: 1,
		OldTuple: tupleData(
			textCol("1"),
			textCol("alice"),
			textCol("alice@example.com"),
		),
	}

	event, err := DecodeMessage(msg, cache)
	require.NoError(t, err)
	require.NotNil(t, event)

	assert.Equal(t, store.EventDelete, event.Type)
	assert.Equal(t, "1", event.OldValues["id"])
	assert.Nil(t, event.NewValues)
}

func TestDecodeMessage_NullValue(t *testing.T) {
	cache := makeCache()
	msg := &pglogrepl.InsertMessage{
		RelationID: 1,
		Tuple: tupleData(
			textCol("1"),
			nullCol(),
			nullCol(),
		),
	}

	event, err := DecodeMessage(msg, cache)
	require.NoError(t, err)
	assert.Nil(t, event.NewValues["name"])
	assert.Nil(t, event.NewValues["email"])
}

func TestDecodeMessage_TOASTValue(t *testing.T) {
	cache := makeCache()
	msg := &pglogrepl.UpdateMessage{
		RelationID: 1,
		NewTuple: tupleData(
			textCol("1"),
			textCol("alice"),
			toastCol(), // email unchanged, not shipped
		),
	}

	event, err := DecodeMessage(msg, cache)
	require.NoError(t, err)
	assert.Equal(t, "[unchanged]", event.NewValues["email"])
}

func TestDecodeMessage_SkipNonDML(t *testing.T) {
	cache := makeCache()

	skippable := []pglogrepl.Message{
		&pglogrepl.BeginMessage{},
		&pglogrepl.CommitMessage{},
		&pglogrepl.RelationMessage{},
		&pglogrepl.TypeMessage{},
	}

	for _, msg := range skippable {
		event, err := DecodeMessage(msg, cache)
		assert.NoError(t, err)
		assert.Nil(t, event)
	}
}

func TestDecodeMessage_UnknownRelation(t *testing.T) {
	cache := NewSchemaCache() // empty cache
	msg := &pglogrepl.InsertMessage{
		RelationID: 99,
		Tuple:      tupleData(textCol("1")),
	}

	_, err := DecodeMessage(msg, cache)
	assert.Error(t, err)
}
