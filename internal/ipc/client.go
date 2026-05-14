package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
)

type Client struct {
	conn   net.Conn
	enc    *json.Encoder
	dec    *json.Decoder
	writer *bufio.Writer

	mu sync.Mutex

	hello    HelloData
	snapshot []store.Event
	events   chan store.Event
	stats    chan StatsData
	errs     chan error

	dropped atomic.Uint64
}

// Dropped returns the number of events that were dropped because the client's
// internal events channel was full (slow consumer). Useful for diagnostics.
func (c *Client) Dropped() uint64 {
	return c.dropped.Load()
}

func Dial(ctx context.Context, socketPath string) (*Client, error) {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("connect to daemon socket: %w", err)
	}

	writer := bufio.NewWriter(conn)
	c := &Client{
		conn:   conn,
		enc:    json.NewEncoder(writer),
		dec:    json.NewDecoder(conn),
		writer: writer,
		events: make(chan store.Event, 100),
		stats:  make(chan StatsData, 8),
		errs:   make(chan error, 1),
	}

	if err := c.readHandshake(); err != nil {
		_ = c.Close()
		return nil, err
	}

	go c.readLoop()
	return c, nil
}

func (c *Client) readHandshake() error {
	var helloEnv Envelope
	if err := c.dec.Decode(&helloEnv); err != nil {
		return fmt.Errorf("read hello envelope: %w", err)
	}
	if helloEnv.Type != TypeHello {
		return fmt.Errorf("unexpected first message type %q", helloEnv.Type)
	}
	if err := json.Unmarshal(helloEnv.Data, &c.hello); err != nil {
		return fmt.Errorf("decode hello payload: %w", err)
	}

	var snapshotEnv Envelope
	if err := c.dec.Decode(&snapshotEnv); err != nil {
		return fmt.Errorf("read snapshot envelope: %w", err)
	}
	if snapshotEnv.Type != TypeSnapshot {
		return fmt.Errorf("unexpected second message type %q", snapshotEnv.Type)
	}
	if len(snapshotEnv.Data) > 0 {
		if err := json.Unmarshal(snapshotEnv.Data, &c.snapshot); err != nil {
			return fmt.Errorf("decode snapshot payload: %w", err)
		}
	}
	return nil
}

func (c *Client) readLoop() {
	defer close(c.events)
	defer close(c.stats)
	defer close(c.errs)

	for {
		var env Envelope
		if err := c.dec.Decode(&env); err != nil {
			c.errs <- fmt.Errorf("read daemon stream: %w", err)
			return
		}

		switch env.Type {
		case TypeEvent:
			var e store.Event
			if err := json.Unmarshal(env.Data, &e); err != nil {
				c.errs <- fmt.Errorf("decode event: %w", err)
				return
			}
			select {
			case c.events <- e:
			default:
				// Slow consumer — drop this event for this client. The daemon
				// keeps the canonical buffer; reattaching will resync via the
				// snapshot. We track the count for diagnostics rather than
				// silently swallowing.
				n := c.dropped.Add(1)
				if n == 1 || n%100 == 0 {
					slog.Debug("ipc client dropped event (slow consumer)",
						"event_id", e.ID, "table", e.Table, "dropped_total", n)
				}
			}
		case TypeStats:
			var s StatsData
			if err := json.Unmarshal(env.Data, &s); err == nil {
				select {
				case c.stats <- s:
				default:
				}
			}
		}
	}
}

func (c *Client) Hello() HelloData {
	return c.hello
}

func (c *Client) Snapshot() []store.Event {
	out := make([]store.Event, len(c.snapshot))
	copy(out, c.snapshot)
	return out
}

func (c *Client) Events() <-chan store.Event {
	return c.events
}

func (c *Client) Stats() <-chan StatsData {
	return c.stats
}

func (c *Client) Errors() <-chan error {
	return c.errs
}

func (c *Client) Ping() error {
	return c.send(Envelope{Type: TypePing})
}

func (c *Client) SubscribeAll() error {
	return c.send(Envelope{Type: TypeSubscribe, Data: mustMarshal(SubscribeData{})})
}

func (c *Client) RequestStats(ctx context.Context) (StatsData, error) {
	if err := c.Ping(); err != nil {
		return StatsData{}, err
	}

	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()

	for {
		select {
		case s, ok := <-c.stats:
			if !ok {
				return StatsData{}, fmt.Errorf("stats stream closed")
			}
			return s, nil
		case err := <-c.errs:
			if err == nil {
				return StatsData{}, fmt.Errorf("connection closed")
			}
			return StatsData{}, err
		case <-ctx.Done():
			return StatsData{}, ctx.Err()
		case <-timer.C:
			return StatsData{}, fmt.Errorf("timed out waiting for stats")
		}
	}
}

func (c *Client) send(env Envelope) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.enc.Encode(env); err != nil {
		return fmt.Errorf("send ipc message: %w", err)
	}
	if err := c.writer.Flush(); err != nil {
		return fmt.Errorf("flush ipc message: %w", err)
	}
	return nil
}

func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}
