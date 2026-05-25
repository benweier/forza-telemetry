# Superset Tick schema, additive evolution only

The canonical **Tick** schema is the union of every field across every supported Forza game version. Fields not present in a given version are stored as NULL (cheap in Parquet's columnar layout). Each Tick carries `gameVersion` and `packetVariant` metadata columns so per-version queries remain trivial.

The schema evolves additively only — new game versions may add columns; existing columns never change meaning or get repurposed.

## Considered Options

- Per-game schemas (FH5 type vs FH6 type) — rejected: forks the type system through Go, the API, and the React frontend; complicates Comparison across versions.
- Lowest-common-denominator schema — rejected: discards FH6-only fields permanently.

## Consequences

- Parser is a pluggable interface keyed by inbound packet size detected at the UDP listener.
- New game versions can ship their parser without storage migration.
- Frontend renders fields conditionally on presence (`!= null`); never branches on `gameVersion`.
