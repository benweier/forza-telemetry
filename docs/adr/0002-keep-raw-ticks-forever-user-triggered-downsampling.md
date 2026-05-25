# Keep raw Ticks forever; downsampling is user-triggered

Every **Tick** is persisted at full ingest rate and retained indefinitely by default. **Sessions** are never auto-downsampled. The UI may recommend downsampling for unpinned Sessions older than 10 days, but the operation is always explicit and irreversible. **Pinned** Sessions are exempt from downsampling recommendations.

Reasoning: Forza Data Out is non-reproducible — you cannot re-run a session. Storage is cheap; lost fidelity is permanent. Putting the downsample trigger in the user's hands (with a UI nudge) matches the value asymmetry.

## Consequences

- Storage grows roughly linearly with play time (~1.7 GB/day of continuous play at full rate).
- The "downsample now" action is a first-class UI operation and a first-class backend job.
- **Snapshots** survive downsampling of their source Session — this is the durable record for critical moments even after raw is reduced.
