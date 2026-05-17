package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"

	"github.com/rifqiagniamubarok/dbwatcher/internal/ddlwatcher"
)

// ddlToolsCmd groups helper subcommands for installing / inspecting the DDL
// event trigger. Useful for the split-privilege workflow where a DBA installs
// the trigger once and developers run dbwatch with a non-superuser account.
func ddlToolsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ddl-tools",
		Short: "Install / inspect the DDL event trigger",
		Long: "Helper subcommands for DDL tracking.\n\n" +
			"Installing the event trigger requires superuser. A DBA can run\n" +
			"`ddl-tools install` once, after which developers use\n" +
			"`dbwatch tail --track-ddl --ddl-install-mode=none`.",
	}
	cmd.AddCommand(ddlPrintSQLCmd())
	cmd.AddCommand(ddlInstallCmd())
	cmd.AddCommand(ddlUninstallCmd())
	cmd.AddCommand(ddlStatusCmd())
	return cmd
}

func ddlPrintSQLCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "print-sql",
		Short: "Print the SQL that installs the DDL event trigger",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Print(ddlwatcher.PrintSQL())
			return nil
		},
	}
}

func ddlInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the DDL event trigger (requires superuser)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withDDLConn(cmd, func(ctx context.Context, conn *pgx.Conn) error {
				if err := ddlwatcher.Install(ctx, conn); err != nil {
					if errors.Is(err, ddlwatcher.ErrInsufficientPrivilege) {
						return fmt.Errorf("%w\n\nConnect with a superuser account, or ask your DBA to run:\n  dbwatch ddl-tools print-sql | psql <superuser-url>", err)
					}
					return err
				}
				fmt.Println("✓ DDL event trigger installed")
				return nil
			})
		},
	}
	addDBURLFlag(cmd)
	return cmd
}

func ddlUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the DDL event trigger",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withDDLConn(cmd, func(ctx context.Context, conn *pgx.Conn) error {
				if err := ddlwatcher.Uninstall(ctx, conn); err != nil {
					return err
				}
				fmt.Println("✓ DDL event trigger removed")
				return nil
			})
		},
	}
	addDBURLFlag(cmd)
	return cmd
}

func ddlStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Report whether the DDL event trigger is installed",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withDDLConn(cmd, func(ctx context.Context, conn *pgx.Conn) error {
				installed, err := ddlwatcher.IsInstalled(ctx, conn)
				if err != nil {
					return err
				}
				if installed {
					fmt.Println("installed")
				} else {
					fmt.Println("not installed")
				}
				return nil
			})
		},
	}
	addDBURLFlag(cmd)
	return cmd
}

func addDBURLFlag(cmd *cobra.Command) {
	cmd.Flags().String("db-url", "", "Postgres connection URL (or set DBWATCH_DB_URL)")
}

// withDDLConn resolves the db-url, opens a regular (non-replication) pgx
// connection, runs fn, and closes the connection.
func withDDLConn(cmd *cobra.Command, fn func(ctx context.Context, conn *pgx.Conn) error) error {
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

// ddlStripReplication drops the replication=database query param — ddl-tools
// uses a regular connection, where that parameter is invalid.
func ddlStripReplication(dbURL string) string {
	u, err := url.Parse(dbURL)
	if err != nil {
		return dbURL
	}
	q := u.Query()
	q.Del("replication")
	u.RawQuery = q.Encode()
	return u.String()
}
