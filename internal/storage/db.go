// Package storage persists DBWatch snapshots in a local SQLite database.
// It uses modernc.org/sqlite (pure Go, no CGo) so the binary stays
// statically linked and cross-compiles cleanly.
package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/rifqiagniamubarok/dbwatcher/internal/snapshot"
)

// ErrNotFound is returned when a snapshot label does not exist.
var ErrNotFound = errors.New("snapshot not found")

// DBFileName is the SQLite file created inside the data directory.
const DBFileName = "data.db"

// DB is a handle to the snapshot store.
type DB struct {
	sql *sql.DB
}

// SnapshotMeta is the lightweight listing form of a snapshot (no payload).
type SnapshotMeta struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"created_at"`
}

// Open opens (creating if needed) the snapshot database under dataDir and
// applies the schema. The directory is created with 0700.
func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	path := filepath.Join(dataDir, DBFileName)

	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite handles one writer at a time; a single connection avoids
	// "database is locked" churn for this low-traffic local store.
	sqlDB.SetMaxOpenConns(1)

	db := &DB{sql: sqlDB}
	if err := db.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}

// Close releases the underlying database handle.
func (db *DB) Close() error { return db.sql.Close() }

func (db *DB) migrate() error {
	_, err := db.sql.Exec(`
		CREATE TABLE IF NOT EXISTS snapshots (
			id         TEXT PRIMARY KEY,
			label      TEXT NOT NULL UNIQUE,
			created_at TIMESTAMP NOT NULL,
			data       BLOB NOT NULL
		)`)
	if err != nil {
		return fmt.Errorf("migrate snapshots table: %w", err)
	}
	return nil
}

// SaveSnapshot stores a snapshot. The label must be unique; saving with an
// existing label replaces the prior snapshot under that label.
func (db *DB) SaveSnapshot(s *snapshot.Snapshot) error {
	payload, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	_, err = db.sql.Exec(`
		INSERT INTO snapshots (id, label, created_at, data)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(label) DO UPDATE SET
			id = excluded.id,
			created_at = excluded.created_at,
			data = excluded.data`,
		s.ID, s.Label, s.CapturedAt, payload)
	if err != nil {
		return fmt.Errorf("save snapshot %q: %w", s.Label, err)
	}
	return nil
}

// LoadSnapshot retrieves a snapshot by label. Returns ErrNotFound if absent.
func (db *DB) LoadSnapshot(label string) (*snapshot.Snapshot, error) {
	var payload []byte
	err := db.sql.QueryRow(
		`SELECT data FROM snapshots WHERE label = ?`, label,
	).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load snapshot %q: %w", label, err)
	}
	var s snapshot.Snapshot
	if err := json.Unmarshal(payload, &s); err != nil {
		return nil, fmt.Errorf("decode snapshot %q: %w", label, err)
	}
	return &s, nil
}

// ListSnapshots returns metadata for every stored snapshot, newest first.
func (db *DB) ListSnapshots() ([]SnapshotMeta, error) {
	rows, err := db.sql.Query(
		`SELECT id, label, created_at FROM snapshots ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	defer rows.Close()

	var out []SnapshotMeta
	for rows.Next() {
		var m SnapshotMeta
		if err := rows.Scan(&m.ID, &m.Label, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan snapshot meta: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteSnapshot removes a snapshot by label. Returns ErrNotFound if absent.
func (db *DB) DeleteSnapshot(label string) error {
	res, err := db.sql.Exec(`DELETE FROM snapshots WHERE label = ?`, label)
	if err != nil {
		return fmt.Errorf("delete snapshot %q: %w", label, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete snapshot %q: %w", label, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
