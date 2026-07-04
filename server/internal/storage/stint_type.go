package storage

// Stint splitting (ADR 0013) uses exactly three triggers: a packet gap, an
// IsRaceOn flip, and a Car change. CurrentRaceTime no longer splits — it only
// classifies the closed stint. Race entry/exit normally passes through a
// loading screen that drops IsRaceOn, so structured events still land in
// their own stints; see docs/data-needed.md for the confirmation status of
// that assumption.

// stintTypeNames is the canonical string label for each terminal stint type.
const (
	stintTypeIdle     = "idle"
	stintTypeFreeroam = "freeroam"
	stintTypeSprint   = "sprint"
	stintTypeCircuit  = "circuit"
)

// resolveStintType classifies a closed stint from what its ticks showed.
//
//   - raceOn is the stint's IsRaceOn (uniform per stint — it's a split axis)
//   - sawRace is whether any tick had CurrentRaceTime > 0 (the freeroam-vs-
//     race discriminator; confirmed against 716k race-on packets)
//   - lapDelta is max(LapNumber) − min(LapNumber); non-zero means at least
//     one completed lap, so `circuit` rather than `sprint`
func resolveStintType(raceOn, sawRace bool, lapDelta uint16) string {
	if !raceOn {
		return stintTypeIdle
	}
	if !sawRace {
		return stintTypeFreeroam
	}
	if lapDelta > 0 {
		return stintTypeCircuit
	}
	return stintTypeSprint
}
