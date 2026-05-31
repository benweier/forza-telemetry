# Driver's-Seat Cluster — WebGPU Live View (Design Spec)

- **Date:** 2026-05-31
- **Status:** Approved design → ready for implementation plan
- **Scope target:** Shared core + **raw WebGPU** renderer (Phase 1). R3F/Three.js WebGPURenderer is **Phase 2**, documented here but not implemented in the first plan.

## Summary

An alternative live view that renders a skeuomorphic driver's-seat instrument cluster in **WebGPU**, driven by the same live telemetry as the existing DOM HUD. It is a deliberate showcase piece — overkill by design — demonstrating WebGPU as the WebGL successor. Everything visible (dials, needles, bars, **and text**) is GPU-rendered.

## Goals

- A concentric analog cluster with glass + neon highlights, GPU-rendered, fed by `useLiveStore`.
- Reachable without disturbing the existing HUD.
- Clean separation between renderer-agnostic domain logic and the renderer, so a second (Three.js) renderer can be added later for comparison with maximal code reuse.

## Non-goals

- No DOM/Canvas2D fallback render. Browsers without WebGPU get a notice (below).
- No 3D scene, camera rig, lighting, or asset pipeline — this is 2D SDF/vector rendering.
- No new instruments beyond the locked set. Fuel/boost/lap/position, steering, and per-wheel tyre data are out of scope.

## Locked visual design

**Layout — "asymmetric sport".** A large **concentric main gauge** on the left, a vertical **right rail** of secondary instruments.

