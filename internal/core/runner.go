package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/rifqiagniamubarok/dbwatcher/internal/config"
	"github.com/rifqiagniamubarok/dbwatcher/internal/ddlwatcher"
	"github.com/rifqiagniamubarok/dbwatcher/internal/listener"
	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
)

// Run starts the listener and forwards decoded events into the provided store.
// It blocks until the listener exits or context is cancelled.
//
// onEvent (optional) is invoked synchronously on the forwarder goroutine after
// each successful Store.Push. Keep it fast and non-blocking — anything slow
// here back-pressures the entire Listener→Store pipeline. The tail command
// uses this to emit JSON to stdout; daemon mode passes nil and lets clients
// subscribe to the Store directly.
//
// When cfg.TrackDDL is set, a DDL listener runs on a separate connection in
// parallel. DDL setup failures (privilege, install) are surfaced as a warning
// and DML tracking continues — they never abort Run.
func Run(ctx context.Context, cfg *config.Config, st *store.Store, onEvent func(store.Event)) error {
	listenerIn := make(chan store.Event, cfg.BufferSize)
	l := listener.New(cfg.DBURL, cfg.Publication, cfg.Slot, listenerIn)

	forwardDone := make(chan struct{})
	go func() {
		defer close(forwardDone)
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-listenerIn:
				if !ok {
					return
				}
				st.Push(e)
				if onEvent != nil {
					onEvent(e)
				}
			}
		}
	}()

	if cfg.TrackDDL {
		startDDLListener(ctx, cfg, st, onEvent)
	}

	err := l.Start(ctx)
	// Listener does not close the output channel; close it here so the
	// forwarder can exit and Run can return listener errors promptly.
	close(listenerIn)
	<-forwardDone
	return err
}

// startDDLListener prepares the event trigger (per install mode) and launches
// the DDL listener in a background goroutine. Any setup failure is reported
// via the optional DDLWarn callback and swallowed — DDL tracking is opt-in
// and must never break DML tracking.
func startDDLListener(ctx context.Context, cfg *config.Config, st *store.Store, onEvent func(store.Event)) {
	if err := prepareDDL(ctx, cfg); err != nil {
		slog.Warn("ddl tracking disabled", "reason", err)
		if DDLWarn != nil {
			DDLWarn(ddlSetupHint(cfg, err))
		}
		return
	}

	dl := ddlwatcher.New(cfg.DBURL)
	go func() {
		err := dl.Start(ctx, func(e store.Event) {
			st.Push(e)
			if onEvent != nil {
				onEvent(e)
			}
		})
		if err != nil && ctx.Err() == nil {
			slog.Error("ddl listener stopped", "err", err)
		}
	}()
	slog.Info("ddl tracking enabled")
}

// prepareDDL connects with a regular connection and ensures the event trigger
// is present, honoring cfg.DDLInstallMode.
func prepareDDL(ctx context.Context, cfg *config.Config) error {
	if cfg.DDLInstallMode == config.DDLInstallNone {
		return nil // assume the trigger exists; just LISTEN
	}

	conn, err := pgx.Connect(ctx, stripReplication(cfg.DBURL))
	if err != nil {
		return fmt.Errorf("connect for ddl setup: %w", err)
	}
	defer conn.Close(context.Background())

	installed, err := ddlwatcher.IsInstalled(ctx, conn)
	if err != nil {
		return err
	}
	if installed {
		return nil
	}

	if cfg.DDLInstallMode == config.DDLInstallManual {
		return errors.New("event trigger not installed (install mode is manual)")
	}

	// auto mode: install it.
	if err := ddlwatcher.Install(ctx, conn); err != nil {
		return err
	}
	slog.Info("ddl event trigger installed")
	return nil
}
