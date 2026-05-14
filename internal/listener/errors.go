package listener

import (
	"errors"
	"net/url"
	"strings"
)

// friendlyError converts raw Postgres/network errors into actionable messages.
func friendlyError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()

	switch {
	case isConnRefused(msg):
		return errors.New(
			"Cannot connect to Postgres.\n" +
				"  → Is the database running and accessible at the given address?\n" +
				"  → Check --db-url or DBWATCH_DB_URL.",
		)
	case strings.Contains(msg, "wal_level"):
		return errors.New(
			"Postgres is not configured for logical replication.\n" +
				"  → Set wal_level=logical in postgresql.conf and restart Postgres.\n" +
				"  → For Docker: add -c wal_level=logical to your docker run command.",
		)
	case strings.Contains(msg, "REPLICATION") && strings.Contains(msg, "privilege"):
		return errors.New(
			"The database user does not have REPLICATION privilege.\n" +
				"  → Run: ALTER USER <your-user> REPLICATION;",
		)
	case strings.Contains(msg, "role") && strings.Contains(msg, "does not exist"):
		return errors.New(
			"Database user not found.\n" +
				"  → Check the username in your --db-url connection string.",
		)
	case strings.Contains(msg, "password authentication failed"):
		return errors.New(
			"Password authentication failed.\n" +
				"  → Check the password in your --db-url connection string.",
		)
	case strings.Contains(msg, "database") && strings.Contains(msg, "does not exist"):
		return errors.New(
			"Database not found.\n" +
				"  → Check the database name in your --db-url connection string.",
		)
	case strings.Contains(msg, "publication") && strings.Contains(msg, "does not exist"):
		return errors.New(
			"Publication not found.\n" +
				"  → Create it manually: CREATE PUBLICATION dbwatch_pub FOR ALL TABLES;\n" +
				"  → Or use --publication to specify an existing publication name.",
		)
	case strings.Contains(msg, "replication slot") && strings.Contains(msg, "already exists"):
		return errors.New(
			"Replication slot already exists (possibly from a previous crashed run).\n" +
				"  → Drop it: SELECT pg_drop_replication_slot('<slot-name>');\n" +
				"  → Or use --slot to specify a different slot name.",
		)
	case strings.Contains(msg, "tls") || strings.Contains(msg, "TLS"):
		return errors.New(
			"TLS connection error.\n" +
				"  → If your database does not require TLS, add ?sslmode=disable to the URL.\n" +
				"  → Example: postgres://user:pass@localhost:5432/db?sslmode=disable",
		)
	}

	return err
}

// stripReplicationParam removes the replication=database query parameter from
// a Postgres connection URL. That parameter is only valid for pgconn replication
// connections; a regular pgx connection will reject it.
func stripReplicationParam(dbURL string) string {
	u, err := url.Parse(dbURL)
	if err != nil {
		// Not parseable — return as-is and let pgx report the real error.
		return dbURL
	}
	q := u.Query()
	q.Del("replication")
	u.RawQuery = q.Encode()
	return u.String()
}

func isConnRefused(msg string) bool {
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connect: no such file") ||
		strings.Contains(msg, "no route to host") ||
		strings.Contains(msg, "i/o timeout")
}
