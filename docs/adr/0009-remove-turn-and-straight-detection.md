# Remove Turn and Straight detection

Supersedes [ADR 0008](./0008-turn-detection-curvature-and-heading-change.md) (and transitively [ADR 0007](./0007-corner-detection-curvature-and-lateral-g.md)).

Turn detection (curvature + per-region heading change) and the derived Straight
segmentation are removed entirely. The per-place catalogue they fed — the
"Turns" list on the historical stint view — is gone, along with the detector,
its storage tables, the REST endpoints, and the client query/types/UI.

## Context

Turns/Straights existed to give each curve a stable per-place identity (a
catalogue panel today, shape classification and Comparison anchoring later).
In practice the panel earned little: the channel-coloured **Track Path** on the
mini-map already shows *where* things happen along a stint, and the detector
carried real cost — synthetic-only threshold tuning (κ_min, Δθ_min, boundary
extension), a curvature/heading geometry pass over every race stint's path
samples, two DB tables with their own lifecycle, and a chunk of UI. The
identity was never exercised by a shipped Comparison feature. It was complexity
without a consumer.

## Decision

Strip it end to end:

- **Server:** delete `storage/turn.go`, `storage/straight.go`, `storage/geometry.go`
  (curvature/resample/boundary math — used only by the detector; the Track Path
  endpoint reads positions straight from Parquet and is untouched) and their
  tests. Remove the detection block + `insertTurns`/`insertStraights` from
  `aggregate.go`, the path-sample collection from `writer.go`, the `turns` /
  `straights` DDL from `schema.go`, those tables from the cleanup cascade, and
  the `/stints/{id}/turns` and `/stints/{id}/straights` routes + handlers.
- **Client:** remove the Turn/Straight valibot schemas, the `turnsQuery` /
  `straightsQuery` factories, and the `TurnsCard` from the stint view.
- **Docs:** prune the Turn/Straight glossary entries and the relationships line
  from `CONTEXT.md`, the Turn-detector tuning section from `data-needed.md`, and
  the stale `/corners` block from `api.md`.

## Consequences

- The **Tick** schema (ADR 0003) is unaffected — Turns/Straights were always a
  separate sub-resource, never Tick fields.
- Older databases carry leftover `turns` / `straights` tables (and `hot_spots`,
  dropped earlier) whose foreign keys onto `stints` would block stint/session
  deletion. `dropLegacyTables` removes them at startup — idempotent, and a no-op
  on a current DB. (The first cut of this ADR called them "harmless orphans";
  they were not — the later delete feature surfaced the blocked FK.)
- If per-place identity is ever wanted again, it returns as a fresh ADR built on
  whatever Comparison actually needs, rather than carried speculatively.
