# XDG directories for data and config (incl. on macOS)

Persistent data lives at `$XDG_DATA_HOME/forza-telemetry/` (default `~/.local/share/forza-telemetry/`) and config at `$XDG_CONFIG_HOME/forza-telemetry/` (default `~/.config/forza-telemetry/`), on both macOS and Linux.

This is **non-standard on macOS** — the platform convention would be `~/Library/Application Support/forza-telemetry/`. We deviate deliberately for cross-platform consistency: the same paths work on the macOS dev/deploy host today and on a Linux server tomorrow without code changes.

## Consequences

- macOS users (including the primary user) won't find data in the platform-conventional location.
- Time Machine still backs up `~/.local/share/` by default (it backs up the home dir).
- The Parquet hot/cold tree and DuckDB DB live under the data dir; moving deployments is a single directory copy.
