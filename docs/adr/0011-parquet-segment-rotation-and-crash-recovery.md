# Parquet segment rotation and crash recovery

A Parquet file is only readable once its footer is written at close. The
original one-file-per-Stint layout therefore had a durability hole: a crash or
power loss mid-Stint left a footerless file, and the startup sweep deleted
both the row and the file — hours of driving lost, for a tool whose whole
premise (ADR 0002) is that raw ticks are kept forever.

Each Stint is now a **directory of segment files** (`hot/<session>/<stint>/
0001.parquet`, `0002.parquet`, …). The Writer closes the current segment
(footer + fsync — durable) and opens the next whenever the segment spans
`segmentRotateEvery` of tick time (5 minutes). `stints.parquet_path` stores
the glob `<dir>/*.parquet`; DuckDB's `read_parquet()` accepts globs natively,
so the tick-series, path, and aggregation readers are unchanged. Every query
already orders by `server_recv_ns`, so cross-file ordering is a non-issue.

At startup, **recovery** (`storage/recover.go`) runs before the polluted-stint
sweep: for each Stint with `ended_at_ns IS NULL`, unreadable segments (the one
open at the crash) are deleted, and if durable segments remain the row is
finalized exactly as `closeStint` would have — same fields (reconstructed by
SQL over the segments; valid because a Stint has one category and one Car for
its whole span, ADR 0006), same discard thresholds, same aggregation. A crash
now costs at most the open segment (≤ 5 minutes), not the Stint.

## Considered Options

- **Row-group flush + footer reconstruction** — recover the footerless file
  itself by re-deriving the footer from its row groups. Rejected: deep
  format-level surgery for the same outcome rotation gets with `os.Create`.
- **Rotate by closing the Stint** (max-duration split) — rejected: turns one
  long drive into many Stints, breaking the Stint's domain meaning (ADR 0006)
  for a storage concern.
- **Keep one file, accept the loss** — rejected: contradicts ADR 0002's
  retention premise; this hole was found by a crash-repro during review.

## Consequences

- Databases from pre-rotation builds hold single-file paths; every consumer
  (readers via `read_parquet`, deletion via `removeParquet`) handles both
  layouts. Legacy crashed stints are still unrecoverable and fall through to
  the sweep as before.
- The Parquet file path stays server-internal (never exposed over the API),
  so segmentation is invisible to clients.
- Directory fsync after segment close is not performed; on some filesystems a
  crash within seconds of a rotation can lose the just-closed segment's
  directory entry. Accepted as negligible for this tool.
