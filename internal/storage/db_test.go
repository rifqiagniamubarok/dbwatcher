package storage

import (
	"testing"
	"time"

	"github.com/rifqiagniamubarok/dbwatcher/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func sampleSnapshot(id, label string) *snapshot.Snapshot {
	return &snapshot.Snapshot{
		ID:         id,
		Label:      label,
		CapturedAt: time.Now().Truncate(time.Second),
		Schema: snapshot.SchemaState{Tables: map[string]snapshot.TableSchema{
			"public.users": {
				Schema: "public", Name: "users",
				Columns: []snapshot.ColumnSchema{
					{Name: "id", DataType: "int4", Nullable: false, Position: 1},
				},
			},
		}},
		Statistics: map[string]snapshot.TableStatistics{
			"public.users": {Table: "public.users", RowCount: 42, Columns: map[string]snapshot.ColumnStats{}},
		},
	}
}

func TestSaveAndLoad_Roundtrip(t *testing.T) {
	db := openTestDB(t)

	want := sampleSnapshot("snap_001", "pre-migration")
	require.NoError(t, db.SaveSnapshot(want))

	got, err := db.LoadSnapshot("pre-migration")
	require.NoError(t, err)
	assert.Equal(t, want.ID, got.ID)
	assert.Equal(t, want.Label, got.Label)
	assert.True(t, want.CapturedAt.Equal(got.CapturedAt))
	assert.Equal(t, int64(42), got.Statistics["public.users"].RowCount)
	require.Len(t, got.Schema.Tables["public.users"].Columns, 1)
	assert.Equal(t, "id", got.Schema.Tables["public.users"].Columns[0].Name)
}

func TestLoad_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.LoadSnapshot("does-not-exist")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestSave_DuplicateLabelReplaces(t *testing.T) {
	db := openTestDB(t)

	require.NoError(t, db.SaveSnapshot(sampleSnapshot("snap_001", "v1")))
	second := sampleSnapshot("snap_002", "v1")
	second.Statistics["public.users"] = snapshot.TableStatistics{Table: "public.users", RowCount: 99}
	require.NoError(t, db.SaveSnapshot(second))

	got, err := db.LoadSnapshot("v1")
	require.NoError(t, err)
	assert.Equal(t, "snap_002", got.ID)
	assert.Equal(t, int64(99), got.Statistics["public.users"].RowCount)

	list, err := db.ListSnapshots()
	require.NoError(t, err)
	assert.Len(t, list, 1, "duplicate label should not create a second row")
}

func TestListSnapshots_NewestFirst(t *testing.T) {
	db := openTestDB(t)

	older := sampleSnapshot("snap_a", "older")
	older.CapturedAt = time.Now().Add(-1 * time.Hour).Truncate(time.Second)
	newer := sampleSnapshot("snap_b", "newer")
	newer.CapturedAt = time.Now().Truncate(time.Second)

	require.NoError(t, db.SaveSnapshot(older))
	require.NoError(t, db.SaveSnapshot(newer))

	list, err := db.ListSnapshots()
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "newer", list[0].Label)
	assert.Equal(t, "older", list[1].Label)
}

func TestDeleteSnapshot(t *testing.T) {
	db := openTestDB(t)

	require.NoError(t, db.SaveSnapshot(sampleSnapshot("snap_001", "to-delete")))
	require.NoError(t, db.DeleteSnapshot("to-delete"))

	_, err := db.LoadSnapshot("to-delete")
	assert.ErrorIs(t, err, ErrNotFound)

	// Deleting again is ErrNotFound.
	assert.ErrorIs(t, db.DeleteSnapshot("to-delete"), ErrNotFound)
}

func TestOpen_PersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()

	db1, err := Open(dir)
	require.NoError(t, err)
	require.NoError(t, db1.SaveSnapshot(sampleSnapshot("snap_001", "persisted")))
	require.NoError(t, db1.Close())

	db2, err := Open(dir)
	require.NoError(t, err)
	defer db2.Close()

	got, err := db2.LoadSnapshot("persisted")
	require.NoError(t, err)
	assert.Equal(t, "snap_001", got.ID)
}

func TestListSnapshots_Empty(t *testing.T) {
	db := openTestDB(t)
	list, err := db.ListSnapshots()
	require.NoError(t, err)
	assert.Empty(t, list)
}
