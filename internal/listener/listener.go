package listener

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"

	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
)

const (
	outputPlugin    = "pgoutput"
	standbyTimeout  = 10 * time.Second
	ackInterval     = 5 * time.Second
)

// Listener connects to Postgres via logical replication and pushes decoded
// events to the provided channel.
type Listener struct {
	dbURL       string
	publication string
	slot        string
	out         chan<- store.Event
}

// New creates a Listener. out is the channel where decoded events are sent.
func New(dbURL, publication, slot string, out chan<- store.Event) *Listener {
	return &Listener{
		dbURL:       dbURL,
		publication: publication,
		slot:        slot,
		out:         out,
	}
}

// Start begins streaming. Blocks until ctx is cancelled or a fatal error occurs.
func (l *Listener) Start(ctx context.Context) error {
	conn, err := pgconn.Connect(ctx, l.dbURL)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer conn.Close(context.Background())

	slog.Info("connected to postgres")

	// Ensure publication exists.
	if err := l.ensurePublication(ctx, conn); err != nil {
		return err
	}

	// Create a temporary replication slot (auto-dropped on disconnect).
	slotLSN, err := l.createSlot(ctx, conn)
	if err != nil {
		return err
	}

	slog.Info("replication slot ready", "slot", l.slot, "lsn", slotLSN)

	// Start replication.
	opts := pglogrepl.StartReplicationOptions{
		PluginArgs: []string{
			"proto_version '1'",
			fmt.Sprintf("publication_names '%s'", l.publication),
		},
	}
	if err := pglogrepl.StartReplication(ctx, conn, l.slot, slotLSN, opts); err != nil {
		return fmt.Errorf("start replication: %w", err)
	}

	slog.Info("replication started", "lsn", slotLSN)

	cache := NewSchemaCache()
	return l.readLoop(ctx, conn, cache, slotLSN)
}

func (l *Listener) ensurePublication(ctx context.Context, conn *pgconn.PgConn) error {
	checkSQL := fmt.Sprintf(
		"SELECT 1 FROM pg_publication WHERE pubname = '%s'", l.publication,
	)
	res := conn.Exec(ctx, checkSQL)
	rows, err := res.ReadAll()
	if err != nil {
		return fmt.Errorf("check publication: %w", err)
	}
	if len(rows) > 0 && len(rows[0].Rows) > 0 {
		slog.Debug("publication exists", "publication", l.publication)
		return nil
	}

	createSQL := fmt.Sprintf(
		"CREATE PUBLICATION %s FOR ALL TABLES", l.publication,
	)
	res2 := conn.Exec(ctx, createSQL)
	if _, err := res2.ReadAll(); err != nil {
		return fmt.Errorf("create publication %q: %w", l.publication, err)
	}
	slog.Info("created publication", "publication", l.publication)
	return nil
}

func (l *Listener) createSlot(ctx context.Context, conn *pgconn.PgConn) (pglogrepl.LSN, error) {
	result, err := pglogrepl.CreateReplicationSlot(
		ctx, conn, l.slot, outputPlugin,
		pglogrepl.CreateReplicationSlotOptions{Temporary: true},
	)
	if err != nil {
		return 0, fmt.Errorf("create replication slot: %w", err)
	}
	lsn, err := pglogrepl.ParseLSN(result.ConsistentPoint)
	if err != nil {
		return 0, fmt.Errorf("parse lsn: %w", err)
	}
	return lsn, nil
}

func (l *Listener) readLoop(
	ctx context.Context,
	conn *pgconn.PgConn,
	cache *SchemaCache,
	startLSN pglogrepl.LSN,
) error {
	clientXLogPos := startLSN
	nextAck := time.Now().Add(ackInterval)

	for {
		if time.Now().After(nextAck) {
			if err := sendStandbyStatus(ctx, conn, clientXLogPos); err != nil {
				return err
			}
			nextAck = time.Now().Add(ackInterval)
		}

		recvCtx, cancel := context.WithDeadline(ctx, time.Now().Add(standbyTimeout))
		rawMsg, err := conn.ReceiveMessage(recvCtx)
		cancel()

		if err != nil {
			if pgconn.Timeout(err) {
				// No message within standby timeout — send keepalive and continue.
				if sendErr := sendStandbyStatus(ctx, conn, clientXLogPos); sendErr != nil {
					return sendErr
				}
				nextAck = time.Now().Add(ackInterval)
				continue
			}
			if ctx.Err() != nil {
				return nil // clean shutdown
			}
			return fmt.Errorf("receive message: %w", err)
		}

		switch msg := rawMsg.(type) {
		case *pgproto3.CopyData:
			if msg.Data[0] == pglogrepl.PrimaryKeepaliveMessageByteID {
				pkm, err := pglogrepl.ParsePrimaryKeepaliveMessage(msg.Data[1:])
				if err != nil {
					return fmt.Errorf("parse keepalive: %w", err)
				}
				if pkm.ReplyRequested {
					if err := sendStandbyStatus(ctx, conn, clientXLogPos); err != nil {
						return err
					}
					nextAck = time.Now().Add(ackInterval)
				}
				continue
			}
			if msg.Data[0] != pglogrepl.XLogDataByteID {
				continue
			}

			xld, err := pglogrepl.ParseXLogData(msg.Data[1:])
			if err != nil {
				return fmt.Errorf("parse xlog data: %w", err)
			}

			logicalMsg, err := pglogrepl.Parse(xld.WALData)
			if err != nil {
				slog.Warn("parse wal message failed", "err", err)
				continue
			}

			// Update schema cache on RelationMessage.
			if rel, ok := logicalMsg.(*pglogrepl.RelationMessage); ok {
				cache.Update(rel)
			}

			event, err := DecodeMessage(logicalMsg, cache)
			if err != nil {
				slog.Warn("decode message failed", "err", err)
				continue
			}

			if event != nil {
				event.LSN = xld.WALStart.String()
				select {
				case l.out <- *event:
				default:
					slog.Warn("event channel full, dropping event", "table", event.Table)
				}
			}

			if xld.WALStart > clientXLogPos {
				clientXLogPos = xld.WALStart
			}

		case *pgproto3.ErrorResponse:
			return fmt.Errorf("postgres error: %s", msg.Message)
		}
	}
}

func sendStandbyStatus(ctx context.Context, conn *pgconn.PgConn, lsn pglogrepl.LSN) error {
	err := pglogrepl.SendStandbyStatusUpdate(ctx, conn, pglogrepl.StandbyStatusUpdate{
		WALWritePosition: lsn,
	})
	if err != nil {
		return fmt.Errorf("send standby status: %w", err)
	}
	slog.Debug("sent standby status", "lsn", lsn)
	return nil
}
