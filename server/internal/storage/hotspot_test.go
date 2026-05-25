package storage

import (
	"testing"

	"github.com/benweier/forza-telemetry/server/internal/tick"
)

// lateralGCfg is the detector under test in unit tests below.
var lateralGCfg = detectorConfig{
	typ:       hotSpotPeakLateralG,
	triggerHi: 0.7,
	releaseLo: 0.5,
	extract:   func(t *tick.Tick) float32 { return absF32(t.LateralG) },
	label:     func(v float32) string { return "lat" },
}

func feedLateralG(d *peakDetector, values []float32) {
	for i, v := range values {
		d.feed(&tick.Tick{LateralG: v, ServerRecvNS: int64(i) * 16_000_000})
	}
}

func TestPeakDetectorSinglePeak(t *testing.T) {
	d := newPeakDetector(lateralGCfg)
	feedLateralG(d, []float32{0.1, 0.3, 0.6, 0.9, 1.5, 1.2, 0.8, 0.4, 0.2})
	got := d.flush(0)
	if len(got) != 1 {
		t.Fatalf("want 1 hot-spot, got %d", len(got))
	}
	if got[0].PeakValue != 1.5 {
		t.Errorf("PeakValue: want 1.5 got %v", got[0].PeakValue)
	}
	// Peak at index 4 (value 1.5), start at index 3 (first >= 0.7), end at
	// index 7 (first < 0.5).
	if got[0].StartNS != 3*16_000_000 {
		t.Errorf("StartNS: want %d got %d", 3*16_000_000, got[0].StartNS)
	}
	if got[0].PeakNS != 4*16_000_000 {
		t.Errorf("PeakNS: want %d got %d", 4*16_000_000, got[0].PeakNS)
	}
	if got[0].EndNS != 7*16_000_000 {
		t.Errorf("EndNS: want %d got %d", 7*16_000_000, got[0].EndNS)
	}
}

func TestPeakDetectorTwoPeaks(t *testing.T) {
	d := newPeakDetector(lateralGCfg)
	feedLateralG(d, []float32{
		0.1, 0.8, 1.0, 0.6, 0.2, // peak 1
		0.4, 0.9, 1.3, 0.5, 0.1, // peak 2
	})
	got := d.flush(0)
	if len(got) != 2 {
		t.Fatalf("want 2 hot-spots, got %d", len(got))
	}
	wantPeaks := []float32{1.0, 1.3}
	for i, c := range got {
		if c.PeakValue != wantPeaks[i] {
			t.Errorf("hot-spot %d PeakValue: want %v got %v", i, wantPeaks[i], c.PeakValue)
		}
	}
}

func TestPeakDetectorBelowThreshold(t *testing.T) {
	d := newPeakDetector(lateralGCfg)
	feedLateralG(d, []float32{0.0, 0.3, 0.5, 0.6, 0.4, 0.1})
	got := d.flush(0)
	if len(got) != 0 {
		t.Errorf("want 0 hot-spots, got %d", len(got))
	}
}

func TestPeakDetectorHysteresisNoFlap(t *testing.T) {
	// Signal hovers between releaseLo (0.5) and triggerHi (0.7) — must NOT
	// emit until it crosses both thresholds in sequence.
	d := newPeakDetector(lateralGCfg)
	feedLateralG(d, []float32{0.6, 0.55, 0.6, 0.55, 0.5})
	got := d.flush(0)
	if len(got) != 0 {
		t.Errorf("want 0 hot-spots (sub-trigger), got %d: %+v", len(got), got)
	}
}

func TestPeakDetectorFlushClosesActive(t *testing.T) {
	d := newPeakDetector(lateralGCfg)
	// Ends still above release — flush must close it.
	feedLateralG(d, []float32{0.2, 0.8, 1.1, 0.9})
	got := d.flush(99_999_999)
	if len(got) != 1 {
		t.Fatalf("want 1 hot-spot, got %d", len(got))
	}
	if got[0].EndNS != 99_999_999 {
		t.Errorf("EndNS: want close-time %d got %d", 99_999_999, got[0].EndNS)
	}
	if got[0].PeakValue != 1.1 {
		t.Errorf("PeakValue: want 1.1 got %v", got[0].PeakValue)
	}
}

func TestPeakDetectorAbsoluteLateralG(t *testing.T) {
	// Negative lateral G (left corner) must still register.
	d := newPeakDetector(lateralGCfg)
	feedLateralG(d, []float32{0.0, -0.8, -1.2, -0.9, -0.3})
	got := d.flush(0)
	if len(got) != 1 {
		t.Fatalf("want 1 hot-spot, got %d", len(got))
	}
	if got[0].PeakValue != 1.2 {
		t.Errorf("PeakValue: want 1.2 (abs) got %v", got[0].PeakValue)
	}
}
