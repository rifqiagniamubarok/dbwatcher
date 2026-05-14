package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
)

type ServerOptions struct {
	SocketPath string
	Version    string
	DB         string
	Capacity   int

	Snapshot    func() []store.Event
	Subscribe   func(filter store.Filter) <-chan store.Event
	Unsubscribe func(ch <-chan store.Event)
	Stats       func(clients int) StatsData
}

type Server struct {
	opts    ServerOptions
	ln      net.Listener
	clients atomic.Int64
}

func NewServer(opts ServerOptions) *Server {
	return &Server{opts: opts}
}

func (s *Server) ClientCount() int {
	return int(s.clients.Load())
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	if err := os.RemoveAll(s.opts.SocketPath); err != nil {
		return fmt.Errorf("remove stale socket: %w", err)
	}

	ln, err := net.Listen("unix", s.opts.SocketPath)
	if err != nil {
		return fmt.Errorf("listen unix socket: %w", err)
	}
	s.ln = ln
	if err := os.Chmod(s.opts.SocketPath, 0o600); err != nil {
		_ = ln.Close()
		return fmt.Errorf("set socket permissions: %w", err)
	}

	errCh := make(chan error, 1)
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				_ = os.Remove(s.opts.SocketPath)
				return nil
			}
			select {
			case errCh <- fmt.Errorf("accept connection: %w", err):
			default:
			}
			continue
		}
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	s.clients.Add(1)
	defer s.clients.Add(-1)

	writer := bufio.NewWriter(conn)
	enc := json.NewEncoder(writer)
	dec := json.NewDecoder(conn)
	var writeMu sync.Mutex

	send := func(env Envelope) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		if err := enc.Encode(env); err != nil {
			return err
		}
		return writer.Flush()
	}

	if err := send(helloEnvelope(HelloData{Version: s.opts.Version, DB: s.opts.DB})); err != nil {
		return
	}
	if err := send(snapshotEnvelope(s.opts.Snapshot())); err != nil {
		return
	}

	stopPump := make(chan struct{})
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() { close(stopPump) })
	}
	defer stop()

	sub := s.opts.Subscribe(&store.AllowAllFilter{})
	defer s.opts.Unsubscribe(sub)

	go func(ch <-chan store.Event) {
		for {
			select {
			case <-stopPump:
				return
			case e, ok := <-ch:
				if !ok {
					return
				}
				if err := send(eventEnvelope(e)); err != nil {
					stop()
					return
				}
			}
		}
	}(sub)

	statsTicker := time.NewTicker(5 * time.Second)
	defer statsTicker.Stop()
	go func() {
		for {
			select {
			case <-stopPump:
				return
			case <-statsTicker.C:
				if err := send(statsEnvelope(s.opts.Stats(s.ClientCount()))); err != nil {
					stop()
					return
				}
			}
		}
	}()

	for {
		select {
		case <-stopPump:
			return
		default:
		}

		var env Envelope
		if err := dec.Decode(&env); err != nil {
			return
		}

		switch env.Type {
		case TypePing:
			if err := send(statsEnvelope(s.opts.Stats(s.ClientCount()))); err != nil {
				return
			}
		case TypeSubscribe:
			// Protocol placeholder: table-filtered subscriptions can be added later.
			if err := send(Envelope{Type: TypePong}); err != nil {
				return
			}
		}
	}
}
