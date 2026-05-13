package store

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeEvent(id uint64, table string) Event {
	return Event{ID: id, Timestamp: time.Now(), Type: EventInsert, Schema: "public", Table: table}
}

// --- Push & Snapshot ---

func TestStore_PushAndSnapshot(t *testing.T) {
	s := New(10)
	s.Push(makeEvent(1, "users"))
	snap := s.Snapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, uint64(1), snap[0].ID)
}

func TestStore_SnapshotOrder(t *testing.T) {
	s := New(10)
	for i := uint64(1); i <= 5; i++ {
		s.Push(makeEvent(i, "users"))
	}
	snap := s.Snapshot()
	require.Len(t, snap, 5)
	for i, e := range snap {
		assert.Equal(t, uint64(i+1), e.ID)
	}
}

func TestStore_RingBufferOverwrite(t *testing.T) {
	s := New(3)
	for i := uint64(1); i <= 5; i++ {
		s.Push(makeEvent(i, "users"))
	}
	snap := s.Snapshot()
	require.Len(t, snap, 3)
	// Oldest events (1,2) evicted; 3,4,5 remain in order.
	assert.Equal(t, uint64(3), snap[0].ID)
	assert.Equal(t, uint64(4), snap[1].ID)
	assert.Equal(t, uint64(5), snap[2].ID)
}

// --- Stats ---

func TestStore_Stats(t *testing.T) {
	s := New(10)
	for i := uint64(1); i <= 7; i++ {
		s.Push(makeEvent(i, "users"))
	}
	st := s.Stats()
	assert.Equal(t, uint64(7), st.Total)
	assert.Equal(t, 7, st.Buffered)
}

func TestStore_StatsOverCapacity(t *testing.T) {
	s := New(3)
	for i := uint64(1); i <= 5; i++ {
		s.Push(makeEvent(i, "users"))
	}
	st := s.Stats()
	assert.Equal(t, uint64(5), st.Total)
	assert.Equal(t, 3, st.Buffered)
}

// --- Subscribe ---

func TestStore_SubscriberReceivesEvents(t *testing.T) {
	s := New(10)
	ch := s.Subscribe()
	defer s.Unsubscribe(ch)

	s.Push(makeEvent(1, "users"))

	select {
	case e := <-ch:
		assert.Equal(t, uint64(1), e.ID)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestStore_MultipleSubscribersAllReceive(t *testing.T) {
	s := New(10)
	ch1 := s.Subscribe()
	ch2 := s.Subscribe()
	defer s.Unsubscribe(ch1)
	defer s.Unsubscribe(ch2)

	s.Push(makeEvent(1, "orders"))

	for _, ch := range []<-chan Event{ch1, ch2} {
		select {
		case e := <-ch:
			assert.Equal(t, uint64(1), e.ID)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for event on subscriber")
		}
	}
}

func TestStore_UnsubscribeStopsDelivery(t *testing.T) {
	s := New(10)
	ch := s.Subscribe()
	s.Unsubscribe(ch)

	s.Push(makeEvent(1, "users"))

	select {
	case _, ok := <-ch:
		assert.False(t, ok, "channel should be closed after unsubscribe")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("channel not closed after unsubscribe")
	}
}

// --- Filter ---

func TestStore_FilterSubscriber(t *testing.T) {
	s := New(10)
	ch := s.SubscribeWithFilter(&TableFilter{Tables: []string{"orders"}})
	defer s.Unsubscribe(ch)

	s.Push(makeEvent(1, "users"))   // should be filtered out
	s.Push(makeEvent(2, "orders"))  // should pass

	select {
	case e := <-ch:
		assert.Equal(t, uint64(2), e.ID)
		assert.Equal(t, "orders", e.Table)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for filtered event")
	}

	// Ensure the users event did NOT arrive.
	select {
	case e := <-ch:
		t.Fatalf("unexpected event: %+v", e)
	case <-time.After(50 * time.Millisecond):
		// good — no extra event
	}
}

func TestStore_AllowAllFilter(t *testing.T) {
	s := New(10)
	ch := s.SubscribeWithFilter(&AllowAllFilter{})
	defer s.Unsubscribe(ch)

	s.Push(makeEvent(1, "users"))
	s.Push(makeEvent(2, "orders"))

	for i := 0; i < 2; i++ {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}
}

// --- Concurrency ---

func TestStore_ConcurrentPush(t *testing.T) {
	s := New(1000)
	ch := s.Subscribe()
	defer s.Unsubscribe(ch)

	const goroutines = 10
	const eventsEach = 50

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < eventsEach; i++ {
				s.Push(makeEvent(uint64(g*eventsEach+i), "users"))
			}
		}(g)
	}
	wg.Wait()

	st := s.Stats()
	assert.Equal(t, uint64(goroutines*eventsEach), st.Total)
}
