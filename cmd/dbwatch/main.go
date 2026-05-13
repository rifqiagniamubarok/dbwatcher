package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/rifqiagniamubarok/dbwatcher/internal/config"
	"github.com/rifqiagniamubarok/dbwatcher/internal/listener"
	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
	"github.com/rifqiagniamubarok/dbwatcher/internal/tui"
)

// Set via -ldflags at build time.
var (
	version   = "0.0.0-dev"
	commit    = "none"
	buildDate = "unknown"
)

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
	cmd.Flags().String("output", "auto", "Output mode: auto, tui, json")

	return cmd
}

func runTail(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cmd)
	if err != nil {
		return err
	}

	setupLogger(cfg.LogLevel)

	outputMode, _ := cmd.Flags().GetString("output")
	useTUI := shouldUseTUI(outputMode)

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st := store.New(cfg.BufferSize)

	listenerIn := make(chan store.Event, cfg.BufferSize)
	l := listener.New(cfg.DBURL, cfg.Publication, cfg.Slot, listenerIn)

	go func() {
		for e := range listenerIn {
			st.Push(e)
		}
	}()

	listenerErr := make(chan error, 1)
	go func() {
		listenerErr <- l.Start(ctx)
	}()

	if useTUI {
		return runTUIMode(ctx, stop, cfg, st, listenerErr)
	}
	return runJSONMode(ctx, st, listenerErr)
}

func runTUIMode(ctx interface{ Done() <-chan struct{} }, stop func(), cfg *config.Config, st *store.Store, listenerErr <-chan error) error {
	model := tui.New(cfg.DBURL)
	p := tea.NewProgram(model, tea.WithAltScreen())

	sub := st.Subscribe()

	// Forward store events into the Bubble Tea program.
	go func() {
		for {
			select {
			case e, ok := <-sub:
				if !ok {
					return
				}
				p.Send(tui.EventMsg(e))
			case <-ctx.Done():
				return
			}
		}
	}()

	// Quit program when listener errors out.
	go func() {
		select {
		case err := <-listenerErr:
			if err != nil {
				p.Send(tea.QuitMsg{})
				fmt.Fprintf(os.Stderr, "\nlistener error: %v\n", err)
			}
		case <-ctx.Done():
			p.Send(tea.QuitMsg{})
		}
	}()

	_, err := p.Run()
	st.Unsubscribe(sub)
	stop()
	return err
}

func runJSONMode(ctx interface{ Done() <-chan struct{} }, st *store.Store, listenerErr <-chan error) error {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s := st.Stats()
				fmt.Fprintf(os.Stderr, "[stats] received=%d buffered=%d\n", s.Total, s.Buffered)
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
			if err := <-listenerErr; err != nil {
				return fmt.Errorf("listener: %w", err)
			}
			return nil
		}
	}
}

func shouldUseTUI(mode string) bool {
	switch mode {
	case "tui":
		return true
	case "json":
		return false
	default: // "auto"
		return isatty.IsTerminal(os.Stdout.Fd())
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
			fmt.Printf("dbwatch %s (commit %s, built %s)\n", version, commit, buildDate)
		},
	}
}
