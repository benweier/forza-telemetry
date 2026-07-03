// Package stream owns the in-process Tick fan-out: the ingest listener Publishes
// each enriched Tick, and any number of subscribers (WebSocket sessions, storage
// writer, derived analytics) receive them.
//
// A small ring buffer is kept for late-joiner replay on subscribe so a freshly
// connected browser gets immediate context instead of an empty view.
package stream

import (
	"sync"
	"sync/atomic"

	"github.com/benweier/forza-telemetry/server/internal/tick"
)

// subBuf is the per-subscriber channel buffer: a slow consumer's grace window
// before frames are dropped (~4s at 60Hz).
const subBuf = 256

type Subscription struct {
	ch      chan *tick.Tick
	broker  *Broker
	dropped atomic.Uint64
}

func (s *Subscription) C() <-chan *tick.Tick { return s.ch }

// Dropped reports frames dropped on this subscription because its buffer was
// full. Consumers with durability contracts (the storage writer) should watch
// this and log — a nonzero value means a gap in what they received.
func (s *Subscription) Dropped() uint64 { return s.dropped.Load() }

func (s *Subscription) Close() {
	s.broker.unsubscribe(s)
}

type Broker struct {
	mu      sync.RWMutex
	subs    map[*Subscription]struct{}
	ring    []*tick.Tick
	ringSz  int
	head    int
	full    bool
	dropped atomic.Uint64
}

// Dropped reports the total frames dropped across all subscribers since the
// broker was created. Live UIs falling behind is tolerable; a rising counter
// with no UI connected means something upstream of durability is starving.
func (b *Broker) Dropped() uint64 { return b.dropped.Load() }

func NewBroker(ringSize int) *Broker {
	if ringSize <= 0 {
		ringSize = 1
	}
	return &Broker{
		subs:   make(map[*Subscription]struct{}),
		ring:   make([]*tick.Tick, ringSize),
		ringSz: ringSize,
	}
}

func (b *Broker) Publish(t *tick.Tick) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ring[b.head] = t
	b.head = (b.head + 1) % b.ringSz
	if b.head == 0 {
		b.full = true
	}
	// Sends happen under the lock so unsubscribe's close() can never
	// interleave with a send (send on a closed channel panics). Sends are
	// non-blocking, so holding the lock here is cheap.
	for s := range b.subs {
		select {
		case s.ch <- t:
		default:
			// Subscriber is slow; drop rather than block ingest. Live UIs that
			// fall behind can resubscribe to re-anchor.
			s.dropped.Add(1)
			b.dropped.Add(1)
		}
	}
}

// Subscribe returns a new Subscription. If `replay` is true, the current ring
// contents are pushed onto the channel before live frames begin to arrive.
// Channel buffer is sized to give a slow consumer a small grace window before
// drops begin.
func (b *Broker) Subscribe(replay bool) *Subscription {
	return b.SubscribeBuffered(subBuf, replay)
}

// SubscribeBuffered is Subscribe with an explicit buffer size, for consumers
// whose contract is durability rather than liveness (the storage writer): a
// ring-sized buffer rides out multi-second stalls (stint-close aggregation)
// that would overflow the default UI-grade buffer.
func (b *Broker) SubscribeBuffered(buf int, replay bool) *Subscription {
	if buf < 1 {
		buf = 1
	}
	// A replaying subscriber's channel must hold the full ring plus a live
	// grace window — with only subBuf capacity the replay kept the *oldest*
	// ~4s of the ring and silently dropped the rest.
	if replay {
		buf += b.ringSz
	}
	sub := &Subscription{ch: make(chan *tick.Tick, buf), broker: b}

	b.mu.Lock()
	if replay {
		for _, t := range b.snapshotLocked() {
			select {
			case sub.ch <- t:
			default:
			}
		}
	}
	b.subs[sub] = struct{}{}
	b.mu.Unlock()
	return sub
}

func (b *Broker) unsubscribe(s *Subscription) {
	b.mu.Lock()
	if _, ok := b.subs[s]; ok {
		delete(b.subs, s)
		close(s.ch)
	}
	b.mu.Unlock()
}

func (b *Broker) snapshotLocked() []*tick.Tick {
	if !b.full {
		out := make([]*tick.Tick, b.head)
		copy(out, b.ring[:b.head])
		return out
	}
	out := make([]*tick.Tick, 0, b.ringSz)
	out = append(out, b.ring[b.head:]...)
	out = append(out, b.ring[:b.head]...)
	return out
}
