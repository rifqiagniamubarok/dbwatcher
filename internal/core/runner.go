package core

import (
	"context"

	"github.com/rifqiagniamubarok/dbwatcher/internal/config"
	"github.com/rifqiagniamubarok/dbwatcher/internal/listener"
	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
)

// Run starts the listener and forwards decoded events into the provided store.
// It blocks until the listener exits or context is cancelled.
//
// onEvent (optional) is invoked synchronously on the forwarder goroutine after
// each successful Store.Push. Keep it fast and non-blocking — anything slow
// here back-pressures the entire Listener→Store pipeline. The tail command
// uses this to emit JSON to stdout; daemon mode passes nil and lets clients
// subscribe to the Store directly.
func Run(ctx context.Context, cfg *config.Config, st *store.Store, onEvent func(store.Event)) error {
	listenerIn := make(chan store.Event, cfg.BufferSize)
	l := listener.New(cfg.DBURL, cfg.Publication, cfg.Slot, listenerIn)

	forwardDone := make(chan struct{})
	go func() {
		defer close(forwardDone)
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-listenerIn:
				if !ok {
					return
				}
				st.Push(e)
				if onEvent != nil {
					onEvent(e)
				}
			}
		}
	}()

	err := l.Start(ctx)
	// Listener does not close the output channel; close it here so the
	// forwarder can exit and Run can return listener errors promptly.
	close(listenerIn)
	<-forwardDone
	return err
}
