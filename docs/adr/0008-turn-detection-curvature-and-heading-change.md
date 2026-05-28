# Turn detection from path curvature and per-region heading change

Supersedes [ADR 0007](./0007-corner-detection-curvature-and-lateral-g.md).

**Turns** (renamed from Corners) are detected at end-of-stint by combining path curvature `κ(s)` with per-region accumulated heading change `Δθ`. Lateral G no longer participates in detection — driver repositioning on a straight produces lat-G without imposing a track-defined direction change, and was a known source of false positives.

Boundaries are extended backwards into the braking zone and forwards into the exit-acceleration zone using longitudinal G thresholds (carried forward from ADR 0007). The entry / apex / exit phases of a Turn therefore include the braking and acceleration adjacent to the curvature peak.

The complement of Turns within a Stint is materialised as **Straights** — first-class sibling rows that fill every tick not covered by a Turn. A Stint with `K` Turns has exactly `K+1` Straights, including zero-length ones when a Turn's extended boundary reaches the Stint edge.

**Hot-spots** are attributed to exactly one Turn or exactly one Straight via two nullable foreign keys with an XOR check constraint.

## Considered Options

- **Curvature + sustained lateral G (the old ADR 0007 model).** Rejected: lat-G is driver-aggression-sensitive and false-positives on in-lane corrections.
- **Curvature only.** Rejected: a long shallow bend with `κ` just over threshold could trigger without meaningful direction change.
- **Cumulative `Σ|Δθ|` to bundle chicanes into one Turn.** Rejected: bundles a *classification* concern (chicane vs. simple turn) into *detection*. Detection stays geometric and dumb; classification is left to a future shape-categorisation pass that reads the sequence of detected Turns.
- **Identity model: one row per (Lap × Corner).** Rejected: the use case driving this design is "best hot-spot per place on the track within a stint", which is stint-level. Per-lap variation is derivable from the stored tick range at query time. Cuts row count substantially on multi-lap circuit stints.
- **Detection at ingest (streaming).** Rejected: detection requires resampling the full path of the stint at uniform distance, which is naturally a one-shot batch over the closed parquet — not a streaming workload. Hot-spot attribution also runs more cleanly once all hot-spots for the stint exist.
- **Lazy / request-time detection.** Rejected: inverts the DuckDB single-writer model (would put writes inside GET handlers) and adds first-request latency for marginal benefit (re-tuning thresholds is rare).
- **Implicit / derived Straights (no Straight rows).** Rejected: hot-spot attribution becomes a two-pass query that reinvents "compute the N-th gap" everywhere it's needed. Storing Straights once at aggregate time is cheaper than recomputing them on every read.

## Consequences

- Turn detection runs in `aggregateStint` after `stint_summary` / `lap_summary` / `preview_samples`, then Straights are derived from the Turn boundaries, then hot-spot rows are updated with their `turn_id` or `straight_id`.
- Detection runs on Circuit and Sprint Stints only (Freeroam continues to skip path-sample collection per existing memory policy; Idle is trivially excluded).
- A chicane is represented as two adjacent Turn rows (left + right) until a future shape classifier groups them. Each row's direction (left/right) stays unambiguous.
- The `corners` table is dropped outright. The old detector is removed. Existing aggregated stints lose their corner data; turns appear on stints aggregated under the new build (forward-only — no backfill).
- Hot-spot rows gain two nullable foreign keys (`turn_id`, `straight_id`) with a `CHECK ((turn_id IS NULL) <> (straight_id IS NULL))` constraint enforcing the XOR.
- Stored per-Turn annotations (`peak_curvature`, `peak_delta_theta`, `direction`, plus the `shape` placeholder for the future classifier) and per-Straight annotations (`distance_m`, `peak_speed_ms`) precompute commonly-accessed metrics at detection time. Matches the existing `stint_summary` / `lap_summary` precedent.
- Thresholds (`κ_min`, `Δθ_min`, `long-G` braking/acceleration cutoffs, run-length minima) are first-pass guesses tracked in `docs/data-needed.md` and refined against real captures.
