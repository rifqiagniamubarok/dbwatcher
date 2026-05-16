package ddlwatcher

import (
	"testing"

	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePayload_CreateTable(t *testing.T) {
	raw := `{"command_tag":"CREATE TABLE","object_type":"table","schema":"public","object_identity":"public.users","in_extension":false,"timestamp":1747000000.5}`
	e, err := parsePayload(raw)
	require.NoError(t, err)
	assert.Equal(t, store.KindDDL, e.Kind)
	assert.Equal(t, "CREATE TABLE", e.CommandTag)
	assert.Equal(t, "table", e.ObjectType)
	assert.Equal(t, "public.users", e.ObjectIdentity)
	assert.Equal(t, int64(1747000000), e.Timestamp.Unix())
}

func TestParsePayload_AlterTable(t *testing.T) {
	raw := `{"command_tag":"ALTER TABLE","object_type":"table","schema":"public","object_identity":"public.users","timestamp":1747000001}`
	e, err := parsePayload(raw)
	require.NoError(t, err)
	assert.Equal(t, "ALTER TABLE", e.CommandTag)
	assert.True(t, e.IsDDL())
}

func TestParsePayload_CreateIndex(t *testing.T) {
	raw := `{"command_tag":"CREATE INDEX","object_type":"index","schema":"public","object_identity":"public.idx_users_email"}`
	e, err := parsePayload(raw)
	require.NoError(t, err)
	assert.Equal(t, "CREATE INDEX", e.CommandTag)
	assert.Equal(t, "index", e.ObjectType)
	assert.Equal(t, "public.idx_users_email", e.ObjectIdentity)
}

func TestParsePayload_DropTable(t *testing.T) {
	raw := `{"command_tag":"DROP TABLE","object_type":"table","schema":"public","object_identity":"public.old_table","timestamp":1747000002}`
	e, err := parsePayload(raw)
	require.NoError(t, err)
	assert.Equal(t, "DROP TABLE", e.CommandTag)
	assert.Equal(t, "public.old_table", e.ObjectIdentity)
}

func TestParsePayload_EmptyIdentityFallsBackToSchema(t *testing.T) {
	raw := `{"command_tag":"CREATE SCHEMA","object_type":"schema","schema":"analytics","object_identity":""}`
	e, err := parsePayload(raw)
	require.NoError(t, err)
	assert.Equal(t, "analytics", e.ObjectIdentity)
}

func TestParsePayload_MalformedJSON(t *testing.T) {
	_, err := parsePayload(`{not valid json`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode ddl payload")
}

func TestParsePayload_EmptyCommandTag(t *testing.T) {
	_, err := parsePayload(`{"command_tag":"","object_identity":"public.users"}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty command_tag")
}

func TestParsePayload_NoTimestamp(t *testing.T) {
	// Missing timestamp must not crash — Timestamp is left as NewDDL's now().
	raw := `{"command_tag":"ALTER TABLE","object_type":"table","object_identity":"public.users"}`
	e, err := parsePayload(raw)
	require.NoError(t, err)
	assert.False(t, e.Timestamp.IsZero())
}

func TestParsePayload_WhitespaceTrimmed(t *testing.T) {
	raw := `{"command_tag":"  ALTER TABLE  ","object_type":" table ","object_identity":"  public.users  "}`
	e, err := parsePayload(raw)
	require.NoError(t, err)
	assert.Equal(t, "ALTER TABLE", e.CommandTag)
	assert.Equal(t, "table", e.ObjectType)
	assert.Equal(t, "public.users", e.ObjectIdentity)
}

func TestPrintSQL_ContainsTriggers(t *testing.T) {
	sql := PrintSQL()
	for _, want := range []string{
		captureFuncName, dropCaptureFuncName,
		endTriggerName, dropTriggerName,
		"ddl_command_end", "sql_drop", notifyChannel,
	} {
		assert.Contains(t, sql, want)
	}
}

func TestStripReplicationParam(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{
			"postgres://u:p@h:5432/db?replication=database&sslmode=disable",
			"postgres://u:p@h:5432/db?sslmode=disable",
		},
		{
			"postgres://u:p@h:5432/db?sslmode=disable",
			"postgres://u:p@h:5432/db?sslmode=disable",
		},
		{
			"postgres://u:p@h:5432/db?replication=database",
			"postgres://u:p@h:5432/db",
		},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, stripReplicationParam(c.in))
	}
}
