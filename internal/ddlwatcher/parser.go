package ddlwatcher

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
)

// ddlPayload mirrors the JSON object built by the Postgres capture functions
// (see sql.go). Every field is optional on the wire — a malformed or partial
// payload should degrade gracefully rather than crash the listener.
type ddlPayload struct {
	CommandTag     string  `json:"command_tag"`
	ObjectType     string  `json:"object_type"`
	Schema         string  `json:"schema"`
	ObjectIdentity string  `json:"object_identity"`
	InExtension    bool    `json:"in_extension"`
	Timestamp      float64 `json:"timestamp"`
}

// parsePayload converts a raw pg_notify payload into a DDL Event.
// It returns an error for malformed JSON or an empty command tag so the
// caller can log-and-skip without aborting the listen loop.
func parsePayload(raw string) (store.Event, error) {
	var p ddlPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return store.Event{}, fmt.Errorf("decode ddl payload: %w", err)
	}

	tag := strings.TrimSpace(p.CommandTag)
	if tag == "" {
		return store.Event{}, fmt.Errorf("ddl payload has empty command_tag")
	}

	identity := strings.TrimSpace(p.ObjectIdentity)
	if identity == "" {
		// Fall back to the schema name when the object identity is missing
		// (some command tags report no identity).
		identity = strings.TrimSpace(p.Schema)
	}

	e := store.NewDDL(tag, strings.TrimSpace(p.ObjectType), identity)
	if p.Timestamp > 0 {
		// Postgres reports epoch seconds with a fractional part.
		sec := int64(p.Timestamp)
		nsec := int64((p.Timestamp - float64(sec)) * 1e9)
		e.Timestamp = time.Unix(sec, nsec)
	}
	return e, nil
}
