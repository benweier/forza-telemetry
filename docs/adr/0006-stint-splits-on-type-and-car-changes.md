# Stint boundaries split on Stint Type and Car changes

A **Stint** ends on any of: (a) packet-arrival gap of ≥ 10 seconds, (b) **Stint Type** transition (e.g. free-roam → race start), (c) **Car** change. Stints shorter than 2 seconds are discarded as noise.

This guarantees every Stint has exactly one Stint Type and one Car for its full duration — Stint Type and Car are invariants of the Stint, not properties that vary inside it.

## Considered Options

- Gap-only boundaries with mixed-type Stints — rejected: "show me my races" becomes a sub-range query rather than a filter; per-event analysis is harder.
- Gap-only + sub-phases inside a Stint — rejected: introduces a second axis of segmentation (Stint contains Phases contains Laps) without enough payoff.

## Consequences

- Stints are cheap; expect ~2–3× more Stints than gap-only would produce. Fine — Stints are metadata, not bulk data.
- A 10s gap threshold lets brief map / menu checks fall inside the same Stint; longer pauses end it.
