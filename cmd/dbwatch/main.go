package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/rifqiagniamubarok/dbwatcher/internal/config"
	"github.com/rifqiagniamubarok/dbwatcher/internal/core"
	"github.com/rifqiagniamubarok/dbwatcher/internal/daemon"
	"github.com/rifqiagniamubarok/dbwatcher/internal/ipc"
	"github.com/rifqiagniamubarok/dbwatcher/internal/markerapi"
	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
	"github.com/rifqiagniamubarok/dbwatcher/internal/tui"
)

// Set via -ldflags at build time.
var (
	version   = "0.0.0-dev"
	commit    = "none"
	buildDate = "unknown"
)

const (
	flagName        = "name"
	flagDaemonChild = "daemon-child"
	flagMarkerPort  = "marker-port"
	flagMarkerBind  = "marker-bind"
	flagNoMarker    = "no-marker"
	daemonNameDesc  = "Daemon name"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "dbwatch",
		Short: "tail -f for your Postgres database",
		Long:  "DBWatch streams INSERT, UPDATE, and DELETE events from Postgres to your terminal in realtime.",
	}

	rootCmd.AddCommand(tailCmd())
	rootCmd.AddCommand(attachCmd())
	rootCmd.AddCommand(daemonCmd())
	rootCmd.AddCommand(ddlToolsCmd())
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
	addMarkerFlags(cmd)
	addDDLFlags(cmd)

	return cmd
}

// addMarkerFlags registers the marker HTTP API flags on cmd.
// Shared by `tail` and `daemon start`.
func addMarkerFlags(cmd *cobra.Command) {
	cmd.Flags().Int(flagMarkerPort, markerapi.DefaultPort, "Marker HTTP API port (set 0 for ephemeral)")
	cmd.Flags().String(flagMarkerBind, markerapi.DefaultBind, "Marker HTTP API bind address")
	cmd.Flags().Bool(flagNoMarker, false, "Disable the marker HTTP API")
}

// addDDLFlags registers the DDL-tracking flags on cmd.
// Shared by `tail` and `daemon start`.
func addDDLFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("track-ddl", false, "Track schema changes (DDL) — requires superuser to install the event trigger")
	cmd.Flags().String("ddl-install-mode", config.DDLInstallAuto, "DDL trigger install: auto, manual, none")
}

// startMarkerAPI starts the marker HTTP server in a goroutine, unless
// --no-marker is set. It returns a channel that emits the server's exit
// error (so the caller can react to a port conflict) and a boolean
// indicating whether the server was actually started.
func startMarkerAPI(ctx context.Context, cmd *cobra.Command, st *store.Store, startedAt time.Time) (<-chan error, bool) {
	noMarker, _ := cmd.Flags().GetBool(flagNoMarker)
	if noMarker {
		return nil, false
	}
	port, _ := cmd.Flags().GetInt(flagMarkerPort)
	bind, _ := cmd.Flags().GetString(flagMarkerBind)

	server := markerapi.New(markerapi.Options{
		Bind:    bind,
		Port:    port,
		Store:   st,
		StartAt: startedAt,
		Version: version,
	})
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe(ctx)
	}()
	return errCh, true
}

// setupDDLWarn routes core's DDL-setup warning into the Store as a log entry,
// so it surfaces uniformly in the TUI, attach, and JSON modes without
// corrupting the Bubble Tea alt-screen. The full multi-line hint also goes to
// stderr when not in TUI mode (visible before the program exits / when piped).
func setupDDLWarn(st *store.Store, useTUI bool) {
	core.DDLWarn = func(message string) {
		st.Push(store.NewLog("DDL tracking disabled — see logs for details"))
		if !useTUI {
			fmt.Fprintln(os.Stderr, message)
		} else {
			slog.Warn("ddl tracking disabled", "detail", message)
		}
	}
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
	var lastLSN atomic.Value
	setupDDLWarn(st, useTUI)

	sourceErr := make(chan error, 1)
	go func() {
		sourceErr <- core.Run(ctx, cfg, st, func(e store.Event) {
			lastLSN.Store(e.LSN)
		})
	}()

	markerErr, _ := startMarkerAPI(ctx, cmd, st, time.Now())
	go func() {
		if markerErr == nil {
			return
		}
		if err := <-markerErr; err != nil {
			fmt.Fprintf(os.Stderr, "marker api: %v\n", err)
		}
	}()

	if useTUI {
		return runTUIMode(ctx, stop, cfg.DBURL, st, sourceErr, "listener")
	}
	return runJSONMode(ctx, st, sourceErr, "listener")
}

