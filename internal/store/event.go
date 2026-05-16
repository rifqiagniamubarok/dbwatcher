package store

import (
	"encoding/json"
	"fmt"
	"time"
)

const timestampFormat = "15:04:05.000"

type EventType string

const (
	EventInsert EventType = "INSERT"
	EventUpdate EventType = "UPDATE"
	EventDelete EventType = "DELETE"
)

// Kind discriminates between a database change (default) and externally
// pushed feed items (markers, log lines from the marker HTTP API).
// An empty Kind is treated as KindEvent for backward compatibility.
type Kind string

const (
	KindEvent  Kind = "event"
	KindMarker Kind = "marker"
	KindLog    Kind = "log"
)

// Allowed marker colors (validated by the marker API).
const (
	MarkerColorDefault = "default"
	MarkerColorYellow  = "yellow"
	MarkerColorGreen   = "green"
	MarkerColorRed     = "red"
	MarkerColorBlue    = "blue"
	MarkerColorDim     = "dim"
)

// AllowedMarkerColors lists every color string the marker API accepts.
var AllowedMarkerColors = []string{
	MarkerColorDefault,
	MarkerColorYellow,
	MarkerColorGreen,
	MarkerColorRed,
	MarkerColorBlue,
	MarkerColorDim,
}

type Column struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
	IsKey    bool   `json:"is_key"`
}

// Event is a single item in the feed. For database changes (KindEvent or
// empty) the Type / Schema / Table / NewValues / OldValues fields are
// populated. For KindMarker the Label / Color fields are populated.
// For KindLog the Message field is populated.
type Event struct {
	ID        uint64         `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	Kind      Kind           `json:"kind,omitempty"`

	// Database-change fields (KindEvent).
	LSN       string         `json:"lsn,omitempty"`
	Type      EventType      `json:"type,omitempty"`
	Schema    string         `json:"schema,omitempty"`
	Table     string         `json:"table,omitempty"`
	Columns   []Column       `json:"columns,omitempty"`
	NewValues map[string]any `json:"new_values,omitempty"`
	OldValues map[string]any `json:"old_values,omitempty"`
	TxID      uint32         `json:"tx_id,omitempty"`

	// Marker fields (KindMarker).
	Label string `json:"label,omitempty"`
	Color string `json:"color,omitempty"`

	// Log fields (KindLog).
	Message string `json:"message,omitempty"`
}

// IsMarker reports whether this entry is a marker (separator line in the TUI).
func (e Event) IsMarker() bool { return e.Kind == KindMarker }

// IsLog reports whether this entry is a free-form log line.
func (e Event) IsLog() bool { return e.Kind == KindLog }

// IsDBEvent reports whether this entry is a database change event.
// Empty Kind is treated as a database event for backward compatibility.
func (e Event) IsDBEvent() bool { return e.Kind == "" || e.Kind == KindEvent }

// NewMarker constructs a marker feed item. Empty color means "default".
func NewMarker(label, color string) Event {
	if color == "" {
		color = MarkerColorDefault
	}
	return Event{
		Timestamp: time.Now(),
		Kind:      KindMarker,
		Label:     label,
		Color:     color,
	}
}

// NewLog constructs a free-form log entry.
func NewLog(message string) Event {
	return Event{
		Timestamp: time.Now(),
		Kind:      KindLog,
		Message:   message,
	}
}

func (e Event) JSON() (string, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return "", fmt.Errorf("marshal event: %w", err)
	}
	return string(b), nil
}

func (e Event) String() string {
	switch e.Kind {
	case KindMarker:
		return fmt.Sprintf("[%s] MARKER %s", e.Timestamp.Format(timestampFormat), e.Label)
	case KindLog:
		return fmt.Sprintf("[%s] LOG %s", e.Timestamp.Format(timestampFormat), e.Message)
	default:
		return fmt.Sprintf("[%s] %s %s.%s", e.Timestamp.Format(timestampFormat), e.Type, e.Schema, e.Table)
	}
}
