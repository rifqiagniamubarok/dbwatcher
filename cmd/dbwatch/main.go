package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/rifqiagniamubarok/dbwatcher/internal/config"
	"github.com/rifqiagniamubarok/dbwatcher/internal/listener"
	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
)

const version = "0.0.0-dev"

func main() {
	rootCmd := &cobra.Command{
		Use:   "dbwatch",
		Short: "tail -f for your Postgres database",
		Long:  "DBWatch streams INSERT, UPDATE, and DELETE events from Postgres to your terminal in realtime.",
	}

	rootCmd.AddCommand(tailCmd())
	rootCmd.AddCommand(versionCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func tailCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tail",
		Short: "Stream database changes to the terminal",
		RunE:  runTail,
	}

	cmd.Flags().String("db-url", "", "Postgres connection URL (or set DBWATCH_DB_URL)")
	cmd.Flags().String("publication", config.DefaultPublication, "Postgres publication name")
	cmd.Flags().String("slot", config.DefaultSlot, "Replication slot name")
	cmd.Flags().String("log-level", config.DefaultLogLevel, "Log level: debug, info, warn, error")
	cmd.Flags().Int("buffer", config.DefaultBufferSize, "Event ring buffer size")

	return cmd
}

func runTail(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cmd)
	if err != nil {
		return err
	}

	setupLogger(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st := store.New(cfg.BufferSize)

	listenerIn := make(chan store.Event, cfg.BufferSize)
	l := listener.New(cfg.DBURL, cfg.Publication, cfg.Slot, listenerIn)

	// Forward listener output into the Store.
	go func() {
		for e := range listenerIn {
			st.Push(e)
		}
	}()

	listenerErr := make(chan error, 1)
	go func() {
		listenerErr <- l.Start(ctx)
	}()

	// Print periodic stats to stderr.
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s := st.Stats()
				fmt.Fprintf(os.Stderr, "[stats] received=%d buffered=%d subscribers=1\n", s.Total, s.Buffered)
			case <-ctx.Done():
				return
			}
		}
	}()

	sub := st.Subscribe()
	defer st.Unsubscribe(sub)

	for {
		select {
		case e, ok := <-sub:
			if !ok {
				return nil
			}
			b, err := json.Marshal(e)
			if err != nil {
				slog.Error("marshal event", "err", err)
				continue
			}
			fmt.Println(string(b))

		case err := <-listenerErr:
			if err != nil {
				return fmt.Errorf("listener: %w", err)
			}
			return nil

		case <-ctx.Done():
			if err := <-listenerErr; err != nil && ctx.Err() == nil {
				return fmt.Errorf("listener: %w", err)
			}
			return nil
		}
	}
}

func setupLogger(level string) {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "info":
		l = slog.LevelInfo
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelWarn
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l})))
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version)
		},
	}
}
