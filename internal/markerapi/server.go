// Package markerapi exposes a small HTTP API that lets external tools
// (test runners, CI scripts, ad-hoc curl) push markers and log lines
// into the DBWatch feed without speaking the IPC protocol.
//
// All endpoints bind to 127.0.0.1 by default. The API is intended for
// local development workflows only — there is no authentication.
package markerapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
)

// DefaultPort is the port the marker API listens on when none is specified.
const DefaultPort = 6677

// DefaultBind is the address the marker API binds to when none is specified.
// It intentionally points at the loopback interface so the API is not
// accessible from other hosts on the network.
const DefaultBind = "127.0.0.1"

// Pusher is the subset of *store.Store the marker API needs. Keeping this
// as an interface makes the handlers trivially testable without spinning
// up the real ring buffer.
type Pusher interface {
	Push(e store.Event)
}

// Options configures a Server.
type Options struct {
	Bind     string    // host to bind to, defaults to DefaultBind
	Port     int       // TCP port, defaults to DefaultPort
	Store    Pusher    // where marker / log entries are pushed
	StartAt  time.Time // used to report uptime on /health
	Version  string    // reported on /health
}

// Server wraps a net/http server and the listener address that was actually
// bound (useful when Port is 0 for tests).
type Server struct {
	opts Options
	srv  *http.Server

	mu   sync.RWMutex
	addr string
}

// New constructs a Server with the given options applied on top of defaults.
func New(opts Options) *Server {
	if opts.Bind == "" {
		opts.Bind = DefaultBind
	}
	if opts.Port == 0 {
		opts.Port = DefaultPort
	}
	if opts.StartAt.IsZero() {
		opts.StartAt = time.Now()
	}
	return &Server{opts: opts}
}

// Addr returns the address the server is bound to. Only valid after
// ListenAndServe has been called.
func (s *Server) Addr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.addr
}

func (s *Server) setAddr(addr string) {
	s.mu.Lock()
	s.addr = addr
	s.mu.Unlock()
}

// ListenAndServe binds the listener and serves until ctx is cancelled.
// The returned error is nil on a clean ctx-cancel shutdown.
func (s *Server) ListenAndServe(ctx context.Context) error {
	if s.opts.Store == nil {
		return errors.New("markerapi: Store is required")
	}

	mux := http.NewServeMux()
	h := &handlers{store: s.opts.Store, startAt: s.opts.StartAt, version: s.opts.Version}
	mux.HandleFunc("/marker", h.handleMarker)
	mux.HandleFunc("/log", h.handleLog)
	mux.HandleFunc("/health", h.handleHealth)

	addr := fmt.Sprintf("%s:%d", s.opts.Bind, s.opts.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("marker api listen %s: %w", addr, err)
	}
	s.setAddr(ln.Addr().String())

	s.srv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		err := s.srv.Serve(ln)
		if !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shutdownCtx)
		<-errCh
		return nil
	case err := <-errCh:
		return err
	}
}