func runTUIMode(ctx context.Context, stop func(), dbTarget string, st *store.Store, sourceErr <-chan error, sourceLabel string) error {
	model := tui.NewWithEvents(dbTarget, st.Snapshot())
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
		case err := <-sourceErr:
			if err != nil {
				p.Send(tea.QuitMsg{})
				if sourceLabel == "daemon" {
					fmt.Fprintf(os.Stderr, "\nConnection to daemon lost: %v\n", err)
				} else {
					fmt.Fprintf(os.Stderr, "\n%s error: %v\n", sourceLabel, err)
				}
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

func runJSONMode(ctx context.Context, st *store.Store, sourceErr <-chan error, sourceLabel string) error {
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

	// Print buffered history first so daemon attach is useful before new events arrive.
	for _, e := range st.Snapshot() {
		b, err := json.Marshal(e)
		if err != nil {
			slog.Error("marshal event", "err", err)
			continue
		}
		fmt.Println(string(b))
	}

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

		case err := <-sourceErr:
			return wrapSourceError(sourceLabel, err)

		case <-ctx.Done():
			return waitSourceOnShutdown(sourceErr, sourceLabel)
		}
	}
}

func waitSourceOnShutdown(sourceErr <-chan error, sourceLabel string) error {
	err := <-sourceErr
	return wrapSourceError(sourceLabel, err)
}

func wrapSourceError(sourceLabel string, err error) error {
	if err == nil {
		return nil
	}
	if sourceLabel == "daemon" {
		return fmt.Errorf("connection to daemon lost: %w", err)
	}
	return fmt.Errorf("%s: %w", sourceLabel, err)
}

func attachCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attach",
		Short: "Attach to a running dbwatch daemon",
		RunE:  runAttach,
	}

	cmd.Flags().String(flagName, "default", daemonNameDesc)
	cmd.Flags().String("output", "auto", "Output mode: auto, tui, json")
	cmd.Flags().Int("buffer", config.DefaultBufferSize, "Local attach buffer size")

	return cmd
}

func runAttach(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString(flagName)
	outputMode, _ := cmd.Flags().GetString("output")
	bufferSize, _ := cmd.Flags().GetInt("buffer")

	socketPath, err := ipc.ResolveSocketPath(name)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, err := ipc.Dial(ctx, socketPath)
	if err != nil {
		return fmt.Errorf("attach to daemon %q: %w", name, err)
	}
	defer client.Close()

	st := store.New(bufferSize)
	for _, e := range client.Snapshot() {
		st.Push(e)
	}

	sourceErr := make(chan error, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				sourceErr <- nil
				return
			case e, ok := <-client.Events():
				if !ok {
					sourceErr <- io.EOF
					return
				}
				st.Push(e)
			case err, ok := <-client.Errors():
				if !ok {
					sourceErr <- io.EOF
					return
				}
				sourceErr <- err
				return
			}
		}
	}()

	dbTarget := client.Hello().DB
	if dbTarget == "" {
		dbTarget = "daemon:" + name
	}

	if shouldUseTUI(outputMode) {
		return runTUIMode(ctx, stop, dbTarget, st, sourceErr, "daemon")
	}
	return runJSONMode(ctx, st, sourceErr, "daemon")
}

func daemonCmd() *cobra.Command {
	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage dbwatch daemon processes",
	}

	daemonCmd.AddCommand(daemonStartCmd())
	daemonCmd.AddCommand(daemonStopCmd())
	daemonCmd.AddCommand(daemonStatusCmd())
	daemonCmd.AddCommand(daemonListCmd())
	daemonCmd.AddCommand(daemonLogsCmd())

	return daemonCmd
}

func daemonStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start daemon mode",
		RunE:  runDaemonStart,
	}

	cmd.Flags().String("db-url", "", "Postgres connection URL (or set DBWATCH_DB_URL)")
	cmd.Flags().String("publication", config.DefaultPublication, "Postgres publication name")
	cmd.Flags().String("slot", config.DefaultSlot, "Replication slot name")
	cmd.Flags().String("log-level", config.DefaultLogLevel, "Log level: debug, info, warn, error")
	cmd.Flags().Int("buffer", config.DefaultBufferSize, "Event ring buffer size")
	cmd.Flags().String(flagName, "default", daemonNameDesc)
	cmd.Flags().Bool("detach", false, "Run daemon in background")
	cmd.Flags().Bool(flagDaemonChild, false, "Internal: daemon detached child process")
	_ = cmd.Flags().MarkHidden(flagDaemonChild)
	addMarkerFlags(cmd)
	addDDLFlags(cmd)

	return cmd
}

func runDaemonStart(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cmd)
	if err != nil {
		return err
	}

	name, _ := cmd.Flags().GetString(flagName)
	detach, _ := cmd.Flags().GetBool("detach")
	isChild, _ := cmd.Flags().GetBool(flagDaemonChild)

	socketPath, pidPath, logPath, err := resolveDaemonPaths(name)
	if err != nil {
		return err
	}

	if err := prepareDaemonStart(pidPath, socketPath, name); err != nil {
		return err
	}

	if detach && !isChild {
		return runDaemonDetached(cmd, name, cfg, pidPath, socketPath, logPath)
	}

	return runDaemonForeground(cmd, cfg, name, pidPath, socketPath, isChild)
}

func resolveDaemonPaths(name string) (socketPath, pidPath, logPath string, err error) {
	socketPath, err = ipc.ResolveSocketPath(name)
	if err != nil {
		return "", "", "", err
	}
	pidPath, err = ipc.ResolvePIDPath(name)
	if err != nil {
		return "", "", "", err
	}
	logPath, err = ipc.ResolveLogPath(name)
	if err != nil {
		return "", "", "", err
	}
	return socketPath, pidPath, logPath, nil
}

func prepareDaemonStart(pidPath, socketPath, name string) error {
	if existingPID, err := daemon.ReadPIDFile(pidPath); err == nil && daemon.IsProcessRunning(existingPID) {
		return fmt.Errorf("daemon %q is already running (pid %d)", name, existingPID)
	}

	_ = daemon.RemoveFileIfExists(pidPath)
	if fi, err := os.Stat(socketPath); err == nil && fi.Mode()&os.ModeSocket != 0 {
		if removeErr := os.Remove(socketPath); removeErr != nil {
			return fmt.Errorf("remove stale socket %q: %w", socketPath, removeErr)
		}
	}
	return nil
}

func runDaemonDetached(cmd *cobra.Command, name string, cfg *config.Config, pidPath, socketPath, logPath string) error {
	if err := daemon.TruncateIfLarge(logPath, 10*1024*1024); err != nil {
		return fmt.Errorf("prepare daemon log file: %w", err)
	}
	childPID, err := startDetachedDaemon(cmd, name, cfg, logPath)
	if err != nil {
		return err
	}

	if err := waitForDaemonReady(pidPath, socketPath, 4*time.Second); err != nil {
		return err
	}
	fmt.Printf("daemon %q started (pid %d, socket %s)\n", name, childPID, socketPath)
	return nil
}

