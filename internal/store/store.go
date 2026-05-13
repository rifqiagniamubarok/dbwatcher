package store

import "sync"

const defaultSubChanCap = 100

// Stats holds a point-in-time snapshot of Store metrics.
type Stats struct {
	Total    uint64
	Buffered int
}

// Filter decides whether an event should be delivered to a subscriber.
type Filter interface {
	Match(e Event) bool
}

// AllowAllFilter passes every event through.
type AllowAllFilter struct{}

func (f *AllowAllFilter) Match(_ Event) bool { return true }

// TableFilter passes only events whose Table is in the allow-list.
type TableFilter struct {
	Tables []string
}

func (f *TableFilter) Match(e Event) bool {
	for _, t := range f.Tables {
		if t == e.Table {
			return true
		}
	}
	return false
}

type subscriber struct {
	ch     chan Event
	filter Filter
}

// Store holds recent events in a ring buffer and fans them out to subscribers.
type Store struct {
	mu       sync.RWMutex
	ring     []Event
	capacity int
	cursor   int    // next write position
	count    int    // number of valid entries (0..capacity)
	total    uint64 // all-time events received
	subs     []*subscriber
}

// New creates a Store with the given ring buffer capacity.
func New(capacity int) *Store {
	return &Store{
		ring:     make([]Event, capacity),
		capacity: capacity,
	}
}

// Push appends an event to the ring buffer and broadcasts to all subscribers.
func (s *Store) Push(e Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ring[s.cursor] = e
	s.cursor = (s.cursor + 1) % s.capacity
	if s.count < s.capacity {
		s.count++
	}
	s.total++

	for _, sub := range s.subs {
		if sub.filter.Match(e) {
			select {
			case sub.ch <- e:
			default:
				// Subscriber too slow — drop rather than block.
			}
		}
	}
}

// Subscribe returns a channel that receives all future events.
func (s *Store) Subscribe() <-chan Event {
	return s.SubscribeWithFilter(&AllowAllFilter{})
}

// SubscribeWithFilter returns a channel that receives events matching filter.
func (s *Store) SubscribeWithFilter(filter Filter) <-chan Event {
	ch := make(chan Event, defaultSubChanCap)
	s.mu.Lock()
	s.subs = append(s.subs, &subscriber{ch: ch, filter: filter})
	s.mu.Unlock()
	return ch
}

// Unsubscribe removes the subscriber and closes its channel.
func (s *Store) Unsubscribe(ch <-chan Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, sub := range s.subs {
		if sub.ch == ch {
			close(sub.ch)
			s.subs = append(s.subs[:i], s.subs[i+1:]...)
			return
		}
	}
}

// Snapshot returns a copy of all buffered events in oldest-first order.
func (s *Store) Snapshot() []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Event, s.count)
	if s.count < s.capacity {
		copy(out, s.ring[:s.count])
	} else {
		// Ring is full: oldest entry is at cursor.
		n := copy(out, s.ring[s.cursor:])
		copy(out[n:], s.ring[:s.cursor])
	}
	return out
}

// Stats returns current store metrics.
func (s *Store) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Stats{Total: s.total, Buffered: s.count}
}
