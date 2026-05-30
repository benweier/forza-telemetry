# Forza Telemetry

Ingests, stores, and visualizes the UDP "Data Out" telemetry stream from Forza Horizon (primarily FH6, with FH5 support). A Go server on a networked PC captures the stream, persists it, and serves both a live realtime view and a historical scrubable view to a React frontend.

## Language

**Session**:
The outer container for one continuous arrival of Data Out packets — typically the lifetime of a single game launch.
_Avoid_: Run, capture, recording.

**Stint**:
A contiguous span of in-car driving inside a **Session**, bounded by: (a) a packet-arrival gap of ≥ 10 seconds, (b) a change in **Stint Type** (e.g. free-roam → race start), or (c) a change in **Car**. Stints shorter than 2 seconds are discarded as noise; so are `idle` Stints and Stints that never saw a real **Car** (`car_ordinal` 0) — only `freeroam` / `sprint` / `circuit` Stints with a known **Car** are persisted (see ADR 0006). Has exactly one **Stint Type** and one **Car** for its entire duration. A **Session** contains many **Stints**.
_Avoid_: Segment, drive, leg.

**Lap**:
A specialized **Stint** (or sub-span of one) that the game identifies as a timed lap within a structured race event. Not every **Stint** has **Laps** — free-roam **Stints** do not.
_Avoid_: Circuit, loop.

**Tick**:
A single telemetry sample at the game's emission rate (~60 Hz). The canonical, enriched form of one inbound UDP **Packet**: parsed into the superset schema, with derived fields (lateral G, throttle %, gear-shift events, distance-this-Lap, etc.) computed at ingest. Persisted to storage and broadcast on the live channel in this enriched form — never as raw bytes. The atomic data unit; everything else is an aggregate over **Ticks**.
_Avoid_: Frame, sample.

**Packet**:
The raw UDP datagram as it arrives from Forza's Data Out. Strictly the wire-format object; parsed once at the UDP listener boundary into a **Tick** and then discarded. Only used as a term in code dealing with the network layer or the binary parser.
_Avoid_: Tick, frame, message.

**Stint Type**:
An automatically-assigned classification of a **Stint** based on telemetry-only heuristics. Initial values: `circuit` (lap-based race), `sprint` (timed event with no laps), `freeroam` (in-car driving with no active event), `idle` (paused/menu/loading). `idle` still drives Stint splitting (so race time never merges with menu time) but `idle` Stints are not persisted — only freeroam/sprint/circuit reach the DB.
_Avoid_: Mode, category (those are for user-applied tags).

**Tag**:
A user-applied label attached to a **Stint** (or possibly a **Session** or **Lap**) after capture, used to enrich what telemetry cannot tell us. Free-form by default (e.g. `"Goliath"`, `"PR Stunt - Speed Trap"`) with optional structured tag types reserved for future use (event name, event type, tuning notes, weather, surface, etc.).
_Avoid_: Label, annotation (reserve "annotation" for hot-spot markers, TBD).

**Car**:
The vehicle being driven during a **Stint**, identified from telemetry by `CarOrdinal` (Forza's unique vehicle ID) plus `CarClass` and `CarPerformanceIndex`. Auto-derived; not user-tagged.
_Avoid_: Vehicle.

**Pinned**:
A **Session** state that protects it from being downsampled, regardless of age. Set/unset by the user from the UI.
_Avoid_: Locked, starred, favorited.

**Downsampled**:
A **Session** that has been irreversibly reduced from full-rate **Tick** capture to a lower rate (target rate TBD, e.g. 10 Hz) to reclaim space. The aggregates and 1 Hz preview series are preserved; only the raw **Tick** stream is reduced. Triggered by explicit user action; the UI may recommend it for unpinned **Sessions** older than 10 days but never performs it automatically.
_Avoid_: Compressed, archived, decimated.

**Bookmark**:
A user-marked moment placed on a single **Tick** during live or scrub playback, optionally with a free-form note. Distinct from a **Snapshot** (which is a durable capture).
_Avoid_: Marker, flag, pin.

**Snapshot**:
A durable capture of telemetry state at a chosen moment (one **Tick** or a small window around it), saved as a first-class record that survives **Downsampling** of its source **Session**. Can be created from a **Bookmark** or any arbitrary scrub position. Supports side-by-side comparison and export (PNG / JSON / CSV).
_Avoid_: Capture, frame, freeze.

**Track Path**:
The spatial trajectory of a **Stint**, derived from each **Tick**'s `PositionX/Y/Z`. Rendered as a polyline in the mini-map view, optionally coloured by a per-**Tick** channel (speed, brake force, lateral G). Day-one renderer plots raw world coordinates with auto-fit bounds; a future enhancement may overlay onto Forza region map tiles.
_Avoid_: Route, trace, line.

**Turn**:
A **Tick** range within a **Stint** where the driven path deviates from a straight line in a way the track itself imposes (as opposed to driver repositioning on a straight). Detected from **Track Path** curvature, with auxiliary signals (e.g. sustained lateral G, steering input duration) used to suppress false positives from in-lane corrections. Has explicit `entry / apex / exit` phases. Numbered chronologically along the **Stint** (`turn_1`, `turn_2`, …). On **Circuit** stints, curvature-based identity holds the number stable across **Laps** even when one Lap clips the apex differently. **Sprint** stints get a single pass of numbering. Future **Shape** classification (chicane, hairpin, sweeper, dogleg, esses, …) will categorise individual Turns; not yet modelled.
_Avoid_: Corner, bend, segment, sector. (Historical: previously named "Corner"; renamed when detection was extended to non-Lap stints.)

**Straight**:
A **Tick** range within a **Stint** that fills the gap between two **Turns** (or between a stint boundary and the first/last **Turn**). Numbered chronologically along the **Stint** (`straight_1`, `straight_2`, …) such that **Straight** `N` lies before **Turn** `N` (with one trailing **Straight** after the final **Turn**). A **Stint** with `K` **Turns** has exactly `K+1` **Straights** covering the rest of the **Tick** range, never overlapping. Only emitted for **Stints** where path geometry was collected (**Circuit** and **Sprint** types); **Freeroam** and **Idle** stints have neither **Turns** nor **Straights**.
_Avoid_: Straightaway, line, run.

**Comparison**:
A user-assembled set of 2-6 comparable units (**Laps**, **Turns**, or **Snapshots**) rendered together: time-series channels overlaid on shared charts and **Track Paths** overlaid on the mini-map (ghost-car style, color-coded). Time-series x-axis is always **cumulative distance from entity start**, not elapsed time, to avoid misleading visual offsets when one entity is slower than another. Comparison across different **Cars** or different auto-classified tracks is permitted with a UI warning, never blocked.
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
- **Circuit** and **Sprint** **Stints** contain zero or more **Turns** and exactly `(turn_count + 1)` **Straights**; **Freeroam** and **Idle** **Stints** have neither
