# Data Needed

Open empirical questions that block tightening defaults, confirming wire-format
hypotheses, or implementing deferred features. Each item names the data that
would resolve it and the code that depends on it.

Sessions that capture new Forza data should add notes here on what was learned
or what new questions surfaced.

---

## FH6 wire format (`server/internal/ingest/parser/fh6.go`)

### Byte 323 — trailing reserved
- **Resolved (2026-05-31):** still `0` across **716,338 race-on packets** from a real circuit+sprint capture (multi-car field, laps 0–3). Confirmed reserved padding, not a field. Decoder reads + discards it with a comment to that effect; not added to `tick.Tick`.

### Tail input bytes — DrivingLine / AIBrakeDifference (offsets 321, 322)
- **Resolved (2026-05-31):** previously read-and-discarded in `decodeHorizonTail`. Now decoded into `tick.DrivingLine` and `tick.AIBrakeDifference` (both `int8`, additive per ADR 0003).
- **Observed ranges (716k race packets):** DrivingLine `-1..65` (a signed index/angle, NOT a 0..1 normalized fraction despite Forza's "Normalized…" wire name); AIBrakeDifference `-127..122` (signed; player-vs-AI braking delta).
- **Open:** exact units/semantics still uninterpreted — values are stored raw. Cross-reference DrivingLine against on-screen suggested-line state, and AIBrakeDifference against a known AI-comparison scenario, to pin meaning.

### `CarGroup` enum mapping
- **Unknown:** what each integer means (S1, A, B, C, … ?).
- **In captures so far:** single-car free-roam showed only `25`; the 2026-05-31 multi-car race capture shows distinct values `{12, 13, 25, 26, 31, 33, 35, 38, 48}`.
- **Needs:** cross-reference each value with the in-game class/division label per car (D / C / B / A / S1 / S2 / X). Range is now known; the mapping is not.
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
- **Resolved (2026-05-31):** confirmed against a real circuit+sprint capture (716k race-on packets):
  - `RacePosition` u8: `0..10` (an 11-car field) — a genuine race-vs-freeroam signal.
  - `LapNumber` u16: reached `3` on the circuit race — confirms sprint/circuit split logic.
  - `BestLap`/`LastLap` f32: `0` until a lap completes, then up to `45.6`s.
  - `CurrentLap` f32: `0..233`s within a lap; `CurrentRaceTime` f32 nonzero in 716321/716338 race packets — the race discriminator holds.
- **Follow-up (deferred):** these could become *additional* recording-confidence gates (e.g. require `RacePosition > 0`), but solo time-trial / rivals modes may report `0`; confirm those modes before gating on position.

### `Gear` encoding (uint8)
- **Resolved (2026-06-01):** Forza sends `Gear` as a `uint8`: **0 = reverse**, `1..n` = forward gears. No distinct neutral value is signalled. Confirmed live — reversing reports gear `0`. (It bit us twice: the gear had been mislabelled "N" for `0`, with a separate bogus "11 = R" guess in the HUD.)
- **Code dep:** client `gearLabel` (`client/src/utils/format.ts`, shared by the HUD + WebGPU instrument view) maps `0 → "R"`, else the number. Server `GearShift` enrich already treats `0` as a non-forward gear (`prev.Gear != 0 && t.Gear != 0`).
- **Open:** whether any title/mode reports a distinct neutral (and what value) — unconfirmed; treat `0` as reverse until a neutral capture says otherwise.

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

### Stint classifier (`stint_type.go`, ADR 0013)
- **Partially confirmed (2026-05-31):** `CurrentRaceTime` was nonzero in 716,321/716,338 race-on packets from the real circuit+sprint capture — the `> 0` race-vs-freeroam discriminator holds while a race is running. As of ADR 0013 it *classifies* the closed stint (`sawRace`) but never splits.
- **Assumption to confirm:** entering/leaving a structured event passes through a loading screen that drops `IsRaceOn` — that flip is what puts races in their own stints now. If a capture shows an event starting with no flip, the race merges into the surrounding drive (classified sprint/circuit); decide then whether that's acceptable.
- **Edge case to verify:** a single transient `IsRaceOn` flip splits with no hysteresis; sub-2s flaps become discarded micro-stints (harmless), but flapping longer than 2s would split real drives. Real captures may warrant a debounce window.
- **Accepted risk:** `GameTSMillis` wraps its uint32 at ~49.7 days of continuous game uptime, which the session boundary (ADR 0012) would misread as a game relaunch. Implausible; noted for completeness.

---

## Operations / capture pipeline

### FH6 capture log rotation
- **Resolved:** `lumberjack` rotation shipped (`listener.go` opens the capture log through a `lumberjack.Logger`); the file can no longer grow unbounded.

## Live HUD visualisation thresholds (client)

Added 2026-06-01 for the `/live` HUD car diagram, dyno curve, and boost gauge. All
are placeholders in `client/src/components/hud/tire-scale.ts` / `engine.ts` and need
a real capture to confirm. Update or remove these entries when resolved.

### Tire temp (`tt`) unit + grip-window thresholds
- **Currently:** `TEMP_THRESHOLDS` in `tire-scale.ts` assumes a Celsius-ish scale (cold 50, optimal 70–95, hot 110, overheat 120) driving the muted→success→warning→danger heat fill.
- **Needs:** confirm the wire unit (°C / °F) and tune the grip-window band edges against a capture where tire state is known.
- **Code dep:** `heatScaleColor()` + `TEMP_THRESHOLDS` — band edges are the only tuning surface.

### Boost (`bo`) unit + display range
- **Currently:** `BoostGauge` renders raw `bo` across a placeholder −1..2 range; zero is the vacuum/positive pivot.
- **Needs:** confirm whether the unit is bar / PSI / atm and fix `BOOST_MIN`/`BOOST_MAX` and the zero/vacuum convention.
- **Code dep:** `BoostGauge.tsx` `BOOST_MIN`/`BOOST_MAX`; `boostFraction()` in `engine.ts`.

### Slip-ratio (`tsr`) lockup/spin + slip-angle (`tsa`) scale
- **Currently:** `TSR_SLIP_THRESHOLD = 0.15` gates the ring LOCK/SPIN tags; `SLIP_ANGLE_MAX = 0.3` rad sets full-length slip-angle arrows.
- **Needs:** confirm against a capture with deliberate lockup / wheelspin and measured slip angles.
- **Code dep:** `classifyWheelSlip()` + `slipAngleTick()` in `tire-scale.ts`.

---

## How to use this doc

1. When picking up a session, scan this file for items aligned with the next task — especially threshold tuning and FH6 wire-format gaps.
2. When you capture new Forza data, update or remove the relevant entries.
3. New unknowns surfaced during implementation should be added here, not buried in code comments.