**Main gauge (concentric):**
- **Outer ring = tachometer (RPM).** Slim band (~9px at reference size), **270° sweep with a notch at the bottom** (gap centred on 6 o'clock). Colour ramp teal → green → amber → red; the redline zone (≥ 0.88·`rmx`) glows (bloom). Tick ridges along the band.
- **Inner dial = analog speed.** Larger dark dial with a swinging **needle + hub**, **minor ticks every 13.5°** and **longer major ticks every 54°**, sharing the same 270°/bottom-notch sweep. Centre shows a large digital speed value with `KM/H`, and a small `rpm` caption above.

**Right rail (top → bottom):**
- **Gear tile** — rounded glass tile, large digit (`R`/`N`/`1…n`), faint cyan glow.
- **Throttle + Brake** — two vertical bars; throttle green, brake red, with soft glow on fill.
- **G-force** — a circle with crosshair and a **violet dot** positioned by lateral (`lg`) / longitudinal (`lng`) G.

Aesthetic: **analog skeuomorphic** (bezels, needle, depth) with **glass + neon** accents tuned to the Glass theme palette. Fidelity is **"tasteful showcase"**: spring-damped needle/bar motion, redline/shift bloom, crisp anti-aliased edges — not maximal (no environment reflections/specular).

Reference mockups: `.superpowers/brainstorm/63100-1780224908/content/layout-v3.html` (and `style-direction.html`, `layout.html`).

## Integration

- New file-based route **`client/src/routes/live.cluster.tsx`** → path **`/live/cluster`**.
- A small **segmented toggle** ("HUD" / "Cluster") in the live header links the two. The existing `routes/live.tsx` DOM HUD is **untouched**; both read the same `useLiveStore`.

## Fallback

- On mount the host attempts WebGPU device acquisition. On failure (no `navigator.gpu`, no adapter, or `device.lost` unrecoverable) it renders a **"WebGPU required" notice** styled with Glass tokens, including a link back to the HUD view. No automatic redirect.

## Architecture

Renderer-agnostic **core** + a **`ClusterRenderer` interface** + a **raw** implementation + a thin React **host**. This structure is what makes Phase 2 (Three.js) a drop-in second implementation.

```
client/src/cluster/
  core/                      # renderer-agnostic, unit-tested, SHARED across renderers
    spec.ts                  # geometry constants: ring radii, sweep start/extent, notch,
                             #   tick counts, rail layout, normalized positions
    palette.ts               # colour stops + neon/glow colours (tuned to Glass; see Theming)
    scale.ts                 # pure value→sweep-angle mapping, clamping, redline threshold
    physics.ts               # spring-damper integrator (step(state, target, dt) → state)
    state.ts                 # TickFrame → ClusterState projection (units, gear, clamps)
    sdf.wgsl                 # pure WGSL SDF/AA math functions (ring arc, tick, needle,
                             #   circle, rounded rect) — reused by raw; wrapped for Three later
    msdf/
      atlas.png              # pre-generated MSDF glyph atlas (committed asset)
      atlas.json             # glyph metrics
      layout.ts              # renderer-agnostic glyph run layout (string → quads in dial space)
  renderer.ts                # interface: init(canvas, opts) / render(state) / resize() / destroy()
  raw/                       # Phase 1 implementation
    raw-renderer.ts          # owns device/context/pipelines/bind groups/uniform buffer
    passes/
      instruments.wgsl       # fullscreen SDF pass: all analog instruments from a uniform block
      glyphs.wgsl            # MSDF textured-quad pass
      bloom.wgsl             # bright-pass + separable blur + additive composite
  ClusterCanvas.tsx          # React host: <canvas>, device init, rAF loop, lifecycle, notice
```

### `ClusterRenderer` interface (`renderer.ts`)

```ts
interface ClusterRenderer {
  init(canvas: HTMLCanvasElement, opts: RendererOpts): Promise<void>; // throws → notice
  render(state: ClusterState): void;   // called per rAF frame
  resize(width: number, height: number, dpr: number): void;
  destroy(): void;                     // free GPU resources
}
```

`ClusterState` is the renderer-agnostic projection produced by `core/state.ts` + `core/physics.ts`: animated needle angles (rpm, speed), gear string, throttle/brake fills [0,1], g-dot position, redline factor.

### Raw renderer passes (Phase 1)

1. **Instrument pass** — a single fullscreen-quad fragment shader (`instruments.wgsl`) draws *all* analog instruments analytically via SDF functions from `sdf.wgsl`, driven by one uniform block (positions/radii/angles/fills/colours from `spec.ts` + `ClusterState`). Minimal geometry, maximal "shader showcase," and the SDF functions are exactly what Phase 2 reuses.
2. **Glyph pass** — MSDF textured quads (`glyphs.wgsl`) for speed value, gear digit, `rpm`, and labels. Quads come from `core/msdf/layout.ts`.
3. **Bloom post** — render passes 1–2 to an offscreen texture; bright-pass (redline ring, needle tip, glow edges) → 2-pass separable blur → additive composite to the swapchain. Tasteful intensity.

### React host (`ClusterCanvas.tsx`)

Thin. Mounts a `<canvas>`; calls `raw-renderer.init()`. On success, starts an **rAF loop** that reads `useLiveStore.getState().latest` each frame (imperative — no React re-render per frame, same pattern as `Sparkline`), runs `physics.step` toward the new targets, and calls `render(state)`. Handles `devicePixelRatio`, `ResizeObserver` → `resize()`, and `device.lost` → re-init or notice. Cleanup cancels rAF and calls `destroy()`. On init failure renders the WebGPU-required notice instead of the canvas.

## Data flow & physics

- Source: `useLiveStore.getState().latest` (latest `TickFrame`), polled per frame.
- Mapping (`core/state.ts`): speed `sp` (m/s) → km/h; tach from `rpm` against `rmx`, redline at `0.88·rmx`; gear from `g` (`0`→`N`, negative→`R`, else number); throttle `tp`, brake `bp` (already [0,1]); G dot from `lg`/`lng` clamped to a ±limit.
- **Motion is spring-damped toward the latest value, integrated per frame** — needles and bars chase the *value*, never keyed off `sts`. (Telemetry arrives in bursts; timestamps are unreliable for animation timing — the same lesson the sparkline learned. See the `sts`-is-bursty gotcha.)
- Scales are **fixed** (e.g., speed `0..400` km/h) and configurable in `spec.ts`, for stable analog behaviour rather than auto-ranging.

## Theming / colours

Canvas can't read CSS tokens directly. `core/palette.ts` holds explicit colours tuned to the Glass palette (teal/green/amber/red ramp, red needle, cyan gear glow, violet g-dot). At init the host may sample a few CSS custom properties (`--accent`, `--warning`, `--separator`) via `getComputedStyle` and pass overrides into `RendererOpts`, falling back to the constants. No hex/OKLCH inlined in components (DESIGN.md rule); colour constants live in `palette.ts`.

## Lifecycle & robustness

- Device acquisition isolated in a helper returning device-or-reason.
- DPR-aware sizing; `ResizeObserver` drives `resize()`.
- `device.lost` → one re-init attempt, else notice.
- rAF runs while mounted; may idle (skip render) when physics is settled and no new tick has arrived, to avoid burning frames at rest.

## MSDF font assets

Ship a **pre-generated MSDF atlas** (`atlas.png` + `atlas.json`) for one font, committed as an asset — covering digits `0-9`, `R`, `N`, `.`, `-`, and the label strings (`KM/H`, `RPM`, `THR`, `BRK`, `G`). Generated offline (e.g. `msdf-atlas-gen`); the generator is documented but **not** wired into the build (avoids native build-time deps). `core/msdf/layout.ts` turns a string + metrics into positioned quads in dial space.

## Testing

- **Unit tests (new):** add **Vitest** (idiomatic for Vite; client currently has no test runner — this is a deliberate, flagged addition). Cover the pure core:
  - `scale.ts` — value→angle within sweep, clamping at ends, redline threshold.
  - `physics.ts` — spring step converges, no unbounded overshoot, stable across dt.
  - `state.ts` — tick→state mapping: unit conversion, gear `N`/`R`, clamps.
  - `msdf/layout.ts` — glyph run advances/positions for a sample string.
- **Visual verification:** Chrome DevTools MCP against injected `useLiveStore` state (the harness used for the sparkline) — screenshots at idle, mid-range, and redline.
- **No regression** to the DOM HUD (untouched).

## Phase 2 (future, documented): R3F / Three.js WebGPURenderer

A second `ClusterRenderer` implementation under `client/src/cluster/three/`, using `@react-three/fiber` with Three's `WebGPURenderer` and **TSL**. Selected by the host via config/route param for side-by-side comparison.

- **Shared, reused as-is:** all of `core/` — `spec.ts`, `palette.ts`, `scale.ts`, `physics.ts`, `state.ts`, the MSDF atlas + `layout.ts`. The `ClusterState` contract and `ClusterRenderer` interface are identical.
- **Partially reused:** the pure WGSL SDF/math functions in `sdf.wgsl` can be embedded via TSL `wgslFn` nodes.
- **NOT portable (rewritten per renderer):** full shader *programs*, bind-group/uniform/vertex plumbing, and the pass graph — Three owns these via TSL. Three gives bloom/MSAA/resize/device management and WebGL fallback "for free" in exchange.
- **Note:** Three's WebGPURenderer + TSL is newer and churns more than raw WebGPU; acceptable for this single-user LAN tool.

## File manifest (Phase 1)

**New:**
- `client/src/routes/live.cluster.tsx`
- `client/src/cluster/renderer.ts`
- `client/src/cluster/core/{spec,palette,scale,physics,state}.ts`
- `client/src/cluster/core/sdf.wgsl`
- `client/src/cluster/core/msdf/{atlas.png,atlas.json,layout.ts}`
- `client/src/cluster/raw/raw-renderer.ts`
- `client/src/cluster/raw/passes/{instruments,glyphs,bloom}.wgsl`
- `client/src/cluster/ClusterCanvas.tsx`
- Vitest config + `*.test.ts` for the core modules.

**Changed:**
- `client/src/routes/live.tsx` — add the HUD/Cluster segmented toggle in the header (additive; HUD body unchanged).
- `client/package.json` — add `vitest` (+ any WGSL import handling for Vite).

## Risks / open items

- **WGSL imports under Vite/rolldown** — confirm `.wgsl` loads as a string (raw import or small plugin). Cheap to resolve in the plan.
- **MSDF atlas generation** — one-time offline step; ensure committed atlas covers all needed glyphs.
- **Bloom cost/quality** — keep blur passes small; tune threshold so only redline/glow blooms.
- **Vitest addition** — first test tooling in the client; flagged for approval.
