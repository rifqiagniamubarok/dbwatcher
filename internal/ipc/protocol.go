package ipc

import (
	"encoding/json"
	"time"

	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
)

const (
	TypeHello     = "hello"
	TypeSnapshot  = "snapshot"
	TypeEvent     = "event"
	TypeStats     = "stats"
	TypePing      = "ping"
	TypePong      = "pong"
	TypeSubscribe = "subscribe"
)

// Envelope is the common wire message shape for NDJSON over Unix sockets.
type Envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

type HelloData struct {
	Version string `json:"version"`
	DB      string `json:"db"`
}

type StatsData struct {
	UptimeSeconds int64   `json:"uptime_s"`
	Received      uint64  `json:"received"`
	Clients       int     `json:"clients"`
	LastLSN       string  `json:"last_lsn"`
	Buffered      int     `json:"buffered"`
	Capacity      int     `json:"capacity"`
	BufferRatio   float64 `json:"buffer_ratio"`
}

type SubscribeFilter struct {
	Tables []string `json:"tables,omitempty"`
}

type SubscribeData struct {
	Filter SubscribeFilter `json:"filter"`
}

func mustMarshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func helloEnvelope(data HelloData) Envelope {
	return Envelope{Type: TypeHello, Data: mustMarshal(data)}
}

func snapshotEnvelope(events []store.Event) Envelope {
	return Envelope{Type: TypeSnapshot, Data: mustMarshal(events)}
}

func eventEnvelope(e store.Event) Envelope {
	return Envelope{Type: TypeEvent, Data: mustMarshal(e)}
}

func statsEnvelope(s StatsData) Envelope {
	return Envelope{Type: TypeStats, Data: mustMarshal(s)}
}

func nowSeconds(start time.Time) int64 {
	if start.IsZero() {
		return 0
	}
	return int64(time.Since(start).Seconds())
}
