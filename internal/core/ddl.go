package core

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/rifqiagniamubarok/dbwatcher/internal/config"
	"github.com/rifqiagniamubarok/dbwatcher/internal/ddlwatcher"
)

// DDLWarn, if set by the caller (cmd/dbwatch), receives a human-readable,
// actionable message when DDL tracking cannot be enabled. core.Run continues
// with DML tracking either way. It is a package-level hook rather than a
// Config field so the cmd layer owns terminal output and the library layer
// never prints directly.
var DDLWarn func(message string)

// ddlSetupHint turns a DDL setup error into an actionable, multi-line message
// for the end user. It always ends by reassuring them DML tracking continues.
func ddlSetupHint(cfg *config.Config, err error) string {
	var b strings.Builder
	b.WriteString("DDL tracking could not be enabled.\n")

	switch {
	case errors.Is(err, ddlwatcher.ErrInsufficientPrivilege):
		b.WriteString("  Reason: the database user lacks EVENT TRIGGER (superuser) privilege.\n")
		b.WriteString("  → Ask your DBA to install the trigger once:\n")
		b.WriteString("      dbwatch ddl-tools install --db-url=<superuser-url>\n")
		b.WriteString("  → Then re-run with: --track-ddl --ddl-install-mode=none\n")
	case cfg.DDLInstallMode == config.DDLInstallManual:
		b.WriteString("  Reason: the event trigger is not installed and install mode is 'manual'.\n")
		b.WriteString("  → Install it once (as superuser):\n")
		b.WriteString("      dbwatch ddl-tools install --db-url=<superuser-url>\n")
		b.WriteString("  → Or print the SQL: dbwatch ddl-tools print-sql\n")
	default:
		fmt.Fprintf(&b, "  Reason: %v\n", err)
		b.WriteString("  → See: dbwatch ddl-tools status --db-url=<url>\n")
	}

	b.WriteString("Continuing with DML tracking only.")
	return b.String()
}

// stripReplication removes the replication=database query parameter so a
// regular (non-replication) pgx connection can be opened for DDL setup.
func stripReplication(dbURL string) string {
	u, err := url.Parse(dbURL)
	if err != nil {
		return dbURL
	}
	q := u.Query()
	q.Del("replication")
	u.RawQuery = q.Encode()
	return u.String()
}
