package ipc

import (
	"context"
	"testing"
	"time"

	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerClientRoundtrip(t *testing.T) {
	s := store.New(32)
	e := store.Event{ID: 1, Table: "users", Type: store.EventInsert, Timestamp: time.Now()}
	s.Push(e)

	socketPath := t.TempDir() + "/dbwatch.sock"
	server := NewServer(ServerOptions{
		SocketPath: socketPath,
		Version:    "test",
		DB:         "postgres://test",
		Capacity:   32,
		Snapshot:   s.Snapshot,
		Subscribe:  s.SubscribeWithFilter,
		Unsubscribe: func(ch <-chan store.Event) {
			s.Unsubscribe(ch)
		},
		Stats: func(clients int) StatsData {
			st := s.Stats()
			return StatsData{Received: st.Total, Clients: clients, Buffered: st.Buffered, Capacity: 32}
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.ListenAndServe(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	client, err := Dial(context.Background(), socketPath)
	require.NoError(t, err)
	defer client.Close()

	hello := client.Hello()
	assert.Equal(t, "test", hello.Version)
	assert.Equal(t, "postgres://test", hello.DB)

	snapshot := client.Snapshot()
	require.Len(t, snapshot, 1)
	assert.Equal(t, uint64(1), snapshot[0].ID)

	next := store.Event{ID: 2, Table: "orders", Type: store.EventInsert, Timestamp: time.Now()}
	s.Push(next)

	select {
	case got := <-client.Events():
		assert.Equal(t, uint64(2), got.ID)
		assert.Equal(t, "orders", got.Table)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for streamed event")
	}

	stats, err := client.RequestStats(context.Background())
	require.NoError(t, err)
	assert.Equal(t, uint64(2), stats.Received)
	assert.GreaterOrEqual(t, stats.Clients, 1)

	cancel()
	select {
	case err := <-serveErr:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for server shutdown")
	}
}