func runDaemonForeground(cmd *cobra.Command, cfg *config.Config, name, pidPath, socketPath string, isChild bool) error {
	setupLogger(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := daemon.WritePIDFile(pidPath, os.Getpid()); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}
	defer daemon.RemoveFileIfExists(pidPath)

	st := store.New(cfg.BufferSize)
	var lastLSN atomic.Value
	startedAt := time.Now()
	setupDDLWarn(st, false) // daemon has no TUI; warning goes to the log file

	coreErr := make(chan error, 1)
	go func() {
		coreErr <- core.Run(ctx, cfg, st, func(e store.Event) {
			lastLSN.Store(e.LSN)
		})
	}()

	server := ipc.NewServer(ipc.ServerOptions{
		SocketPath: socketPath,
		Version:    version,
		DB:         cfg.DBURL,
		Capacity:   cfg.BufferSize,
		Snapshot:   st.Snapshot,
		Subscribe:  st.SubscribeWithFilter,
		Unsubscribe: func(ch <-chan store.Event) {
			st.Unsubscribe(ch)
		},
		Stats: func(clients int) ipc.StatsData {
			stats := st.Stats()
			lsn, _ := lastLSN.Load().(string)
			ratio := 0.0
			if cfg.BufferSize > 0 {
				ratio = float64(stats.Buffered) / float64(cfg.BufferSize)
			}
			return ipc.StatsData{
				UptimeSeconds: int64(time.Since(startedAt).Seconds()),
				Received:      stats.Total,
				Clients:       clients,
				LastLSN:       lsn,
				Buffered:      stats.Buffered,
				Capacity:      cfg.BufferSize,
				BufferRatio:   ratio,
			}
		},
	})

	ipcErr := make(chan error, 1)
	go func() {
		ipcErr <- server.ListenAndServe(ctx)
	}()

	markerErr, markerOn := startMarkerAPI(ctx, cmd, st, startedAt)

	if !isChild {
		fmt.Printf("daemon %q started (pid %d, socket %s)\n", name, os.Getpid(), socketPath)
		if markerOn {
			markerPort, _ := cmd.Flags().GetInt(flagMarkerPort)
			markerBind, _ := cmd.Flags().GetString(flagMarkerBind)
			fmt.Printf("marker api listening on %s:%d\n", markerBind, markerPort)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-coreErr:
			return wrapNamedError("listener", err)
		case err := <-ipcErr:
			return wrapNamedError("ipc server", err)
		case err := <-markerErr:
			if err != nil {
				return wrapNamedError("marker api", err)
			}
		}
	}
}

func wrapNamedError(label string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", label, err)
}

func daemonStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop daemon",
		RunE:  runDaemonStop,
	}
	cmd.Flags().String(flagName, "default", daemonNameDesc)
	return cmd
}

func runDaemonStop(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString(flagName)
	pidPath, err := ipc.ResolvePIDPath(name)
	if err != nil {
		return err
	}
	socketPath, err := ipc.ResolveSocketPath(name)
	if err != nil {
		return err
	}

	pid, err := daemon.ReadPIDFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("daemon %q is not running\n", name)
			_ = daemon.RemoveFileIfExists(socketPath)
			return nil
		}
		return fmt.Errorf("read pid file: %w", err)
	}

	if err := daemon.StopProcess(pid, 5*time.Second); err != nil {
		return err
	}
	_ = daemon.RemoveFileIfExists(pidPath)
	_ = daemon.RemoveFileIfExists(socketPath)
	fmt.Printf("daemon %q stopped\n", name)
	return nil
}

func daemonStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show daemon status",
		RunE:  runDaemonStatus,
	}
	cmd.Flags().String(flagName, "default", daemonNameDesc)
	return cmd
}

func runDaemonStatus(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString(flagName)
	status, err := daemonStatus(name)
	if err != nil {
		return err
	}
	fmt.Println(status)
	return nil
}

func daemonListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List daemon instances",
		RunE:  runDaemonList,
	}
}

func runDaemonList(cmd *cobra.Command, args []string) error {
	runtimeDir, err := ipc.ResolveRuntimeDir()
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(runtimeDir)
	if err != nil {
		return fmt.Errorf("read runtime dir: %w", err)
	}

	names := make([]string, 0)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".pid") {
			names = append(names, strings.TrimSuffix(e.Name(), ".pid"))
		}
	}

	if len(names) == 0 {
		fmt.Println("no daemons found")
		return nil
	}

	fmt.Println("NAME      STATUS    PID    UPTIME   EVENTS   CLIENTS")
	for _, name := range names {
		line, err := daemonStatus(name)
		if err != nil {
			fmt.Printf("%-9s error     -      -        -        -\n", name)
			continue
		}
		fmt.Printf("%-9s %s\n", name, line)
	}
	return nil
}

func daemonLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show daemon log file",
		RunE:  runDaemonLogs,
	}
	cmd.Flags().String(flagName, "default", daemonNameDesc)
	cmd.Flags().Bool("follow", false, "Follow log output")
	return cmd
}

func runDaemonLogs(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString(flagName)
	follow, _ := cmd.Flags().GetBool("follow")

	logPath, err := ipc.ResolveLogPath(name)
	if err != nil {
		return err
	}

	f, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("open daemon log: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(os.Stdout, f); err != nil {
		return fmt.Errorf("print daemon log: %w", err)
	}
	if !follow {
		return nil
	}

	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				time.Sleep(250 * time.Millisecond)
				continue
			}
			return err
		}
		fmt.Print(line)
	}
}

func daemonStatus(name string) (string, error) {
	pidPath, err := ipc.ResolvePIDPath(name)
	if err != nil {
		return "", err
	}
	socketPath, err := ipc.ResolveSocketPath(name)
	if err != nil {
		return "", err
	}

	pid, err := daemon.ReadPIDFile(pidPath)
	if err != nil || !daemon.IsProcessRunning(pid) {
		return "not running", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client, err := ipc.Dial(ctx, socketPath)
	if err != nil {
		return fmt.Sprintf("running (pid %d, socket unreachable)", pid), nil
	}
	defer client.Close()

	stats, err := client.RequestStats(ctx)
	if err != nil {
		return fmt.Sprintf("running (pid %d, stats unavailable)", pid), nil
	}

	return fmt.Sprintf("running   %d   %s   %d   %d", pid, formatUptime(stats.UptimeSeconds), stats.Received, stats.Clients), nil
}

func startDetachedDaemon(cmd *cobra.Command, name string, cfg *config.Config, logPath string) (int, error) {
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, fmt.Errorf("open daemon log file: %w", err)
	}

	args := []string{
		"daemon", "start",
		"--" + flagDaemonChild,
		"--" + flagName, name,
		"--db-url", cfg.DBURL,
		"--publication", cfg.Publication,
		"--slot", cfg.Slot,
		"--log-level", cfg.LogLevel,
		"--buffer", strconv.Itoa(cfg.BufferSize),
	}

	// Forward marker flags so the detached child binds the same port/bind
	// and respects --no-marker.
	if noMarker, _ := cmd.Flags().GetBool(flagNoMarker); noMarker {
		args = append(args, "--"+flagNoMarker)
	} else {
		port, _ := cmd.Flags().GetInt(flagMarkerPort)
		bind, _ := cmd.Flags().GetString(flagMarkerBind)
		args = append(args,
			"--"+flagMarkerPort, strconv.Itoa(port),
			"--"+flagMarkerBind, bind,
		)
	}

	// Forward DDL-tracking flags so the detached child tracks DDL too.
	if cfg.TrackDDL {
		args = append(args,
			"--track-ddl",
			"--ddl-install-mode", cfg.DDLInstallMode,
		)
	}

	child := exec.Command(os.Args[0], args...)
	child.Stdout = logFile
	child.Stderr = logFile
	child.Stdin = nil
	child.Dir = currentWorkingDir()
	if err := applyDetachAttrs(child); err != nil {
		_ = logFile.Close()
		return 0, err
	}

	if err := child.Start(); err != nil {
		_ = logFile.Close()
		return 0, fmt.Errorf("start detached daemon: %w", err)
	}
	_ = logFile.Close()
	return child.Process.Pid, nil
}

func waitForDaemonReady(pidPath, socketPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(pidPath); err == nil {
			if _, err := os.Stat(socketPath); err == nil {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not become ready in %s", timeout)
}

func currentWorkingDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return filepath.Dir(os.Args[0])
	}
	return wd
}

func formatUptime(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	d := time.Duration(seconds) * time.Second
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
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
