# Data Needed

Open empirical questions that block tightening defaults, confirming wire-format
hypotheses, or implementing deferred features. Each item names the data that
would resolve it and the code that depends on it.

Sessions that capture new Forza data should add notes here on what was learned
or what new questions surfaced.

---

## FH6 wire format (`server/internal/ingest/parser/fh6.go`)

### Byte 323 — trailing reserved
- **Unknown:** semantics of last packet byte.
- **In captures so far:** always `0` across 9565 race-on packets (free-roam only).
- **Needs:** structured-race captures (lap > 0), FM-style event captures, drift / off-track moments. Any non-zero value reveals what it tracks.
- **Code dep:** `decodeFH6` reads + discards. Once meaning known, add field to `tick.Tick` (additive per ADR 0003) + update decoder.

### `CarGroup` enum mapping
- **Unknown:** what each integer means (S1, A, B, C, … ?).
- **In captures so far:** only value `25` observed (single car, ordinal 3773, B-class, 4-cyl).
- **Needs:** captures with 5–10 different cars across class spectrum (D / C / B / A / S1 / S2 / X). Cross-reference `CarGroup` with the in-game class label.
- **Code dep:** `tick.CarGroup` is `int32`; consider a typed enum once mapping known.

### `SmashableVelDiff` / `SmashableMass` semantics
- **Unknown:** units, range, expected event duration.
- **In captures so far:** 10 packets with non-zero values (single collision cluster). VelDiff 0.004–0.050, Mass 0.020–0.240.
- **Needs:** multiple deliberate smashable hits at varied speeds + sizes (tiny fence, mid bollard, big tree). Capture sequences cleanly.
- **Code dep:** populated in `tick.Tick`; not yet surfaced in UI or used by detectors.

### `Power` — possible semantic shift in FH6
- **Anomaly:** large negative values (-28 kW to -35 kW) during coast / engine-braking. FH5 spec says Power is engine output watts; negatives during compression braking are plausible but the magnitude is suspect.
- **Needs:** controlled tests — known constant-speed cruise (Power should stabilise), known WOT acceleration (should match dyno-style HP curve), engine-off coast.
- **Code dep:** none directly, but UI charts (#10) need Power to be sensible.

### `Fuel` — always 1.0 in capture
- **Unknown:** does the field actually deplete, or is depletion gated by a difficulty setting?
- **Needs:** long drive with fuel-on assists disabled, or race with simulation damage on.
- **Code dep:** none yet; matters for fuel-strategy analysis if added.

### Race-only fields (BestLap / LastLap / CurrentLap / LapNumber / RacePosition)
- **In captures so far:** all zero — capture was free-roam.
- **Needs:** structured race (circuit event with ≥ 2 laps).
- **Code dep:** `resolveStintType` distinguishes sprint vs circuit by LapNumber range; corner detector runs ONLY for circuit stints. Both need confirmed race data.

---

## Parser coverage

### FM Dash (331 B)
- **Not registered.** Parser missing for Forza Motorsport 7 / FM 2023 packets.
- **Needs:** an FM capture; layout is FH5 prefix + TireWear[4] f32 + TrackOrdinal i32 at the tail (per public FM spec).
- **Code dep:** add `parser/fm.go` mirroring `fh6.go`'s structure (parser pkg already supports plug-and-play registration via `MotorsportDashSize = 331`).

### FH6 type confirmation against alternate packets
- **Hypothesis:** Sled prefix (offsets 0–231) is byte-for-byte FH5-compatible. Verified against one capture session.
- **Needs:** captures from different cars, different game modes, different FH6 builds — regression-test the golden-fixture decoder against each.
- **Code dep:** `parser/fh6_test.go` golden-fixture test pins one packet; add more as captures arrive.

---

## Detector tuning (all in `server/internal/storage/`)

### Stint categorizer (`stint_type.go`)
- **Hypothesis:** `CurrentRaceTime > 0` discriminates race vs free-roam.
- **Needs:** real race entry/exit transition — confirm CurrentRaceTime jumps to non-zero at race start and back to zero at finish.
- **Code dep:** drives every stint split + final `stint_type` label.
- **Edge case to verify:** single transient tick of opposite category causes a split today (no hysteresis). Real captures may show flapping that warrants a debounce window.

### Hot-spot thresholds (`hotspot.go`)
- **First-pass defaults:**
  - `peak_lateral_g`: trigger 0.7g / release 0.5g
  - `peak_brake`: trigger 0.5 / release 0.3
  - `top_speed`: trigger 30 m/s / release 25 m/s
- **Needs:** real race captures with rough labels — "this corner felt like 1.2g", "I floored it down the back straight". Tune so detected hot-spots match driver intuition.
- **Missing detectors:** `off_track` (boolean signal — wheel-on-rumble OR in-puddle) and `hard_landing` (vertical accel or suspension travel spike). Need captures with deliberate off-tracks and jumps to characterise the signal shapes before implementing.

### Corner detector (`corner.go`)
- **Direction convention:** `right` if signed κ > 0 under `atan2(dx, dz)`. Verified on synthetic paths only.
- **Needs:** drive a known track with cataloged corner directions (e.g., Goliath, Mulege circuit) — confirm right/left labels match reality.
- **Cross-lap corner identity matching:** today every lap re-numbers its own corners. ADR 0007 promises stable numbering across laps over the same track. Implementation needs reference-path clustering or first-lap-as-template matching.
- **Needs for cross-lap work:** multi-lap circuit captures (3+ laps on the same track in one stint) so a matching algorithm can be developed + tested.
- **Threshold tuning:** κ ≥ 0.01 rad/m, |lat G| ≥ 0.4 confirmation, brake/accel ≥ 0.2g for boundary extension — all first-pass.

---

## Operations / capture pipeline

### FH6 capture log rotation
- **Currently:** single append-only file at `<dataDir>/captures/fh6.log`. Growth ≈ 78 MB/hour at full ingest rate.
- **Needs:** decision — daily rotation? Size-capped (e.g., 100 MB then rotate)? Drop capture once layout is fully verified?
- **Code dep:** `parser/fh6.go logPacket()` writes via passed `io.Writer`. Listener opens the file in `NewListener`. A rotating writer (e.g., `lumberjack`) can be swapped in.

### Multi-hour stints
- **Memory pressure:** `stintState.pathSamples` at ~32B × 60Hz × 3600s ≈ 7 MB / hour. OK for short stints but unbounded.
- **Needs:** check whether real free-roam sessions exceed an hour without category change. If so, decide on eviction or external-spill strategy.
- **Code dep:** `writer.go` accumulates `pathSamples` only for race-category stints today, but the same memory bound applies.

---

## How to use this doc

1. When picking up a session, scan this file for items aligned with the next task — especially threshold tuning and FH6 wire-format gaps.
2. When you capture new Forza data, update or remove the relevant entries.
3. New unknowns surfaced during implementation should be added here, not buried in code comments.
