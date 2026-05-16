// Package ddlwatcher tracks Postgres schema changes (DDL) by installing
// event triggers that pg_notify a dedicated channel, then LISTENing on a
// regular connection. It is independent of the logical-replication
// Listener that handles DML.
package ddlwatcher

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ErrInsufficientPrivilege is returned when the connected user cannot create
// event triggers (requires superuser).
var ErrInsufficientPrivilege = errors.New("ddl tracking requires superuser / EVENT TRIGGER privilege")

// CheckPrivilege reports whether the current user can install event triggers.
// Event triggers require superuser in every supported Postgres version, so we
// check rolsuper for the session user.
func CheckPrivilege(ctx context.Context, conn *pgx.Conn) error {
	var isSuper bool
	err := conn.QueryRow(ctx,
		`SELECT rolsuper FROM pg_roles WHERE rolname = current_user`,
	).Scan(&isSuper)
	if err != nil {
		return fmt.Errorf("check superuser privilege: %w", err)
	}
	if !isSuper {
		return ErrInsufficientPrivilege
	}
	return nil
}

// IsInstalled reports whether the dbwatch event triggers exist.
func IsInstalled(ctx context.Context, conn *pgx.Conn) (bool, error) {
	var count int
	err := conn.QueryRow(ctx,
		`SELECT count(*) FROM pg_event_trigger WHERE evtname = ANY($1)`,
		[]string{endTriggerName, dropTriggerName},
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("query event triggers: %w", err)
	}
	// Both triggers must be present to count as installed.
	return count == 2, nil
}

// Install creates the capture functions and event triggers. It is idempotent:
// functions use CREATE OR REPLACE and triggers are dropped before being
// recreated, so running Install twice is safe.
func Install(ctx context.Context, conn *pgx.Conn) error {
	if err := CheckPrivilege(ctx, conn); err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, installSQL); err != nil {
		return fmt.Errorf("install ddl capture functions: %w", err)
	}
	if _, err := conn.Exec(ctx, dropTriggersSQL); err != nil {
		return fmt.Errorf("drop existing ddl triggers: %w", err)
	}
	if _, err := conn.Exec(ctx, createTriggersSQL); err != nil {
		return fmt.Errorf("create ddl event triggers: %w", err)
	}
	return nil
}

// Uninstall removes the dbwatch event triggers and capture functions.
// Safe to call when nothing is installed.
func Uninstall(ctx context.Context, conn *pgx.Conn) error {
	if _, err := conn.Exec(ctx, uninstallSQL); err != nil {
		return fmt.Errorf("uninstall ddl tracking: %w", err)
	}
	return nil
}

// supportsEventTriggers reports whether the server is new enough for event
// triggers (Postgres 9.3+). Practically always true, but checked for a clear
// error rather than a confusing SQL failure.
//
// SHOW returns its value as text, so server_version_num is scanned into a
// string and parsed. current_setting() would also work but SHOW keeps it
// consistent with how the rest of the codebase probes settings.
func supportsEventTriggers(ctx context.Context, conn *pgx.Conn) (bool, string, error) {
	var versionNumStr, versionStr string
	if err := conn.QueryRow(ctx, "SHOW server_version_num").Scan(&versionNumStr); err != nil {
		return false, "", fmt.Errorf("read server version: %w", err)
	}
	_ = conn.QueryRow(ctx, "SHOW server_version").Scan(&versionStr)

	version, err := strconv.Atoi(strings.TrimSpace(versionNumStr))
	if err != nil {
		return false, versionStr, fmt.Errorf("parse server version %q: %w", versionNumStr, err)
	}
	return version >= 90300, versionStr, nil
}
