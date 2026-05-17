package store

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMarker_FillsDefaults(t *testing.T) {
	m := NewMarker("TEST: create order", "")
	assert.Equal(t, KindMarker, m.Kind)
	assert.Equal(t, "TEST: create order", m.Label)
	assert.Equal(t, MarkerColorDefault, m.Color)
	assert.False(t, m.Timestamp.IsZero())
	assert.True(t, m.IsMarker())
	assert.False(t, m.IsLog())
	assert.False(t, m.IsDBEvent())
}

func TestNewMarker_KeepsExplicitColor(t *testing.T) {
	m := NewMarker("deploy", MarkerColorYellow)
	assert.Equal(t, MarkerColorYellow, m.Color)
}

func TestNewLog_PopulatesMessage(t *testing.T) {
	l := NewLog("starting test suite")
	assert.Equal(t, KindLog, l.Kind)
	assert.Equal(t, "starting test suite", l.Message)
	assert.True(t, l.IsLog())
	assert.False(t, l.IsMarker())
	assert.False(t, l.IsDBEvent())
}

func TestNewDDL_PopulatesFields(t *testing.T) {
	d := NewDDL("ALTER TABLE", "table", "public.users")
	assert.Equal(t, KindDDL, d.Kind)
	assert.Equal(t, "ALTER TABLE", d.CommandTag)
	assert.Equal(t, "table", d.ObjectType)
	assert.Equal(t, "public.users", d.ObjectIdentity)
	assert.False(t, d.Timestamp.IsZero())
	assert.True(t, d.IsDDL())
	assert.False(t, d.IsDBEvent())
	assert.False(t, d.IsMarker())
	assert.False(t, d.IsLog())
}

func TestEvent_String_DDL(t *testing.T) {
	s := NewDDL("CREATE INDEX", "index", "public.idx_users_phone").String()
	assert.Contains(t, s, "DDL")
	assert.Contains(t, s, "CREATE INDEX")
	assert.Contains(t, s, "public.idx_users_phone")
}

func TestEvent_JSON_DDL_OmitsIrrelevantFields(t *testing.T) {
	d := NewDDL("DROP TABLE", "table", "public.old_table")
	raw, err := d.JSON()
	require.NoError(t, err)
	assert.Contains(t, raw, `"kind":"ddl"`)
	assert.Contains(t, raw, `"command_tag":"DROP TABLE"`)
	assert.Contains(t, raw, `"object_type":"table"`)
	assert.Contains(t, raw, `"object_identity":"public.old_table"`)
	// DDL events carry no data-change fields.
	assert.NotContains(t, raw, `"new_values"`)
	assert.NotContains(t, raw, `"label"`)
	assert.NotContains(t, raw, `"message"`)
}

func TestEvent_IsDBEvent_BackwardCompatible(t *testing.T) {
	// An Event built before the Kind field existed has Kind="" and must
	// still be classified as a database event.
	e := Event{ID: 1, Type: EventInsert, Table: "users"}
	assert.True(t, e.IsDBEvent())
	assert.False(t, e.IsMarker())
	assert.False(t, e.IsLog())
}

func TestEvent_String_PerKind(t *testing.T) {
	t.Run("marker", func(t *testing.T) {
		s := NewMarker("create order", "").String()
		assert.Contains(t, s, "MARKER")
		assert.Contains(t, s, "create order")
	})
	t.Run("log", func(t *testing.T) {
		s := NewLog("migrations done").String()
		assert.Contains(t, s, "LOG")
		assert.Contains(t, s, "migrations done")
	})
	t.Run("db_event", func(t *testing.T) {
		e := Event{Type: EventInsert, Schema: "public", Table: "users"}
		s := e.String()
		assert.Contains(t, s, "INSERT")
		assert.Contains(t, s, "public.users")
	})
}

func TestEvent_JSON_OmitsEmptyFields(t *testing.T) {
	m := NewMarker("deploy v1.2", MarkerColorGreen)
	raw, err := m.JSON()
	require.NoError(t, err)

	// Marker JSON must not include database-specific fields.
	assert.NotContains(t, raw, `"table"`)
	assert.NotContains(t, raw, `"new_values"`)
	assert.NotContains(t, raw, `"lsn"`)
	// And must include the marker fields.
	assert.Contains(t, raw, `"kind":"marker"`)
	assert.Contains(t, raw, `"label":"deploy v1.2"`)
	assert.Contains(t, raw, `"color":"green"`)
}

func TestEvent_JSON_LegacyEventRoundtrip(t *testing.T) {
	// A pre-Phase-6 JSON payload (no Kind, no marker/log fields) must
	// decode into an Event that classifies as a DB event.
	legacy := `{"id":42,"timestamp":"2026-05-15T10:00:00Z","type":"INSERT","schema":"public","table":"users","new_values":{"id":1}}`
	var e Event
	require.NoError(t, json.Unmarshal([]byte(legacy), &e))
	assert.True(t, e.IsDBEvent())
	assert.Equal(t, EventInsert, e.Type)
	assert.Equal(t, "users", e.Table)
}

func TestAllowedMarkerColors_HasAll(t *testing.T) {
	// Guard against accidental drift between the constants and the
	// validation list (the marker API depends on this list).
	for _, want := range []string{
		MarkerColorDefault, MarkerColorYellow, MarkerColorGreen,
		MarkerColorRed, MarkerColorBlue, MarkerColorDim,
	} {
		found := false
		for _, c := range AllowedMarkerColors {
			if c == want {
				found = true
				break
			}
		}
		assert.Truef(t, found, "AllowedMarkerColors missing %q", want)
	}
}

func TestEvent_JSON_DBEvent_NoMarkerFields(t *testing.T) {
	e := Event{ID: 1, Type: EventInsert, Schema: "public", Table: "users"}
	raw, err := e.JSON()
	require.NoError(t, err)
	// Legacy/DB event JSON should not leak marker-only keys.
	for _, banned := range []string{`"label"`, `"color"`, `"message"`} {
		assert.False(t, strings.Contains(raw, banned), "unexpected key %s in %s", banned, raw)
	}
}
