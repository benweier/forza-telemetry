package storage

import (
	"fmt"

	"github.com/benweier/forza-telemetry/server/internal/tick"
)

// hotSpotType is the canonical label for each detector kind. Stored verbatim
// in the hot_spots.type column.
type hotSpotType string

const (
	hotSpotPeakLateralG hotSpotType = "peak_lateral_g"
	hotSpotPeakBrake    hotSpotType = "peak_brake"
	hotSpotTopSpeed     hotSpotType = "top_speed"
)

// hotSpotCandidate is one detected region: the tick range during which the
// metric stayed above the trigger threshold, plus the peak observed inside it.
type hotSpotCandidate struct {
	Type      hotSpotType
	StartNS   int64
	EndNS     int64
	PeakNS    int64
	PeakValue float32
	Label     string
}

// detectorConfig defines a single metric detector. Hot-spots are emitted as
// event-style regions: enter when the extracted metric crosses triggerHi,
// exit when it falls below releaseLo. The hysteresis gap between thresholds
// prevents flapping on noisy signals. Peak value + tick are recorded
// continuously while active.
type detectorConfig struct {
	typ       hotSpotType
	triggerHi float32
	releaseLo float32
	extract   func(*tick.Tick) float32
	label     func(value float32) string
}

// newDefaultDetectors returns a fresh slice of detectors with default config.
// Each stint gets its own instances — state is per-stint.
func newDefaultDetectors() []*peakDetector {
	out := make([]*peakDetector, 0, len(defaultDetectorConfigs))
	for _, cfg := range defaultDetectorConfigs {
		out = append(out, newPeakDetector(cfg))
	}
	return out
}

// defaultDetectorConfigs are the three hot-spot kinds detected at ingest.
// Thresholds are first-pass defaults tuned by hand against captured FH6 data;
// they will likely need adjustment after analysing the first few weeks of
// real stints.
var defaultDetectorConfigs = []detectorConfig{
	{
		typ:       hotSpotPeakLateralG,
		triggerHi: 0.7,
		releaseLo: 0.5,
		extract:   func(t *tick.Tick) float32 { return absF32(t.LateralG) },
		label:     func(v float32) string { return fmt.Sprintf("%.1fG lateral", v) },
	},
	{
		typ:       hotSpotPeakBrake,
		triggerHi: 0.5,
		releaseLo: 0.3,
		extract:   func(t *tick.Tick) float32 { return t.BrakePct },
		label:     func(v float32) string { return fmt.Sprintf("%.0f%% brake", v*100) },
	},
	{
		typ:       hotSpotTopSpeed,
		triggerHi: 30, // ~108 km/h — only emit "top speed" regions above this
		releaseLo: 25,
		extract:   func(t *tick.Tick) float32 { return t.Speed },
		label:     func(v float32) string { return fmt.Sprintf("%.0f km/h", v*3.6) },
	},
}

// peakDetector tracks one metric. Stateful across ticks within a stint; reset
// implicitly when the owning Writer rotates to a new stint.
type peakDetector struct {
	cfg     detectorConfig
	active  bool
	peak    float32
	peakNS  int64
	startNS int64
	found   []hotSpotCandidate
}

func newPeakDetector(cfg detectorConfig) *peakDetector {
	return &peakDetector{cfg: cfg}
}

func (d *peakDetector) feed(t *tick.Tick) {
	v := d.cfg.extract(t)
	if !d.active {
		if v >= d.cfg.triggerHi {
			d.active = true
			d.peak = v
			d.peakNS = t.ServerRecvNS
			d.startNS = t.ServerRecvNS
		}
		return
	}
	if v > d.peak {
		d.peak = v
		d.peakNS = t.ServerRecvNS
	}
	if v < d.cfg.releaseLo {
		d.found = append(d.found, d.emit(t.ServerRecvNS))
		d.active = false
	}
}

// flush returns all completed hot-spots and, if the detector is still in an
// active region (e.g. the stint ends mid-peak), closes the open region with
// closeNS as the EndNS. Callers must invoke flush exactly once per stint.
func (d *peakDetector) flush(closeNS int64) []hotSpotCandidate {
	out := d.found
	d.found = nil
	if d.active {
		out = append(out, d.emit(closeNS))
		d.active = false
	}
	return out
}

func (d *peakDetector) emit(endNS int64) hotSpotCandidate {
	return hotSpotCandidate{
		Type:      d.cfg.typ,
		StartNS:   d.startNS,
		EndNS:     endNS,
		PeakNS:    d.peakNS,
		PeakValue: d.peak,
		Label:     d.cfg.label(d.peak),
	}
}

func absF32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
