package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
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
	return &cobra.Command{
		Use:   "tail",
		Short: "Stream database changes to the terminal",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("dbwatch tail: not implemented yet")
			return nil
		},
	}
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
