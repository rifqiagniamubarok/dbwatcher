package listener

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jackc/pglogrepl"

	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
)

var eventCounter atomic.Uint64

// DecodeMessage converts a pgoutput message into an Event.
// Returns (nil, nil) for non-DML messages (Begin, Commit, Relation, Type).
func DecodeMessage(msg pglogrepl.Message, cache *SchemaCache) (*store.Event, error) {
	switch m := msg.(type) {
	case *pglogrepl.InsertMessage:
		return decodeInsert(m, cache)
	case *pglogrepl.UpdateMessage:
		return decodeUpdate(m, cache)
	case *pglogrepl.DeleteMessage:
		return decodeDelete(m, cache)
	case *pglogrepl.BeginMessage,
		*pglogrepl.CommitMessage,
		*pglogrepl.RelationMessage,
		*pglogrepl.TypeMessage:
		return nil, nil
	default:
		return nil, nil
	}
}

func decodeInsert(m *pglogrepl.InsertMessage, cache *SchemaCache) (*store.Event, error) {
	meta, ok := cache.Get(m.RelationID)
	if !ok {
		return nil, fmt.Errorf("decode insert: unknown relation OID %d", m.RelationID)
	}
	newVals := decodeTuple(m.Tuple, meta.Columns)
	return newEvent(store.EventInsert, meta, newVals, nil), nil
}

func decodeUpdate(m *pglogrepl.UpdateMessage, cache *SchemaCache) (*store.Event, error) {
	meta, ok := cache.Get(m.RelationID)
	if !ok {
		return nil, fmt.Errorf("decode update: unknown relation OID %d", m.RelationID)
	}
	newVals := decodeTuple(m.NewTuple, meta.Columns)
	var oldVals map[string]any
	if m.OldTuple != nil {
		oldVals = decodeTuple(m.OldTuple, meta.Columns)
	}
	return newEvent(store.EventUpdate, meta, newVals, oldVals), nil
}

func decodeDelete(m *pglogrepl.DeleteMessage, cache *SchemaCache) (*store.Event, error) {
	meta, ok := cache.Get(m.RelationID)
	if !ok {
		return nil, fmt.Errorf("decode delete: unknown relation OID %d", m.RelationID)
	}
	oldVals := decodeTuple(m.OldTuple, meta.Columns)
	return newEvent(store.EventDelete, meta, nil, oldVals), nil
}

func decodeTuple(tuple *pglogrepl.TupleData, cols []store.Column) map[string]any {
	if tuple == nil {
		return nil
	}
	result := make(map[string]any, len(tuple.Columns))
	for i, col := range tuple.Columns {
		if i >= len(cols) {
			break
		}
		name := cols[i].Name
		switch col.DataType {
		case pglogrepl.TupleDataTypeNull:
			result[name] = nil
		case pglogrepl.TupleDataTypeToast:
			result[name] = "[unchanged]"
		case pglogrepl.TupleDataTypeText:
			result[name] = string(col.Data)
		}
	}
	return result
}

func newEvent(t store.EventType, meta TableMetadata, newVals, oldVals map[string]any) *store.Event {
	return &store.Event{
		ID:        eventCounter.Add(1),
		Timestamp: time.Now().UTC(),
		Type:      t,
		Schema:    meta.Schema,
		Table:     meta.Table,
		Columns:   meta.Columns,
		NewValues: newVals,
		OldValues: oldVals,
	}
}
