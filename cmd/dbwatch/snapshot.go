package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"

	"github.com/rifqiagniamubarok/dbwatcher/internal/snapshot"
	"github.com/rifqiagniamubarok/dbwatcher/internal/storage"
)

const flagDataDir = "data-dir"

// addDataDirFlag registers the shared --data-dir flag on a snapshot subcommand.
func addDataDirFlag(cmd *cobra.Command) {
	cmd.Flags().String(flagDataDir, "", "Snapshot storage directory (default ~/.dbwatch)")
}

// snapshotCmd groups the snapshot / compare subcommands. This is the
// standalone CLI workflow — it does not need a daemon running.
func snapshotCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Capture and compare database snapshots (migration safety)",
		Long: "Capture the schema and per-table statistics of a database,\n" +
			"then compare two snapshots to review what a migration changed.\n\n" +
			"Snapshots are stored in a local SQLite file (~/.dbwatch/data.db).",
	}
	cmd.AddCommand(snapshotTakeCmd())
	cmd.AddCommand(snapshotListCmd())
	cmd.AddCommand(snapshotShowCmd())
	cmd.AddCommand(snapshotCompareCmd())
	cmd.AddCommand(snapshotDeleteCmd())
	return cmd
}

func snapshotTakeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "take",
		Short: "Capture a snapshot of the database now",
		RunE:  runSnapshotTake,
	}
	addDBURLFlag(cmd)
	cmd.Flags().String("label", "", "Snapshot label (auto-generated if empty)")
	addDataDirFlag(cmd)
	cmd.Flags().StringSlice("snapshot-tables", nil, "Only capture these tables")
	cmd.Flags().StringSlice("snapshot-exclude", nil, "Skip these tables")
	cmd.Flags().Duration("snapshot-timeout", 0, "Per-table statistics timeout (default 30s)")
	return cmd
}

func runSnapshotTake(cmd *cobra.Command, args []string) error {
	label, _ := cmd.Flags().GetString("label")
	include, _ := cmd.Flags().GetStringSlice("snapshot-tables")
	exclude, _ := cmd.Flags().GetStringSlice("snapshot-exclude")
	timeout, _ := cmd.Flags().GetDuration("snapshot-timeout")

	if len(include) > 0 && len(exclude) > 0 {
		return errors.New("--snapshot-tables and --snapshot-exclude are mutually exclusive")
	}

	db, err := openSnapshotDB(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	return withSnapshotConn(cmd, func(ctx context.Context, conn *pgx.Conn) error {
		capturer := snapshot.NewCapturer(conn, snapshot.Options{
			PerTableTimeout: timeout,
			IncludeTables:   include,
			ExcludeTables:   exclude,
		})

		start := time.Now()
		snap, err := capturer.Capture(ctx, label)
		if err != nil {
			return fmt.Errorf("capture snapshot: %w", err)
		}
		if err := db.SaveSnapshot(snap); err != nil {
			return err
		}
		fmt.Printf("✓ Snapshot %q captured (%d tables, %s)\n",
			snap.Label, len(snap.Schema.Tables), time.Since(start).Round(time.Millisecond))
		return nil
	})
}

func snapshotListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List stored snapshots",
		RunE:  runSnapshotList,
	}
	addDataDirFlag(cmd)
	return cmd
}

