package storage

import "github.com/benweier/forza-telemetry/server/internal/tick"

// stintCategory is the per-tick split axis. Stints split when the category
// changes from tick to tick. The final stint_type stored in DuckDB is
// resolved at close time and may be a refinement of the category (e.g. a
// `race` category resolves to `sprint` or `circuit` based on observed laps).
type stintCategory uint8

const (
	categoryIdle stintCategory = iota
	categoryFreeroam
	categoryRace
)

// categorize maps a single Tick to its split category.
//
// Rules:
//   - !IsRaceOn         → idle      (menus, loading, pause)
//   - CurrentRaceTime=0 → freeroam  (active gameplay, no structured event)
//   - CurrentRaceTime>0 → race      (active timed event)
//
// CurrentRaceTime is the discriminator between freeroam and race because in
// Forza titles it counts only inside structured events; in free roam it stays
// at zero. Confirm against real captures before relying on this in shipping
// classifications.
func categorize(t *tick.Tick) stintCategory {
	if !t.IsRaceOn {
		return categoryIdle
	}
	if t.CurrentRaceTime > 0 {
		return categoryRace
	}
	return categoryFreeroam
}

// stintTypeNames is the canonical string label for each terminal stint type.
const (
	stintTypeIdle     = "idle"
	stintTypeFreeroam = "freeroam"
	stintTypeSprint   = "sprint"
	stintTypeCircuit  = "circuit"
)

// resolveStintType converts a per-tick category + observed lap range into the
// final stint_type label. `lapDelta` is `max(LapNumber) - min(LapNumber)`
// across the stint. A non-zero delta means at least one lap completed, so the
// stint is `circuit`; otherwise it is `sprint`.
func resolveStintType(c stintCategory, lapDelta uint16) string {
	switch c {
	case categoryIdle:
		return stintTypeIdle
	case categoryFreeroam:
		return stintTypeFreeroam
	case categoryRace:
		if lapDelta > 0 {
			return stintTypeCircuit
		}
		return stintTypeSprint
	}
	return ""
}
