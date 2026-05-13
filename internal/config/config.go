package config

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const (
	DefaultPublication = "dbwatch_pub"
	DefaultSlot        = "dbwatch_slot"
	DefaultBufferSize  = 1000
	DefaultLogLevel    = "warn"
)

// Config holds all runtime configuration for dbwatch.
type Config struct {
	DBURL       string
	Publication string
	Slot        string
	BufferSize  int
	LogLevel    string
}

// Load reads config from environment variables, then overrides with flags if set.
func Load(cmd *cobra.Command) (*Config, error) {
	cfg := &Config{
		DBURL:       envOr("DBWATCH_DB_URL", ""),
		Publication: envOr("DBWATCH_PUBLICATION", DefaultPublication),
		Slot:        envOr("DBWATCH_SLOT", DefaultSlot),
		BufferSize:  DefaultBufferSize,
		LogLevel:    envOr("DBWATCH_LOG_LEVEL", DefaultLogLevel),
	}

	if cmd.Flags().Changed("db-url") {
		cfg.DBURL, _ = cmd.Flags().GetString("db-url")
	}
	if cmd.Flags().Changed("publication") {
		cfg.Publication, _ = cmd.Flags().GetString("publication")
	}
	if cmd.Flags().Changed("slot") {
		cfg.Slot, _ = cmd.Flags().GetString("slot")
	}
	if cmd.Flags().Changed("log-level") {
		cfg.LogLevel, _ = cmd.Flags().GetString("log-level")
	}
	if cmd.Flags().Changed("buffer") {
		cfg.BufferSize, _ = cmd.Flags().GetInt("buffer")
	}

	if cfg.DBURL == "" {
		return nil, fmt.Errorf(
			"database URL is required\n\nSet it via --db-url flag or DBWATCH_DB_URL environment variable.\nExample: --db-url=postgres://user:pass@localhost:5432/mydb",
		)
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
