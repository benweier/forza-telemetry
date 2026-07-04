# Stint splits on race-state, gap, and car only

Supersedes the split axes of ADR 0006 (its discard rules stand unchanged).

A **Stint** now ends on exactly three triggers:

- a packet-arrival gap of ≥ 10 minutes (`stintGap`, up from 10 seconds — in
  active gameplay Forza streams continuously, so a gap means the game was
  closed/suspended or the network dropped)
- an **IsRaceOn flip** (gameplay ↔ menus/loading/pause)
- a **Car** change (`CarOrdinal`, with the existing zero-is-unknown handling)

`CurrentRaceTime` no longer splits. It becomes a close-time **classifier**:
a stint whose ticks ever showed `CurrentRaceTime > 0` resolves to `sprint`
(or `circuit` when the lap counter advanced); otherwise `freeroam`; IsRaceOn
false is `idle` (still discarded). "One Stint Type for the stint's whole
duration" is therefore no longer a split invariant — IsRaceOn state and Car
are; the type is a summary of what happened.

Empirical assumption this leans on: entering/leaving a structured event
passes through a loading screen that drops IsRaceOn, so races still land in
their own stints in practice. If a capture shows an event starting without
the flip, the race merges into the surrounding drive and classifies as
sprint/circuit — accepted. Tracked in docs/data-needed.md.

## Considered Options

- **Keep CurrentRaceTime as a split axis (ADR 0006)** — rejected: redundant
  with the IsRaceOn flip at event boundaries in practice, and its steady-state
  behaviour was the less-confirmed signal of the two.
- **Hysteresis/debounce on IsRaceOn** — deferred: the 2 s / 180-tick discard
  floors already absorb sub-2s flaps (they produce discarded micro-stints,
  not noise rows). Revisit if real captures show flapping that splits real
  drives.

## Consequences

- Far fewer, longer stints: brief menu visits split a drive in two (the idle
  middle is discarded), but freeroam → race → freeroam without a loading
  screen stays one stint.
- The recovery path (ADR 0011) reconstructs the same classification from
  parquet (`BOOL_OR(is_race_on)`, `BOOL_OR(current_race_s > 0)`).
- `CurrentRaceTime > 0` remains the freeroam-vs-race discriminator — just at
  classification time, not split time.
