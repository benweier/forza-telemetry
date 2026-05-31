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

### Turn detector (per ADR 0008)
- **Direction convention:** `right` if signed κ > 0 under `atan2(dx, dz)`. Verified on synthetic paths only.
- **Needs:** drive a known track with catalogued turn directions (e.g., Goliath, Mulege circuit) — confirm right/left labels match reality.
- **Threshold tuning (first-pass guesses):**
  - `κ_min`: 0.01 rad/m — minimum curvature for a candidate region.
  - `Δθ_min`: ~15° (0.26 rad) — minimum per-region accumulated heading change. Rejects swerves on straights (low net Δθ) while keeping shallow real turns.
  - `long_g_brake_threshold`: 0.2g — boundary extension backwards while still braking.
  - `long_g_accel_threshold`: 0.2g — boundary extension forwards while still accelerating out.
  - Minimum run-length per κ-region: TBD (a few resampled metres) to suppress spike noise.
- **Needs:** real race captures across varied corner shapes (fast sweepers, hairpins, chicanes) to confirm `Δθ_min` correctly distinguishes shallow real corners from in-lane corrections.
- **Stint-level identity model (per ADR 0008):** one row per (Stint × Turn). Per-Lap variation derived from the stored tick range at query time. The pre-ADR-0008 "cross-lap corner identity" problem is dissolved by this — Turn 3 is one row, regardless of how many Laps drove through it.
- **Shape classification (deferred):** chicane / hairpin / sweeper / dogleg / esses categorisation reads the stored `peak_curvature`, `peak_delta_theta`, and the sequence of adjacent Turn rows. Needs labelled training examples once Turn detection is stable.

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
