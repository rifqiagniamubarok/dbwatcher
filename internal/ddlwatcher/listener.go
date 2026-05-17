package ddlwatcher

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
)

// Listener opens a regular (non-replication) connection, LISTENs on the
// dbwatch_ddl channel, and forwards parsed DDL events to a sink.
type Listener struct {
	dbURL string
}

// New creates a DDL Listener for the given database URL. The URL may include
// the replication=database parameter; it is stripped before connecting.
func New(dbURL string) *Listener {
	return &Listener{dbURL: stripReplicationParam(dbURL)}
}

// Start connects, LISTENs on the DDL channel, and calls push for every valid
// DDL notification until ctx is cancelled. A malformed notification is logged
// at Warn level and skipped — it never aborts the loop.
//
// push is invoked synchronously; keep it fast (it is store.Store.Push in
// practice, which is a quick mutex-guarded append + broadcast).
func (l *Listener) Start(ctx context.Context, push func(store.Event)) error {
	conn, err := pgx.Connect(ctx, l.dbURL)
	if err != nil {
		return fmt.Errorf("ddl listener connect: %w", err)
	}
	defer conn.Close(context.Background())

	// Verify the server supports event triggers before we LISTEN — gives a
	// clear error instead of silently receiving nothing.
	ok, ver, err := supportsEventTriggers(ctx, conn)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("ddl tracking requires Postgres 9.3+, server is %s", ver)
	}

	if _, err := conn.Exec(ctx, "LISTEN "+notifyChannel); err != nil {
		return fmt.Errorf("listen %s: %w", notifyChannel, err)
	}
	slog.Info("ddl listener started", "channel", notifyChannel)

	for {
		notification, err := conn.WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil // clean shutdown
			}
			return fmt.Errorf("ddl wait for notification: %w", err)
		}

		event, perr := parsePayload(notification.Payload)
		if perr != nil {
			slog.Warn("skipping malformed ddl notification",
				"err", perr, "payload", truncate(notification.Payload, 200))
			continue
		}
		slog.Debug("ddl event", "command", event.CommandTag, "object", event.ObjectIdentity)
		push(event)
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
