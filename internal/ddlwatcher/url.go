package ddlwatcher

import "net/url"

// stripReplicationParam removes the replication=database query parameter from
// a connection URL. The DDL listener uses a regular pg connection (LISTEN /
// NOTIFY), where that parameter is invalid.
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
