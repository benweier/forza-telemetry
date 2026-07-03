package stream

import (
	"sync"
	"testing"

	"github.com/benweier/forza-telemetry/server/internal/tick"
)

// Regression test: Publish used to send on subscriber channels outside the
// broker lock while Close() concurrently closed them — send on a closed
// channel panics and took down the whole server. This hammers that interleave.
func TestBrokerConcurrentCloseDuringPublish(t *testing.T) {
	b := NewBroker(16)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 500_000; i++ {
			b.Publish(&tick.Tick{GameTSMillis: uint32(i)})
		}
	}()

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				s := b.Subscribe(false)
				s.Close()
			}
		}()
	}
	wg.Wait()
}

func TestBrokerReplayDeliversRingNewestLast(t *testing.T) {
	b := NewBroker(8)
	for i := 1; i <= 5; i++ {
		b.Publish(&tick.Tick{GameTSMillis: uint32(i)})
	}
	sub := b.Subscribe(true)
	defer sub.Close()

	var got []uint32
	for len(got) < 5 {
		select {
		case tk := <-sub.C():
			got = append(got, tk.GameTSMillis)
		default:
			t.Fatalf("replay delivered only %d of 5 ring ticks: %v", len(got), got)
		}
	}
	for i, ts := range got {
		if ts != uint32(i+1) {
			t.Fatalf("replay out of order: got %v", got)
		}
	}
}

// A replaying subscriber must receive the ENTIRE ring — the channel used to
// be capped at subBuf, so a full ring replayed only its oldest ~4s.
func TestBrokerReplayHoldsFullRing(t *testing.T) {
	const ringSz = subBuf * 4
	b := NewBroker(ringSz)
	for i := 0; i < ringSz; i++ {
		b.Publish(&tick.Tick{GameTSMillis: uint32(i)})
	}
	sub := b.Subscribe(true)
	defer sub.Close()

	got := 0
	for {
		select {
		case <-sub.C():
			got++
		default:
			if got != ringSz {
				t.Fatalf("replay delivered %d of %d ring ticks", got, ringSz)
			}
			return
		}
	}
}

func TestBrokerSlowSubscriberDoesNotBlockPublish(t *testing.T) {
	b := NewBroker(4)
	sub := b.Subscribe(false)
	defer sub.Close()

	// Overfill the subscriber buffer; Publish must never block.
	for i := 0; i < subBuf*3; i++ {
		b.Publish(&tick.Tick{})
	}
	if d := b.Dropped(); d == 0 {
		t.Fatal("expected dropped frames to be counted for a slow subscriber")
	}
}
