# Stint boundaries split on Stint Type and Car changes

> **Superseded by ADR 0013 (split axes only):** stints now split on a
> 10-minute gap, an IsRaceOn flip, or a Car change — CurrentRaceTime
> classifies at close instead of splitting. The discard rules in the revision
> below remain in force.

> **Revision (post-implementation):** the discard criteria were widened beyond
> the original sub-2s rule. A closed Stint is now also discarded if it carries
> fewer than `minTicks` samples (default 180 ≈ 3s at 60Hz — a data-density floor
> independent of wall-clock, since a 2s+ stint can still arrive thin on packet
> loss), if its **Stint Type** is `idle` (menus / loading / pause carry no
> analysable telemetry), or if it never saw a real **Car** (`car_ordinal` stays
> 0 — a session-opened-but-no-gameplay artifact). Splitting still happens on
> every type/car transition
> as below; idle stints are simply not *persisted*. A one-time idempotent
> startup sweep removes idle / no-car stints (plus their child rows and Parquet
> files) left in the DB by older builds. See `storage/cleanup.go` and
> `discardCause` in `storage/writer.go`.

A **Stint** ends on any of: (a) packet-arrival gap of ≥ 10 seconds, (b) **Stint Type** transition (e.g. free-roam → race start), (c) **Car** change. Stints shorter than 2 seconds are discarded as noise.

This guarantees every Stint has exactly one Stint Type and one Car for its full duration — Stint Type and Car are invariants of the Stint, not properties that vary inside it.

## Considered Options

- Gap-only boundaries with mixed-type Stints — rejected: "show me my races" becomes a sub-range query rather than a filter; per-event analysis is harder.
- Gap-only + sub-phases inside a Stint — rejected: introduces a second axis of segmentation (Stint contains Phases contains Laps) without enough payoff.

## Consequences

- Stints are cheap; expect ~2–3× more Stints than gap-only would produce. Fine — Stints are metadata, not bulk data.
- A 10s gap threshold lets brief map / menu checks fall inside the same Stint; longer pauses end it.