func runSnapshotList(cmd *cobra.Command, args []string) error {
	db, err := openSnapshotDB(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	metas, err := db.ListSnapshots()
	if err != nil {
		return err
	}
	if len(metas) == 0 {
		fmt.Println("No snapshots stored.")
		return nil
	}
	fmt.Printf("%-28s  %-12s  %s\n", "LABEL", "ID", "CAPTURED")
	for _, m := range metas {
		fmt.Printf("%-28s  %-12s  %s\n", m.Label, m.ID, m.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	return nil
}

func snapshotShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <label>",
		Short: "Show a stored snapshot's schema and statistics",
		Args:  cobra.ExactArgs(1),
		RunE:  runSnapshotShow,
	}
	addDataDirFlag(cmd)
	return cmd
}

func runSnapshotShow(cmd *cobra.Command, args []string) error {
	db, err := openSnapshotDB(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	snap, err := db.LoadSnapshot(args[0])
	if err != nil {
		return wrapSnapshotNotFound(args[0], err)
	}

	fmt.Printf("Snapshot %q (%s)\n", snap.Label, snap.CapturedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Tables: %d\n\n", len(snap.Schema.Tables))
	for name, ts := range snap.Schema.Tables {
		stat := snap.Statistics[name]
		fmt.Printf("  %s — %d columns, %d rows\n", name, len(ts.Columns), stat.RowCount)
		if stat.Skipped {
			fmt.Printf("    (statistics skipped: %s)\n", stat.SkippedReason)
		}
	}
	return nil
}

func snapshotCompareCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compare <from> [to]",
		Short: "Compare two snapshots (or a snapshot against the live database)",
		Long: "Compare two stored snapshots. If [to] is omitted, the <from>\n" +
			"snapshot is compared against the current state of the database\n" +
			"(requires --db-url).",
		Args: cobra.RangeArgs(1, 2),
		RunE: runSnapshotCompare,
	}
	addDBURLFlag(cmd)
	addDataDirFlag(cmd)
	cmd.Flags().String("format", snapshot.FormatText, "Output format: text, json, markdown")
	return cmd
}

func runSnapshotCompare(cmd *cobra.Command, args []string) error {
	format, _ := cmd.Flags().GetString("format")

	db, err := openSnapshotDB(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	from, err := db.LoadSnapshot(args[0])
	if err != nil {
		return wrapSnapshotNotFound(args[0], err)
	}

	var to *snapshot.Snapshot
	if len(args) == 2 {
		to, err = db.LoadSnapshot(args[1])
		if err != nil {
			return wrapSnapshotNotFound(args[1], err)
		}
	} else {
		// Compare against the live database.
		captureErr := withSnapshotConn(cmd, func(ctx context.Context, conn *pgx.Conn) error {
			capturer := snapshot.NewCapturer(conn, snapshot.Options{})
			to, err = capturer.Capture(ctx, "now")
			return err
		})
		if captureErr != nil {
			return fmt.Errorf("capture current state: %w", captureErr)
		}
	}

	report := snapshot.Compare(from, to)
	out, err := snapshot.FormatReport(report, format)
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

func snapshotDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <label>",
		Short: "Delete a stored snapshot",
		Args:  cobra.ExactArgs(1),
		RunE:  runSnapshotDelete,
	}
	addDataDirFlag(cmd)
	return cmd
}

func runSnapshotDelete(cmd *cobra.Command, args []string) error {
	db, err := openSnapshotDB(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.DeleteSnapshot(args[0]); err != nil {
		return wrapSnapshotNotFound(args[0], err)
	}
	fmt.Printf("✓ Snapshot %q deleted\n", args[0])
	return nil
}

// --- helpers ---

// resolveDataDir returns the snapshot storage directory: the --data-dir flag,
// else $DBWATCH_DATA_DIR, else ~/.dbwatch.
func resolveDataDir(cmd *cobra.Command) (string, error) {
	if dir, _ := cmd.Flags().GetString(flagDataDir); dir != "" {
		return dir, nil
	}
	if dir := os.Getenv("DBWATCH_DATA_DIR"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".dbwatch"), nil
}

func openSnapshotDB(cmd *cobra.Command) (*storage.DB, error) {
	dir, err := resolveDataDir(cmd)
	if err != nil {
		return nil, err
	}
	return storage.Open(dir)
}

// withSnapshotConn resolves the db-url and opens a regular pgx connection.
func withSnapshotConn(cmd *cobra.Command, fn func(ctx context.Context, conn *pgx.Conn) error) error {
	dbURL, _ := cmd.Flags().GetString("db-url")
	if dbURL == "" {
		dbURL = os.Getenv("DBWATCH_DB_URL")
	}
	if dbURL == "" {
		return errors.New("database URL is required\n\nSet it via --db-url flag or DBWATCH_DB_URL environment variable.")
	}
	ctx := cmd.Context()
	conn, err := pgx.Connect(ctx, ddlStripReplication(dbURL))
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer conn.Close(context.Background())
	return fn(ctx, conn)
}

func wrapSnapshotNotFound(label string, err error) error {
	if errors.Is(err, storage.ErrNotFound) {
		return fmt.Errorf("snapshot %q not found\n\nList snapshots with: dbwatch snapshot list", label)
	}
	return err
}
