// Package stream owns the in-process Tick fan-out: the ingest listener Publishes
// each enriched Tick, and any number of subscribers (WebSocket sessions, storage
// writer, derived analytics) receive them.
//
// A small ring buffer is kept for late-joiner replay on subscribe so a freshly
// connected browser gets immediate context instead of an empty view.
package stream

import (
	"sync"

	"github.com/benweier/forza-telemetry/server/internal/tick"
)

type Subscription struct {
	ch     chan *tick.Tick
	broker *Broker
}

func (s *Subscription) C() <-chan *tick.Tick { return s.ch }

func (s *Subscription) Close() {
	s.broker.unsubscribe(s)
}

type Broker struct {
	mu     sync.RWMutex
	subs   map[*Subscription]struct{}
	ring   []*tick.Tick
	ringSz int
	head   int
	full   bool
}

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
	b.ring[b.head] = t
	b.head = (b.head + 1) % b.ringSz
	if b.head == 0 {
		b.full = true
	}
	subs := make([]*Subscription, 0, len(b.subs))
	for s := range b.subs {
		subs = append(subs, s)
	}
	b.mu.Unlock()

	for _, s := range subs {
		select {
		case s.ch <- t:
		default:
			// Subscriber is slow; drop rather than block ingest. Live UIs that
			// fall behind can resubscribe to re-anchor.
		}
	}
}

// Subscribe returns a new Subscription. If `replay` is true, the current ring
// contents are pushed onto the channel before live frames begin to arrive.
// Channel buffer is sized to give a slow consumer a small grace window before
// drops begin.
func (b *Broker) Subscribe(replay bool) *Subscription {
	const subBuf = 256
	sub := &Subscription{ch: make(chan *tick.Tick, subBuf), broker: b}

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
