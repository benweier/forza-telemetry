# Forza Telemetry

Ingests, stores, and visualizes the UDP "Data Out" telemetry stream from Forza Horizon (primarily FH6, with FH5 support). A Go server on a networked PC captures the stream, persists it, and serves both a live realtime view and a historical scrubable view to a React frontend.

## Language

**Session**:
The outer container for capture: one continuous sitting of Data Out packets. Born from data (the row is created on the first packet; its ID is that packet's arrival time) and ended by data — a silence of ≥ 1 hour, or the game's own clock (`GameTSMillis`) jumping backwards (a relaunch). A Session whose Stints were all discarded is deleted rather than kept empty. See ADR 0012.
_Avoid_: Run, capture, recording.

**Stint**:
A contiguous span of in-car driving inside a **Session**, bounded by exactly three triggers (ADR 0013): (a) a packet-arrival gap of ≥ 10 minutes, (b) an `IsRaceOn` flip (gameplay ↔ menus/loading/pause), or (c) a change in **Car**. Stints shorter than 2 seconds or thinner than 180 ticks are discarded as noise; so are `idle` Stints and Stints that never saw a real **Car** (`car_ordinal` 0) — only `freeroam` / `sprint` / `circuit` Stints with a known **Car** are persisted (ADR 0006's discard rules). Has exactly one `IsRaceOn` state and one **Car** for its entire duration; its **Stint Type** is a close-time classification, not a split invariant. A **Session** contains many **Stints**.
_Avoid_: Segment, drive, leg.

**Lap**:
A game-identified timed lap within a `circuit` **Stint** — a sub-range of that Stint's **Ticks**, never a kind of Stint itself. Materialized today only as a per-lap aggregate row (`lap_summary`: lap number, time, peaks); the underlying tick range is derivable by filtering the Stint's Ticks on `lap_number` but is not stored as a first-class range. Not every **Stint** has **Laps** — free-roam and sprint **Stints** do not.
_Avoid_: Circuit, loop.

**Tick**:
A single telemetry sample at the game's emission rate (~60 Hz). The canonical, enriched form of one inbound UDP **Packet**: parsed into the superset schema, with derived fields (lateral G, throttle %, gear-shift events, distance-this-Lap, etc.) computed at ingest. Persisted to storage and broadcast on the live channel in this enriched form — never as raw bytes. The atomic data unit; everything else is an aggregate over **Ticks**.
_Avoid_: Frame, sample.

**Packet**:
The raw UDP datagram as it arrives from Forza's Data Out. Strictly the wire-format object; parsed once at the UDP listener boundary into a **Tick** and then discarded. Only used as a term in code dealing with the network layer or the binary parser.
_Avoid_: Tick, frame, message.

**Stint Type**:
An automatically-assigned classification of a **Stint**, resolved at close from what its Ticks showed (ADR 0013): `idle` (IsRaceOn false — paused/menu/loading; never persisted), `freeroam` (driving, no race time observed), `sprint` (saw `CurrentRaceTime > 0`, no completed laps), `circuit` (saw race time and the lap counter advanced). A race entered without an `IsRaceOn` flip merges into the surrounding drive and classifies as sprint/circuit — in practice event loading screens flip `IsRaceOn`, so races land in their own Stints.
_Avoid_: Mode, category (those are for user-applied tags).

**Tag** _(planned — not yet implemented)_:
A user-applied label attached to a **Stint** (or possibly a **Session** or **Lap**) after capture, used to enrich what telemetry cannot tell us. Free-form by default (e.g. `"Goliath"`, `"PR Stunt - Speed Trap"`) with optional structured tag types reserved for future use (event name, event type, tuning notes, weather, surface, etc.).
_Avoid_: Label, annotation.

**Car**:
The vehicle being driven during a **Stint**, identified from telemetry by `CarOrdinal` (Forza's unique vehicle ID) plus `CarClass` and `CarPerformanceIndex`. Auto-derived; not user-tagged.
_Avoid_: Vehicle.

**Pinned**:
A **Session** state that protects it from being downsampled, regardless of age. Set/unset by the user from the UI.
_Avoid_: Locked, starred, favorited.

**Downsampled** _(endpoint stubbed — flag + 501 exist, the Parquet rewrite job does not)_:
A **Session** that has been irreversibly reduced from full-rate **Tick** capture to a lower rate (target rate TBD, e.g. 10 Hz) to reclaim space. The aggregates and 1 Hz preview series are preserved; only the raw **Tick** stream is reduced. Triggered by explicit user action; the UI may recommend it for unpinned **Sessions** older than 10 days but never performs it automatically.
_Avoid_: Compressed, archived, decimated.

**Bookmark** _(planned — not yet implemented)_:
A user-marked moment placed on a single **Tick** during live or scrub playback, optionally with a free-form note. Distinct from a **Snapshot** (which is a durable capture).
_Avoid_: Marker, flag, pin.

**Snapshot** _(planned — not yet implemented; note ADR 0002's "Snapshots survive downsampling" durability story depends on this existing before downsampling ships)_:
A durable capture of telemetry state at a chosen moment (one **Tick** or a small window around it), saved as a first-class record that survives **Downsampling** of its source **Session**. Can be created from a **Bookmark** or any arbitrary scrub position. Supports side-by-side comparison and export (PNG / JSON / CSV).
_Avoid_: Capture, frame, freeze.

**Track Path**:
The spatial trajectory of a **Stint**, derived from each **Tick**'s `PositionX/Y/Z`. Rendered as a polyline in the mini-map view, optionally coloured by a per-**Tick** channel (speed, brake force, lateral G). Day-one renderer plots raw world coordinates with auto-fit bounds; a future enhancement may overlay onto Forza region map tiles.
_Avoid_: Route, trace, line.

**Comparison** _(planned — not yet implemented; needs first-class Lap tick ranges that the schema doesn't have yet)_:
A user-assembled set of 2-6 comparable units (**Laps** or **Snapshots**) rendered together: time-series channels overlaid on shared charts and **Track Paths** overlaid on the mini-map (ghost-car style, color-coded). Time-series x-axis is always **cumulative distance from entity start**, not elapsed time, to avoid misleading visual offsets when one entity is slower than another. Comparison across different **Cars** or different auto-classified tracks is permitted with a UI warning, never blocked.
_Avoid_: Overlay, diff, vs-mode.

## Relationships

- A **Session** contains one or more **Stints**
- A **Stint** has exactly one **Stint Type** (auto-assigned)
- A **Stint** has zero or more **Tags** (user-applied)
- A **Stint** contains zero or more **Laps** (zero for free-roam or sprint)
- A **Stint** is composed of a contiguous run of **Ticks**
- A **Stint** references exactly one **Car** (the car driven for its duration)
- A **Lap** references a sub-range of its parent **Stint**'s **Ticks**
- A **Stint** has zero or more **Bookmarks** (user-marked during playback)
- A **Snapshot** references one **Tick** (and optionally a window) and persists independently of its source **Stint**'s downsampling state
