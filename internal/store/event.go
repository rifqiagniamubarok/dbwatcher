package store

import (
	"encoding/json"
	"fmt"
	"time"
)

type EventType string

const (
	EventInsert EventType = "INSERT"
	EventUpdate EventType = "UPDATE"
	EventDelete EventType = "DELETE"
)

type Column struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
	IsKey    bool   `json:"is_key"`
}

type Event struct {
	ID        uint64         `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	LSN       string         `json:"lsn"`
	Type      EventType      `json:"type"`
	Schema    string         `json:"schema"`
	Table     string         `json:"table"`
	Columns   []Column       `json:"columns,omitempty"`
	NewValues map[string]any `json:"new_values,omitempty"`
	OldValues map[string]any `json:"old_values,omitempty"`
	TxID      uint32         `json:"tx_id,omitempty"`
}

func (e Event) JSON() (string, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return "", fmt.Errorf("marshal event: %w", err)
	}
	return string(b), nil
}

func (e Event) String() string {
	return fmt.Sprintf("[%s] %s %s.%s", e.Timestamp.Format("15:04:05.000"), e.Type, e.Schema, e.Table)
}
