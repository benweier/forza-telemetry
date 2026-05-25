# Embedded DuckDB + Parquet for telemetry storage

We use DuckDB embedded in the Go server process (via `go-duckdb`) over Parquet files on disk for all telemetry storage. Hot **Stints** are append-only Parquet in a `hot/` directory; **Downsampled** Stints are rewritten Parquet in a `cold/` directory. There is no separate database process and no relational TSDB (Timescale, Influx, QuestDB).

## Considered Options

- TimescaleDB hot + Parquet cold — rejected: separate Postgres process, heavier ops for single-user macOS deployment, no real win over DuckDB for our query patterns.
- QuestDB hot + Parquet cold — rejected: still a separate process; relational metadata would need a sidecar SQLite, doubling moving parts.
- InfluxDB — rejected: same separate-process cost; query language drift across versions.

## Consequences

- Single binary deploys the entire backend.
- The "downsample" action is just a Parquet rewrite; no cross-engine ETL.
- Live-stream consumers read from an in-memory ring buffer, never from DuckDB — concurrent live reads do not contend with the ingest write path.
- `cgo` is required in the Go build (DuckDB dependency); accepted.
