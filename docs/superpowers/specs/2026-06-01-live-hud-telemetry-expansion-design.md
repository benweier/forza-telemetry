# Live HUD telemetry expansion — design

**Date:** 2026-06-01
**Status:** Approved (brainstorming) — pending implementation plan
**Scope:** `/live` HUD route only. The Instrument (WebGPU) view is explicitly out of scope.

## Goal

Surface the rich per-wheel and engine telemetry already present in every `TickFrame`
but currently unused by the HUD: suspension travel, tire slip (ratio / angle /
combined), tire temperature, wheel rotation speed, drivetrain, cylinders, power,
torque, and boost. Present them as **glanceable visualisations**, not raw numbers,
staying entirely within the existing Glass design language.

## Design principles

- **Unified, not one-widget-per-field.** Per-wheel data converges on a single
  top-down **car diagram** centerpiece that encodes several channels at once.
- **On-palette only.** Use semantic Glass tokens (`bg-surface`, `text-foreground`,
  `text-muted`, `--accent`, `--success`, `--warning`, `--danger`, `rounded-2xl`,
  `shadow-surface`). No inline hex/OKLCH. No new blue/"cold" token — see Heat scale.
- **Avoid hiding content.** Prefer showing metrics simultaneously over toggles.
- **Reuse existing card vocabulary** so new panels don't look out of place.

## Layout

Three-column grid (`lg:grid-cols-[1.25fr_0.95fr_0.95fr]`, stacks on small screens),
with **equal column heights** so the car diagram does not dominate as a tall center
column. The chassis card stretches to match the combined height of the stacked cards
beside it; the diagram SVG scales within that card rather than dictating the row
height.

- **Column 1 — Drive inputs (existing, unchanged):** SpeedCard, RpmBar,
  Throttle/Brake bars, Speed + Lateral-G sparklines.
- **Column 2 — Chassis centerpiece (new):** the car diagram.
- **Column 3 — Engine + forces (new + existing):** live dyno curve + engine badge,
  boost gauge, G-force panel (existing), slim Vehicle meta (existing, trimmed).

## Components

### 1. Car diagram (`CarDiagram`)

A top-down SVG silhouette with four corner wheels (FL, FR, RL, RR — the canonical
per-wheel order). Each corner encodes **four channels simultaneously**:

| Visual channel | Source field | Encoding |
|---|---|---|
| Wheel **fill colour** | `tt` (tire temp) | Grip-window heat scale (see below) |
| **Ring** thickness + intensity | `tcs` (combined slip) | Thin/dim = grip; thick/bright `danger` = losing grip |
| Ring **lockup/spin tag** | `tsr` (slip ratio) sign | `tsr < −threshold` → "LOCK"; `tsr > +threshold` → "SPIN" |
| **Side bar** height | `stn` (normalised suspension travel 0..1) | Vertical fill bar beside each wheel |
| **Slip-angle arrow** | `tsa` (slip angle) | Short tick from wheel; direction + length = angle sign/magnitude |

A center **driven-axle highlight** uses `dt` (drivetrain) to tint the driven
wheels' axle (`--success`-soft): FWD = front, RWD = rear, AWD = both.

Wheel rotation speed (`wrs`) corroborates lockup/spin but `tsr` is the primary
signal (it directly encodes lockup as negative, spin as positive, and needs no
wheel-radius assumption). `wrs` may be used as a secondary cross-check or tooltip.

**Heat scale (grip-window, semantic):**
`muted` (cold) → `success` (optimal grip window) → `warning` (hot) → `danger`
(overheating). Implemented as a value→token interpolation. Green = "in the grip
window", which is more actionable than a literal blue→red thermal ramp and needs no
new token. Thresholds (cold/optimal/hot/overheat boundaries) are **empirical** —
seed with placeholders and tune against real captures; log to `docs/data-needed.md`.

### 2. Live dyno curve (`DynoCurve`)

Plots **power** (`pw`, `--accent`) and **torque** (`tq`, `--warning`) against engine
RPM. The curve is an **envelope**: the client accumulates the max power/torque seen
per RPM bucket over the current car/session and the line fills in as you rev through
the range. A dashed **cursor** tracks live RPM with a dot on each curve; a `danger`-
tinted zone marks the redline (from `rmx`).

- X-axis: 0 → `rmx`. Y-axis: auto-scaled to max-seen power/torque.
- Reset the envelope when `co` (car ordinal) changes — a new car has a new curve.
- Display power in **kW** (`pw` is watts ÷ 1000), torque in **Nm** (`tq` direct).
  An hp/lb-ft toggle is out of scope for now.
- State lives in component-local refs (per-RPM-bucket maxima), updated on the same
  rAF/tick cadence the HUD already uses — not in the global live store.

### 3. Engine badge (`EngineBadge`)

A small pill near the dyno panel: cylinder count → label (e.g. `V8`) from `ncy`,
plus drivetrain (`FWD`/`RWD`/`AWD`) from `dt`. Pure identity context, cheap. Reads
"—" when fields are 0 (FH5/unknown). Displacement is **not** in telemetry — omit it.

### 4. Boost gauge (`BoostGauge`)

A horizontal gauge with a vacuum→positive zone. Centre (zero) is `muted`; positive
boost ramps toward `--accent`; the vacuum end uses `muted` (NOT a blue — the
mockup's blue is dropped). A marker shows current `bo`. Boost **units are
unconfirmed** — display the raw value with a neutral unit label until a capture
confirms scale; log to `docs/data-needed.md`.

## Data flow

No server changes. All fields already exist on `TickFrame` (`tick.generated.ts`) and
arrive via the existing WebSocket → `useLiveStore`. New components read `latest` (and
the ring buffer for sparkline-style history where relevant) exactly as the current
HUD does. The dyno envelope is the only stateful addition and is component-local.

## Units & enums (reference)

- `pw` watts, `tq` Nm, `sp` m/s — SI on the wire (per parquet tags). Speed already
  ×3.6 for km/h in the HUD.
- `dt` drivetrain enum — Forza convention `0 = FWD, 1 = RWD, 2 = AWD`. Confirm
  against capture if not already documented.
- `stn` normalised suspension travel 0..1 (use for the bar); `stm` metres available
  for tooltips.
- `tt` tire temp, `bo` boost — **unit/scale unconfirmed** (open questions below).

## Open empirical questions (→ `docs/data-needed.md`)

1. **Tire temp (`tt`) unit/scale** and the grip-window thresholds (cold / optimal /
   hot / overheat) for the heat scale.
2. **Boost (`bo`) unit** (PSI / bar / atm) and its zero/vacuum convention.
3. **Slip-ratio (`tsr`) lockup/spin thresholds** for the ring LOCK/SPIN tags.

These ship with documented placeholder thresholds; the entries are removed/updated
when real captures resolve them.

## Testing

Pure helpers get Vitest coverage (mirrors the instrument-core pattern):
- Heat-scale: value → token/stop interpolation across the grip window.
- Dyno envelope: per-RPM-bucket max accumulation + reset on car change.
- Lockup/spin classification from `tsr` and slip-angle arrow geometry from `tsa`.
- Drivetrain → driven-axle and cylinder → label mapping.

React rendering verified via the existing Chrome-DevTools-MCP harness (inject a
`TickFrame` into `useLiveStore`, screenshot).

## Out of scope

- Instrument (WebGPU) view changes.
- hp/lb-ft and °C/°F unit toggles.
- Server/schema changes (all data is already present).
- Historical (non-live) rendering of these panels.
