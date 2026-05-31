# WebGPU Driver-Instrument Live View — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an alternative live view at `/live/instrument` that renders a skeuomorphic driver's-seat instrument entirely in WebGPU, driven by the same `useLiveStore` telemetry as the existing DOM HUD.

**Architecture:** A renderer-agnostic `core/` (pure TS: geometry spec, value→angle scaling, spring physics, tick→state projection, MSDF glyph layout) sits behind a `InstrumentRenderer` interface. Phase 1 ships one implementation, `raw/` (hand-written WGSL: a fullscreen SDF instrument pass, an MSDF glyph pass, a bloom post pass). A thin React host (`InstrumentCanvas.tsx`) owns the canvas, device lifecycle, and an rAF loop that reads the store, steps physics, and calls `render(state)`. Browsers without WebGPU get a notice. Phase 2 (R3F/Three.js) reuses all of `core/` behind the same interface and is **out of scope for this plan**.

**Tech Stack:** TypeScript, WebGPU (`navigator.gpu`), WGSL, TanStack Start (file routes), Zustand (`useLiveStore`), Vitest (new — first client test runner), Vite `?raw` imports for WGSL, MSDF font atlas.

**Spec:** `docs/superpowers/specs/2026-05-31-webgpu-cluster-dashboard-design.md`

**Conventions (from CLAUDE.md):** Conventional Commits, granular, one logical concern per commit; stage explicit paths, never `git add .` (there are unrelated uncommitted client edits in the tree — do not stage them). Client gate is `pnpm exec tsc --noEmit`; lint is `pnpm lint` (oxlint — pre-existing warnings exist in other files; only fix issues your own changes introduce). Never inline hex/OKLCH in components (DESIGN.md) — colour constants live in `core/palette.ts`. Run all client commands from `client/`.

---

## File structure (locked)

```
client/src/instrument/
  renderer.ts                 # InstrumentRenderer interface + RendererOpts + InstrumentState types
  core/
    spec.ts                   # geometry constants (radii, sweep, notch, ticks, rail layout)
    palette.ts                # colour stops + neon colours
    scale.ts                  # value→angle mapping, clamp, redline factor (pure)
    physics.ts                # critically-damped scalar smoother (pure)
    state.ts                  # Targets type, targetsFromTick(), buildInstrumentState()
    msdf/
      layout.ts               # string + metrics → positioned glyph quads (pure)
      atlas.png               # committed MSDF atlas (generated offline)
      atlas.json              # committed glyph metrics
  raw/
    device.ts                 # acquireDevice() → {ok, device, ctx, format} | {ok:false, reason}
    raw-renderer.ts           # implements InstrumentRenderer
    passes/
      instruments.wgsl        # fullscreen SDF pass (all analog instruments)
      glyphs.wgsl             # MSDF textured-quad pass
      bloom.wgsl              # bright-pass + separable blur + composite
  InstrumentCanvas.tsx           # React host
client/src/routes/live.instrument.tsx   # new route
client/src/routes/live.tsx           # MODIFY: add HUD/Instrument toggle in header
client/vitest.config.ts              # new
client/package.json                  # MODIFY: add vitest + test script
```

**Delivery order rationale:** plumbing → tested pure core → device/blank canvas → analog instruments (first visible showcase) → bloom → text → host wiring → route/toggle → visual verification. Each renderer task ends with a visual check so the showcase is always in a runnable state.

---

## Task 0: Project plumbing (Vitest + WGSL imports + smoke test)

**Files:**
- Modify: `client/package.json`
- Create: `client/vitest.config.ts`
- Create: `client/src/instrument/core/smoke.test.ts` (temporary, deleted in Step 6)

- [ ] **Step 1: Add Vitest**

Run (from `client/`):
```bash
pnpm add -D vitest
```

- [ ] **Step 2: Add the test script to `client/package.json`**

In the `"scripts"` block add:
```json
"test": "vitest run",
"test:watch": "vitest"
```

- [ ] **Step 3: Create `client/vitest.config.ts`**

```ts
import { defineConfig } from "vitest/config";
import tsconfigPaths from "vite-tsconfig-paths";

export default defineConfig({
  plugins: [tsconfigPaths()],
  test: { environment: "node", include: ["src/**/*.test.ts"] },
});
```

If `vite-tsconfig-paths` is not already a dependency, install it: `pnpm add -D vite-tsconfig-paths`. This makes the `~/*` path alias resolve in tests.

- [ ] **Step 4: Write a smoke test**

`client/src/instrument/core/smoke.test.ts`:
```ts
import { expect, test } from "vitest";

test("vitest runs", () => {
  expect(1 + 1).toBe(2);
});
```

- [ ] **Step 5: Run it**

Run: `pnpm test`
Expected: 1 passed.

- [ ] **Step 6: Delete the smoke test, confirm WGSL `?raw` typing**

Delete `smoke.test.ts`. Add WGSL module typing to `client/src/vite-env.d.ts` (it exists in the tree):
```ts
declare module "*.wgsl?raw" {
  const src: string;
  export default src;
}
```
This is how shaders load — `import shader from "./x.wgsl?raw"` yields the source string. No Vite plugin needed.

- [ ] **Step 7: Commit**

```bash
git add client/package.json client/pnpm-lock.yaml client/vitest.config.ts client/src/vite-env.d.ts
git commit -m "chore(client): add vitest + wgsl raw-import typing for instrument work"
```

---

## Task 1: Renderer types + geometry spec + palette

**Files:**
- Create: `client/src/instrument/renderer.ts`
- Create: `client/src/instrument/core/spec.ts`
- Create: `client/src/instrument/core/palette.ts`
- Test: `client/src/instrument/core/spec.test.ts`

- [ ] **Step 1: Write `renderer.ts` (types + interface, no logic)**

```ts
/** Per-frame, renderer-agnostic description of the instrument. */
export interface InstrumentState {
  speedKmh: number;       // for the digital readout
  rpm: number;            // raw, for the rpm caption
  speedAngle: number;     // radians within the dial sweep (0 at sweep start)
  rpmAngle: number;       // radians within the ring sweep
  redline: number;        // 0..1, depth into redline for glow
  gear: string;           // "R" | "N" | "1".."n"
  throttle: number;       // 0..1
  brake: number;          // 0..1
  gx: number;             // lateral g, normalized -1..1 (right positive)
  gy: number;             // longitudinal g, normalized -1..1 (accel positive)
}

export interface RendererOpts {
  /** Optional colour overrides sampled from CSS custom properties at init. */
  colors?: Partial<import("./core/palette").Palette>;
}

export interface InstrumentRenderer {
  init(canvas: HTMLCanvasElement, opts: RendererOpts): Promise<void>;
  render(state: InstrumentState): void;
  resize(width: number, height: number, dpr: number): void;
  destroy(): void;
}
```

- [ ] **Step 2: Write `palette.ts`**

```ts
/** All colours as [r,g,b] in 0..1 linear-ish sRGB. No CSS tokens reach the GPU. */
export interface Palette {
  ringLow: [number, number, number];
  ringMid: [number, number, number];
  ringRed: [number, number, number];
  needle: [number, number, number];
  gearGlow: [number, number, number];
  gDot: [number, number, number];
  throttle: [number, number, number];
  brake: [number, number, number];
  panel: [number, number, number];
  tick: [number, number, number];
}

export const DEFAULT_PALETTE: Palette = {
  ringLow: [0.18, 0.83, 0.75],   // teal
  ringMid: [1.0, 0.81, 0.23],    // amber
  ringRed: [1.0, 0.35, 0.30],    // red
  needle: [1.0, 0.35, 0.30],
  gearGlow: [0.35, 0.82, 1.0],   // cyan
  gDot: [0.61, 0.55, 1.0],       // violet
  throttle: [0.21, 0.82, 0.48],
  brake: [1.0, 0.35, 0.30],
  panel: [0.07, 0.08, 0.10],
  tick: [0.78, 0.82, 0.87],
};
```

- [ ] **Step 3: Write `spec.ts`**

```ts
/**
 * Instrument geometry in a normalized 0..1 layout space (x right, y down),
 * mapped to the canvas by the renderer. Sweep angles in radians, measured
 * clockwise from the +x axis. The 270° sweep leaves a 90° notch at the bottom.
 */
export const SPEC = {
  // main concentric gauge centre + radii (fraction of min(canvas w,h))
  gauge: { cx: 0.32, cy: 0.5, ringOuter: 0.46, ringInner: 0.42, dialOuter: 0.40 },
  // sweep: start at lower-left (225° = 1.25π... measured from +x, clockwise screen space)
  sweep: { startDeg: 135, extentDeg: 270 }, // notch centred at bottom (6 o'clock)
  speedTicks: { minorEveryDeg: 13.5, majorEveryDeg: 54, minorR: [0.34, 0.38], majorR: [0.32, 0.38] },
  rail: {
    x: 0.78,
    gear: { cy: 0.22, size: 0.16 },
    bars: { cy: 0.5, w: 0.03, h: 0.26, gap: 0.04 },
    gforce: { cy: 0.8, r: 0.11 },
  },
  scales: { speedMaxKmh: 400, redlineFraction: 0.88 },
} as const;
```

- [ ] **Step 4: Write `spec.test.ts` (invariants)**

```ts
import { expect, test } from "vitest";
import { SPEC } from "./spec";

test("sweep leaves a 90° bottom notch", () => {
  expect(SPEC.sweep.extentDeg).toBe(270);
  // start 135° + 270° = 405° → wraps to 45°; notch from 45° to 135° centred on 90° (bottom, screen-down y)
  expect((SPEC.sweep.startDeg + SPEC.sweep.extentDeg) % 360).toBe(45);
});

test("redline fraction is within the dial", () => {
  expect(SPEC.scales.redlineFraction).toBeGreaterThan(0.5);
  expect(SPEC.scales.redlineFraction).toBeLessThan(1);
});
```

- [ ] **Step 5: Run** — `pnpm test` → 2 passed.

- [ ] **Step 6: Commit**

```bash
git add client/src/instrument/renderer.ts client/src/instrument/core/spec.ts client/src/instrument/core/palette.ts client/src/instrument/core/spec.test.ts
git commit -m "feat(client/instrument): renderer interface, geometry spec, palette"
```

---

## Task 2: `scale.ts` — value→angle mapping (TDD)

**Files:**
- Create: `client/src/instrument/core/scale.ts`
- Test: `client/src/instrument/core/scale.test.ts`

- [ ] **Step 1: Write failing tests**

`scale.test.ts`:
```ts
import { expect, test } from "vitest";
import { fractionToAngle, valueToFraction, redlineFactor } from "./scale";

const RAD = Math.PI / 180;

test("valueToFraction clamps to 0..1", () => {
  expect(valueToFraction(-5, 0, 100)).toBe(0);
  expect(valueToFraction(50, 0, 100)).toBe(0.5);
  expect(valueToFraction(150, 0, 100)).toBe(1);
});

test("fractionToAngle spans the sweep from the start", () => {
  expect(fractionToAngle(0, 135, 270)).toBeCloseTo(135 * RAD, 5);
  expect(fractionToAngle(1, 135, 270)).toBeCloseTo(405 * RAD, 5);
  expect(fractionToAngle(0.5, 135, 270)).toBeCloseTo(270 * RAD, 5);
});

test("redlineFactor is 0 below threshold, ramps to 1 at max", () => {
  expect(redlineFactor(0.5, 0.88)).toBe(0);
  expect(redlineFactor(0.88, 0.88)).toBe(0);
  expect(redlineFactor(1, 0.88)).toBeCloseTo(1, 5);
  expect(redlineFactor(0.94, 0.88)).toBeCloseTo(0.5, 5);
});
```

- [ ] **Step 2: Run → FAIL** (`pnpm test` — module not found).

- [ ] **Step 3: Implement `scale.ts`**

```ts
const RAD = Math.PI / 180;

export function valueToFraction(value: number, min: number, max: number): number {
  if (max === min) return 0;
  const f = (value - min) / (max - min);
  return f < 0 ? 0 : f > 1 ? 1 : f;
}

export function fractionToAngle(fraction: number, startDeg: number, extentDeg: number): number {
  return (startDeg + fraction * extentDeg) * RAD;
}

/** 0 below `threshold` fraction, linearly ramps to 1 at fraction 1. */
export function redlineFactor(fraction: number, threshold: number): number {
  if (fraction <= threshold) return 0;
  return Math.min(1, (fraction - threshold) / (1 - threshold));
}
```

- [ ] **Step 4: Run → PASS.**

- [ ] **Step 5: Commit**

```bash
git add client/src/instrument/core/scale.ts client/src/instrument/core/scale.test.ts
git commit -m "feat(client/instrument): value→angle scaling + redline factor"
```

---

## Task 3: `physics.ts` — critically-damped smoother (TDD)

**Files:**
- Create: `client/src/instrument/core/physics.ts`
- Test: `client/src/instrument/core/physics.test.ts`

Critically-damped spring (no overshoot), semi-implicit Euler. `damping = 2·sqrt(stiffness)`.

- [ ] **Step 1: Write failing tests**

`physics.test.ts`:
```ts
import { expect, test } from "vitest";
import { makeSmoother, stepSmoother } from "./physics";

test("converges to target", () => {
  let s = makeSmoother(0);
  for (let i = 0; i < 600; i++) s = stepSmoother(s, 100, 1 / 60, 120);
  expect(s.value).toBeCloseTo(100, 1);
  expect(Math.abs(s.velocity)).toBeLessThan(0.5);
});

test("does not overshoot (critically damped)", () => {
  let s = makeSmoother(0);
  let maxV = 0;
  for (let i = 0; i < 600; i++) {
    s = stepSmoother(s, 100, 1 / 60, 120);
    maxV = Math.max(maxV, s.value);
  }
  expect(maxV).toBeLessThanOrEqual(100.5); // allow tiny numeric slack
});

test("stable for large dt (no NaN/explosion)", () => {
  let s = makeSmoother(0);
  for (let i = 0; i < 50; i++) s = stepSmoother(s, 100, 0.5, 120);
  expect(Number.isFinite(s.value)).toBe(true);
  expect(s.value).toBeGreaterThan(0);
  expect(s.value).toBeLessThanOrEqual(101);
});
```

- [ ] **Step 2: Run → FAIL.**

- [ ] **Step 3: Implement `physics.ts`**

```ts
export interface Smoother {
  value: number;
  velocity: number;
}

export function makeSmoother(value: number): Smoother {
  return { value, velocity: 0 };
}

/**
 * Critically-damped spring step (semi-implicit Euler). `stiffness` higher =
 * snappier. Large dt is sub-stepped so it never overshoots or explodes.
 */
export function stepSmoother(s: Smoother, target: number, dt: number, stiffness: number): Smoother {
  const damping = 2 * Math.sqrt(stiffness);
  const maxStep = 1 / 120;
  let { value, velocity } = s;
  let remaining = dt;
  while (remaining > 0) {
    const h = Math.min(maxStep, remaining);
    const accel = stiffness * (target - value) - damping * velocity;
    velocity += accel * h;
    value += velocity * h;
    remaining -= h;
  }
  return { value, velocity };
}
```

- [ ] **Step 4: Run → PASS.**

- [ ] **Step 5: Commit**

```bash
git add client/src/instrument/core/physics.ts client/src/instrument/core/physics.test.ts
git commit -m "feat(client/instrument): critically-damped needle smoother"
```

---

## Task 4: `state.ts` — tick→targets + buildInstrumentState (TDD)

**Files:**
- Create: `client/src/instrument/core/state.ts`
- Test: `client/src/instrument/core/state.test.ts`

- [ ] **Step 1: Write failing tests**

`state.test.ts`:
```ts
import { expect, test } from "vitest";
import type { TickFrame } from "~/types/tick.generated";
import { targetsFromTick, buildInstrumentState, gearLabel } from "./state";

function tick(p: Partial<TickFrame>): TickFrame {
  return { sp: 0, rpm: 0, rmx: 8000, g: 0, tp: 0, bp: 0, lg: 0, lng: 0 } as TickFrame & typeof p && { ...({} as TickFrame), ...p } as TickFrame;
}

test("speed converts m/s → km/h", () => {
  const t = targetsFromTick({ sp: 50 } as TickFrame, 8000);
  expect(t.speedKmh).toBeCloseTo(180, 1); // 50 m/s * 3.6
});

test("gearLabel maps neutral and reverse", () => {
  expect(gearLabel(0)).toBe("N");
  expect(gearLabel(-1)).toBe("R");
  expect(gearLabel(3)).toBe("3");
});

test("buildInstrumentState produces angles within the sweep", () => {
  const cs = buildInstrumentState({ speedKmh: 0, rpm: 0, throttle: 0, brake: 0, gx: 0, gy: 0, gear: "N", rmx: 8000 });
  const RAD = Math.PI / 180;
  expect(cs.speedAngle).toBeCloseTo(135 * RAD, 5);
  expect(cs.rpmAngle).toBeCloseTo(135 * RAD, 5);
});
```
(The `tick()` helper above is illustrative; tests pass partial `TickFrame` casts directly as shown in the individual `expect`s.)

- [ ] **Step 2: Run → FAIL.**

- [ ] **Step 3: Implement `state.ts`**

```ts
import type { TickFrame } from "~/types/tick.generated";
import type { InstrumentState } from "../renderer";
import { SPEC } from "./spec";
import { fractionToAngle, redlineFactor, valueToFraction } from "./scale";

/** Numeric channels the smoothers animate, plus snap fields. */
export interface Targets {
  speedKmh: number;
  rpm: number;
  throttle: number;
  brake: number;
  gx: number;
  gy: number;
  gear: string;
  rmx: number;
}

const G_LIMIT = 2.5; // g, for normalizing the g-dot to -1..1

export function gearLabel(g: number): string {
  if (g < 0) return "R";
  if (g === 0) return "N";
  return String(g);
}

export function targetsFromTick(t: TickFrame, _fallbackRmx = 8000): Targets {
  const rmx = t.rmx && t.rmx > 0 ? t.rmx : _fallbackRmx;
  return {
    speedKmh: (t.sp ?? 0) * 3.6,
    rpm: t.rpm ?? 0,
    throttle: t.tp ?? 0,
    brake: t.bp ?? 0,
    gx: Math.max(-1, Math.min(1, (t.lg ?? 0) / G_LIMIT)),
    gy: Math.max(-1, Math.min(1, (t.lng ?? 0) / G_LIMIT)),
    gear: gearLabel(t.g ?? 0),
    rmx,
  };
}

/** Build the renderer state from (already-smoothed) numeric channels. */
export function buildInstrumentState(a: Targets): InstrumentState {
  const { startDeg, extentDeg } = SPEC.sweep;
  const speedFrac = valueToFraction(a.speedKmh, 0, SPEC.scales.speedMaxKmh);
  const rpmFrac = valueToFraction(a.rpm, 0, a.rmx);
  return {
    speedKmh: a.speedKmh,
    rpm: a.rpm,
    speedAngle: fractionToAngle(speedFrac, startDeg, extentDeg),
    rpmAngle: fractionToAngle(rpmFrac, startDeg, extentDeg),
    redline: redlineFactor(rpmFrac, SPEC.scales.redlineFraction),
    gear: a.gear,
    throttle: a.throttle,
    brake: a.brake,
    gx: a.gx,
    gy: a.gy,
  };
}
```

- [ ] **Step 4: Run → PASS.** (Fix the `tick()` helper in the test to a clean partial-cast factory if TS complains; the assertions above use direct casts and are the real checks.)

- [ ] **Step 5: Commit**

```bash
git add client/src/instrument/core/state.ts client/src/instrument/core/state.test.ts
git commit -m "feat(client/instrument): tick→targets projection + instrument state builder"
```

---

## Task 5: WebGPU device acquisition

**Files:**
- Create: `client/src/instrument/raw/device.ts`

(No unit test — browser API; exercised by the host and visual checks.)

- [ ] **Step 1: Implement `device.ts`**

```ts
export type DeviceResult =
  | { ok: true; device: GPUDevice; context: GPUCanvasContext; format: GPUTextureFormat }
  | { ok: false; reason: string };

export async function acquireDevice(canvas: HTMLCanvasElement): Promise<DeviceResult> {
  if (!("gpu" in navigator)) return { ok: false, reason: "This browser has no WebGPU support." };
  const adapter = await navigator.gpu.requestAdapter();
  if (!adapter) return { ok: false, reason: "No suitable GPU adapter was found." };
  let device: GPUDevice;
  try {
    device = await adapter.requestDevice();
  } catch {
    return { ok: false, reason: "Failed to create a GPU device." };
  }
  const context = canvas.getContext("webgpu");
  if (!context) return { ok: false, reason: "Could not get a WebGPU canvas context." };
  const format = navigator.gpu.getPreferredCanvasFormat();
  context.configure({ device, format, alphaMode: "premultiplied" });
  return { ok: true, device, context, format };
}
```

- [ ] **Step 2: Type check** — `pnpm exec tsc --noEmit`. (Requires `@webgpu/types`; if `tsc` errors on `navigator.gpu`/`GPUDevice`, install: `pnpm add -D @webgpu/types` and add `"types": ["@webgpu/types"]` to `client/tsconfig.json` compilerOptions, or add `/// <reference types="@webgpu/types" />` at the top of `device.ts`.) Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add client/src/instrument/raw/device.ts client/package.json client/pnpm-lock.yaml client/tsconfig.json
git commit -m "feat(client/instrument): webgpu device acquisition helper"
```

---

## Task 6: Raw renderer skeleton — clear pass + blank canvas

**Files:**
- Create: `client/src/instrument/raw/raw-renderer.ts`

Goal: `init` → configures device, `render` → clears to the panel colour, `resize`/`destroy` work. Verified by a temporary host snippet in Step 3.

- [ ] **Step 1: Implement the skeleton**

```ts
import type { InstrumentRenderer, InstrumentState, RendererOpts } from "../renderer";
import { DEFAULT_PALETTE, type Palette } from "../core/palette";
import { acquireDevice } from "./device";

export class RawInstrumentRenderer implements InstrumentRenderer {
  private device!: GPUDevice;
  private context!: GPUCanvasContext;
  private format!: GPUTextureFormat;
  private palette: Palette = DEFAULT_PALETTE;
  private width = 1;
  private height = 1;

  async init(canvas: HTMLCanvasElement, opts: RendererOpts): Promise<void> {
    const res = await acquireDevice(canvas);
    if (!res.ok) throw new Error(res.reason);
    this.device = res.device;
    this.context = res.context;
    this.format = res.format;
    this.palette = { ...DEFAULT_PALETTE, ...(opts.colors ?? {}) };
  }

  resize(width: number, height: number, dpr: number): void {
    this.width = Math.max(1, Math.round(width * dpr));
    this.height = Math.max(1, Math.round(height * dpr));
    // canvas backing size is set by the host; nothing else needed for the clear pass yet.
  }

  render(_state: InstrumentState): void {
    const [r, g, b] = this.palette.panel;
    const encoder = this.device.createCommandEncoder();
    const view = this.context.getCurrentTexture().createView();
    const pass = encoder.beginRenderPass({
      colorAttachments: [{ view, clearValue: { r, g, b, a: 1 }, loadOp: "clear", storeOp: "store" }],
    });
    pass.end();
    this.device.queue.submit([encoder.finish()]);
  }

  destroy(): void {
    this.device?.destroy?.();
  }
}
```

- [ ] **Step 2: Type check** — `pnpm exec tsc --noEmit` → clean.

- [ ] **Step 3: Manual visual smoke (temporary)**

Temporarily wire the renderer into `InstrumentCanvas` is Task 11; for now verify via a throwaway in the browser console on any page after `pnpm dev`:
```js
const c = document.createElement("canvas"); c.width = 400; c.height = 300; document.body.append(c);
const { RawInstrumentRenderer } = await import("/src/instrument/raw/raw-renderer.ts");
const r = new RawInstrumentRenderer(); await r.init(c, {}); r.resize(400,300,1); r.render({});
```
Expected: a dark (panel-colour) rectangle appears. Remove the canvas afterward (`c.remove()`).

- [ ] **Step 4: Commit**

```bash
git add client/src/instrument/raw/raw-renderer.ts
git commit -m "feat(client/instrument): raw renderer skeleton with clear pass"
```

---

## Task 7: Instrument pass — fullscreen SDF, tach ring first

**Files:**
- Create: `client/src/instrument/raw/passes/instruments.wgsl`
- Modify: `client/src/instrument/raw/raw-renderer.ts`

Approach: one fullscreen-triangle vertex stage + a fragment stage that draws instruments analytically from a uniform block. Start with just the tach ring so the pipeline is proven, then add instruments in Task 8.

- [ ] **Step 1: Write `instruments.wgsl` (uniforms + fullscreen tri + tach ring)**

```wgsl
struct Uniforms {
  resolution : vec2f,
  sweepStart : f32,   // radians
  sweepExtent: f32,   // radians
  rpmFrac    : f32,   // 0..1 fill of the ring
  redline    : f32,   // 0..1 glow factor
  speedAngle : f32,
  throttle   : f32,
  brake      : f32,
  gx         : f32,
  gy         : f32,
  _pad       : f32,
  ringLow    : vec4f,
  ringMid    : vec4f,
  ringRed    : vec4f,
  panel      : vec4f,
  tick       : vec4f,
};
@group(0) @binding(0) var<uniform> u : Uniforms;

@vertex
fn vs(@builtin(vertex_index) i : u32) -> @builtin(position) vec4f {
  // fullscreen triangle
  var p = array<vec2f,3>(vec2f(-1.,-1.), vec2f(3.,-1.), vec2f(-1.,3.));
  return vec4f(p[i], 0., 1.);
}

const PI = 3.14159265;

// signed distance to an annulus arc; returns <=0 inside the band within [a0,a1]
fn arc(p: vec2f, r0: f32, r1: f32, a0: f32, a1: f32) -> f32 {
  let radius = length(p);
  var ang = atan2(p.y, p.x);
  if (ang < 0.) { ang = ang + 2.*PI; }
  let band = max(r0 - radius, radius - r1);
  // angular mask: distance outside [a0,a1] (a1 may exceed 2π; normalize)
  let aa = a0 % (2.*PI);
  let span = a1 - a0;
  var rel = ang - aa; if (rel < 0.) { rel = rel + 2.*PI; }
  let outside = max(-rel, rel - span);
  return max(band, outside * radius);
}

@fragment
fn fs(@builtin(position) frag : vec4f) -> @location(0) vec4f {
  let res = u.resolution;
  let mn = min(res.x, res.y);
  // normalized coords centred on the gauge (cx,cy from spec baked by renderer via uniforms? use 0.32,0.5)
  let center = vec2f(0.32, 0.5) * res;
  let p = (frag.xy - center) / mn; // y down
  var col = u.panel.rgb;

  // tach ring band
  let r0 = 0.42; let r1 = 0.46;
  let dBand = arc(p, r0, r1, u.sweepStart, u.sweepStart + u.sweepExtent);
  // fill up to rpmFrac along the sweep
  var ang = atan2(p.y, p.x); if (ang < 0.) { ang = ang + 2.*PI; }
  var rel = ang - (u.sweepStart % (2.*PI)); if (rel < 0.) { rel = rel + 2.*PI; }
  let frac = rel / u.sweepExtent;
  let filled = step(frac, u.rpmFrac);
  // colour ramp along the sweep
  var ramp = mix(u.ringLow.rgb, u.ringMid.rgb, smoothstep(0.0, 0.7, frac));
  ramp = mix(ramp, u.ringRed.rgb, smoothstep(0.82, 1.0, frac));
  let aa = 1.5 / mn;
  let band = 1.0 - smoothstep(0.0, aa, dBand);
  let trackCol = mix(u.panel.rgb, vec3f(0.08,0.10,0.13), 1.0);
  col = mix(col, mix(trackCol, ramp, filled), band);
  return vec4f(col, 1.0);
}
```

- [ ] **Step 2: Build the pipeline + uniform buffer in `raw-renderer.ts`**

Add imports and members:
```ts
import instrumentsWGSL from "./passes/instruments.wgsl?raw";
import { SPEC } from "../core/spec";
```
Add fields: `private pipeline!: GPURenderPipeline; private uniformBuf!: GPUBuffer; private bindGroup!: GPUBindGroup;` and a `Float32Array` CPU mirror sized to the uniform struct (round up to 16-byte alignment — here 24 floats fits; allocate 32 floats / 128 bytes to be safe).

In `init`, after device setup:
```ts
const module = this.device.createShaderModule({ code: instrumentsWGSL });
this.uniformBuf = this.device.createBuffer({
  size: 128, usage: GPUBufferUsage.UNIFORM | GPUBufferUsage.COPY_DST,
});
this.pipeline = this.device.createRenderPipeline({
  layout: "auto",
  vertex: { module, entryPoint: "vs" },
  fragment: { module, entryPoint: "fs", targets: [{ format: this.format }] },
  primitive: { topology: "triangle-list" },
});
this.bindGroup = this.device.createBindGroup({
  layout: this.pipeline.getBindGroupLayout(0),
  entries: [{ binding: 0, resource: { buffer: this.uniformBuf } }],
});
```

Replace `render` body to pack uniforms + draw 3 verts:
```ts
render(state: InstrumentState): void {
  const RAD = Math.PI / 180;
  const u = new Float32Array(32);
  u[0] = this.width; u[1] = this.height;
  u[2] = SPEC.sweep.startDeg * RAD; u[3] = SPEC.sweep.extentDeg * RAD;
  // rpmFrac is recoverable from rpmAngle: (angle - start)/extent
  u[4] = (state.rpmAngle - SPEC.sweep.startDeg * RAD) / (SPEC.sweep.extentDeg * RAD);
  u[5] = state.redline;
  u[6] = state.speedAngle; u[7] = state.throttle; u[8] = state.brake;
  u[9] = state.gx; u[10] = state.gy;
  const p = this.palette;
  u.set([...p.ringLow, 0], 12);
  u.set([...p.ringMid, 0], 16);
  u.set([...p.ringRed, 0], 20);
  u.set([...p.panel, 1], 24);
  u.set([...p.tick, 1], 28);
  this.device.queue.writeBuffer(this.uniformBuf, 0, u);

  const encoder = this.device.createCommandEncoder();
  const view = this.context.getCurrentTexture().createView();
  const pass = encoder.beginRenderPass({
    colorAttachments: [{ view, clearValue: { r: p.panel[0], g: p.panel[1], b: p.panel[2], a: 1 }, loadOp: "clear", storeOp: "store" }],
  });
  pass.setPipeline(this.pipeline);
  pass.setBindGroup(0, this.bindGroup);
  pass.draw(3);
  pass.end();
  this.device.queue.submit([encoder.finish()]);
}
```
(Note: the WGSL `Uniforms` field offsets must match this packing — `resolution` at floats 0–1, scalars 2–10, then vec4s starting at float 12/16/20/24/28. Keep them in sync; `_pad` at index 11.)

- [ ] **Step 3: Type check** — `pnpm exec tsc --noEmit` → clean.

- [ ] **Step 4: Visual check** — repeat the Task 6 Step 3 console snippet but call `r.render({ rpmAngle: (135+200)*Math.PI/180, redline:0, speedAngle:0, throttle:0, brake:0, gx:0, gy:0, speedKmh:0, rpm:0, gear:'3' })`. Expected: a teal→amber arc filling ~74% of a 270° ring with a bottom notch. Iterate on the shader until the ring reads correctly.

- [ ] **Step 5: Commit**

```bash
git add client/src/instrument/raw/passes/instruments.wgsl client/src/instrument/raw/raw-renderer.ts
git commit -m "feat(client/instrument): instrument pass with tach ring (SDF)"
```

---

## Task 8: Instrument pass — speed dial + ticks + needle, gear tile, input bars, g-circle

**Files:**
- Modify: `client/src/instrument/raw/passes/instruments.wgsl`

Add the remaining analog instruments to the fragment shader, each as an SDF contribution composited over the panel. Add these helper SDFs and draw calls **incrementally, visual-checking after each** (each is its own commit-worthy increment, but they share one file — commit per instrument group).

- [ ] **Step 1: Speed dial base + tick rings + needle**

Add to `instruments.wgsl` (helpers near `arc`):
```wgsl
fn circleSDF(p: vec2f, r: f32) -> f32 { return length(p) - r; }

// tick marks: bright where angle is near a tick within the sweep and radius in [r0,r1]
fn ticks(p: vec2f, r0: f32, r1: f32, a0: f32, ext: f32, everyRad: f32, halfWidth: f32) -> f32 {
  let radius = length(p);
  if (radius < r0 || radius > r1) { return 0.; }
  var ang = atan2(p.y, p.x); if (ang < 0.) { ang = ang + 2.*PI; }
  var rel = ang - (a0 % (2.*PI)); if (rel < 0.) { rel = rel + 2.*PI; }
  if (rel > ext) { return 0.; }
  let m = rel % everyRad;
  let d = min(m, everyRad - m);
  return 1.0 - smoothstep(halfWidth*0.5, halfWidth, d);
}

// rounded line segment SDF for the needle
fn segSDF(p: vec2f, a: vec2f, b: vec2f, r: f32) -> f32 {
  let pa = p - a; let ba = b - a;
  let h = clamp(dot(pa,ba)/dot(ba,ba), 0., 1.);
  return length(pa - ba*h) - r;
}
```
In `fs`, after the ring, before `return`:
```wgsl
  // speed dial face
  let dial = circleSDF(p, 0.40);
  col = mix(col, vec3f(0.13,0.15,0.20), 1.0 - smoothstep(0., aa, dial));
  // minor + major ticks (13.5° / 54°), radii from spec
  let minorT = ticks(p, 0.34, 0.38, u.sweepStart, u.sweepExtent, 13.5*PI/180., 0.6*PI/180.);
  let majorT = ticks(p, 0.32, 0.38, u.sweepStart, u.sweepExtent, 54.0*PI/180., 1.2*PI/180.);
  col = mix(col, u.tick.rgb, max(minorT, majorT) * 0.9);
  // needle
  let tip = vec2f(cos(u.speedAngle), sin(u.speedAngle)) * 0.34;
  let needle = segSDF(p, vec2f(0.,0.), tip, 0.006);
  col = mix(col, u.ringRed.rgb, 1.0 - smoothstep(0., aa, needle));
  // hub
  col = mix(col, vec3f(0.23,0.26,0.30), 1.0 - smoothstep(0., aa, circleSDF(p, 0.02)));
```

Visual check (console snippet, vary `speedAngle`); needle should sweep with the same notch. Commit:
```bash
git add client/src/instrument/raw/passes/instruments.wgsl
git commit -m "feat(client/instrument): speed dial, ticks, needle"
```

- [ ] **Step 2: Gear tile + throttle/brake bars + g-circle (rail)**

The rail is right of the gauge. Add a second coordinate frame in `fs` using rail positions from `SPEC.rail` (baked as constants here to match `spec.ts`):
```wgsl
fn roundRect(p: vec2f, half: vec2f, r: f32) -> f32 {
  let q = abs(p) - half + vec2f(r);
  return min(max(q.x,q.y),0.) + length(max(q,vec2f(0.))) - r;
}
```
```wgsl
  // rail uses canvas-normalized coords (not gauge-centred)
  let mnv = min(res.x, res.y);
  let q = (frag.xy - vec2f(0.78,0.0)*res) / mnv; // x relative to rail.x, y absolute/mn
  let qy = frag.y / mnv;
  // gear tile at cy 0.22
  let gear = roundRect(vec2f(q.x, qy - 0.22*res.y/mnv), vec2f(0.08,0.08), 0.03);
  col = mix(col, vec3f(0.10,0.13,0.18), 1.0 - smoothstep(0., aa, gear));
  // throttle bar (left of pair) fill from bottom
  let barH = 0.13; let cyBars = 0.5*res.y/mnv;
  let thrX = -0.025; let brkX = 0.025;
  let inThr = roundRect(vec2f(q.x-thrX, qy-cyBars), vec2f(0.015, barH), 0.012);
  let thrFillTop = cyBars + barH - u.throttle*2.0*barH;
  let thrLit = step(thrFillTop, qy) * (1.0 - smoothstep(0., aa, inThr));
  col = mix(col, vec3f(0.09,0.10,0.13), 1.0 - smoothstep(0., aa, inThr));
  col = mix(col, u.throttle*0. + vec3f(0.21,0.82,0.48), thrLit);
  // brake bar
  let inBrk = roundRect(vec2f(q.x-brkX, qy-cyBars), vec2f(0.015, barH), 0.012);
  let brkFillTop = cyBars + barH - u.brake*2.0*barH;
  let brkLit = step(brkFillTop, qy) * (1.0 - smoothstep(0., aa, inBrk));
  col = mix(col, vec3f(0.09,0.10,0.13), 1.0 - smoothstep(0., aa, inBrk));
  col = mix(col, vec3f(1.0,0.35,0.30), brkLit);
  // g-circle at cy 0.8 with crosshair + dot
  let gcY = 0.8*res.y/mnv;
  let gc = abs(circleSDF(vec2f(q.x, qy-gcY), 0.10)) - 0.002;
  col = mix(col, vec3f(0.30,0.33,0.40), 1.0 - smoothstep(0., aa, gc));
  let dotPos = vec2f(q.x - u.gx*0.085, qy - gcY - u.gy*0.085);
  let gdot = circleSDF(dotPos, 0.012);
  col = mix(col, vec3f(0.61,0.55,1.0), 1.0 - smoothstep(0., aa, gdot));
```
(These rail constants mirror `SPEC.rail`; keep them consistent if `spec.ts` changes. The colours mirror `palette.ts`; if you prefer, add `throttle`/`brake`/`gearGlow`/`gDot` to the uniform block and reference `u.*` instead of literals — recommended for DRY, but inline is acceptable for this pass.)

Visual check; commit:
```bash
git add client/src/instrument/raw/passes/instruments.wgsl
git commit -m "feat(client/instrument): gear tile, throttle/brake bars, g-force circle"
```

---

## Task 9: Bloom post pass

**Files:**
- Create: `client/src/instrument/raw/passes/bloom.wgsl`
- Modify: `client/src/instrument/raw/raw-renderer.ts`

Render the instrument pass to an offscreen texture, then: bright-pass (keep redline ring + needle + glowing fills) → horizontal blur → vertical blur → additive composite to the swapchain.

- [ ] **Step 1: Change the instrument pass to render to an offscreen `rgba16float` texture** (allocate in `resize`, recreate on size change). Add a sampler.

- [ ] **Step 2: Write `bloom.wgsl`** with three fragment entry points sharing the fullscreen-tri vs:
```wgsl
@group(0) @binding(0) var samp : sampler;
@group(0) @binding(1) var tex : texture_2d<f32>;
struct BP { texel: vec2f, dir: vec2f, threshold: f32, intensity: f32, _p: vec2f };
@group(0) @binding(2) var<uniform> b : BP;

@vertex fn vs(@builtin(vertex_index) i:u32)->@builtin(position) vec4f {
  var p = array<vec2f,3>(vec2f(-1.,-1.),vec2f(3.,-1.),vec2f(-1.,3.));
  return vec4f(p[i],0.,1.);
}
fn uv(fc: vec4f, texel: vec2f) -> vec2f { return fc.xy * texel; }

@fragment fn bright(@builtin(position) fc: vec4f) -> @location(0) vec4f {
  let c = textureSample(tex, samp, uv(fc, b.texel)).rgb;
  let l = max(c.r, max(c.g, c.b));
  let k = max(0., l - b.threshold) / max(1e-3, 1.0 - b.threshold);
  return vec4f(c * k, 1.);
}
@fragment fn blur(@builtin(position) fc: vec4f) -> @location(0) vec4f {
  let w = array<f32,5>(0.227, 0.194, 0.121, 0.054, 0.016);
  var sum = textureSample(tex, samp, uv(fc, b.texel)).rgb * w[0];
  for (var i=1; i<5; i++) {
    let o = b.dir * f32(i);
    sum += textureSample(tex, samp, uv(fc, b.texel) + o*b.texel).rgb * w[i];
    sum += textureSample(tex, samp, uv(fc, b.texel) - o*b.texel).rgb * w[i];
  }
  return vec4f(sum, 1.);
}
@fragment fn composite(@builtin(position) fc: vec4f) -> @location(0) vec4f {
  // tex bound = scene; bloom added via a second bind group in a separate draw is simpler —
  // here composite samples the blurred bloom and the host draws scene first with loadOp:load.
  let bloom = textureSample(tex, samp, uv(fc, b.texel)).rgb * b.intensity;
  return vec4f(bloom, 1.); // additive blend state in the pipeline
}
```

- [ ] **Step 3: Wire passes in `raw-renderer.ts`** — ping-pong two half-res textures for blur; composite pipeline uses additive blend (`{color:{srcFactor:"one",dstFactor:"one"}}`) drawing over the scene (scene blitted/!rendered to swapchain first, then bloom added). Keep blur half-resolution for cost.

- [ ] **Step 4: Visual check** — at redline the ring + needle should glow softly; mid-range should not bloom. Tune `threshold` (~0.6) and `intensity` (~0.8).

- [ ] **Step 5: Commit**

```bash
git add client/src/instrument/raw/passes/bloom.wgsl client/src/instrument/raw/raw-renderer.ts
git commit -m "feat(client/instrument): bloom post pass for redline/needle glow"
```

---

## Task 10: MSDF text — atlas asset, layout, glyph pass

**Files:**
- Create: `client/src/instrument/core/msdf/atlas.png`, `atlas.json` (generated offline)
- Create: `client/src/instrument/core/msdf/layout.ts`
- Test: `client/src/instrument/core/msdf/layout.test.ts`
- Create: `client/src/instrument/raw/passes/glyphs.wgsl`
- Modify: `client/src/instrument/raw/raw-renderer.ts`

- [ ] **Step 1: Generate the MSDF atlas offline**

Install the generator and produce an atlas covering `0123456789RN.-` plus the label strings' letters (`KMH RPM THRBKG /`):
```bash
# one-time, not wired into the build
npx msdf-bmfont-xml -f json -o client/src/instrument/core/msdf/atlas \
  -s 42 -t msdf -i "0123456789RN.-KMHRPTBG/ " <path-to-a-condensed-sans.ttf>
```
This writes `atlas.png` + `atlas.json` (glyph `chars` with `x,y,width,height,xoffset,yoffset,xadvance` and `common.scaleW/scaleH`). Commit the two assets. (If `msdf-bmfont-xml` is unavailable, `msdf-atlas-gen` produces an equivalent JSON; adapt field names in `layout.ts`.)

- [ ] **Step 2: Write failing test for `layout.ts`**

`layout.test.ts`:
```ts
import { expect, test } from "vitest";
import { layoutText, type MsdfAtlas } from "./layout";

const atlas: MsdfAtlas = {
  scaleW: 100, scaleH: 100,
  glyphs: {
    "1": { x: 0, y: 0, width: 10, height: 20, xoffset: 0, yoffset: 0, xadvance: 12 },
    "2": { x: 10, y: 0, width: 10, height: 20, xoffset: 0, yoffset: 0, xadvance: 12 },
  },
};

test("layoutText advances per glyph and emits a quad each", () => {
  const quads = layoutText("12", atlas, 1);
  expect(quads).toHaveLength(2);
  expect(quads[0].x).toBeCloseTo(0);
  expect(quads[1].x).toBeCloseTo(12); // second glyph advanced by xadvance
});

test("unknown glyph is skipped but still advances space", () => {
  const quads = layoutText("1 2", atlas, 1);
  expect(quads).toHaveLength(2); // space has no glyph entry
});
```

- [ ] **Step 3: Run → FAIL.**

- [ ] **Step 4: Implement `layout.ts`**

```ts
export interface MsdfGlyph { x: number; y: number; width: number; height: number; xoffset: number; yoffset: number; xadvance: number; }
export interface MsdfAtlas { scaleW: number; scaleH: number; glyphs: Record<string, MsdfGlyph>; }
export interface GlyphQuad { x: number; y: number; w: number; h: number; u0: number; v0: number; u1: number; v1: number; }

/** Lay out `text` left-to-right at unit `scale` (px per atlas unit). Origin at baseline-left. */
export function layoutText(text: string, atlas: MsdfAtlas, scale: number): GlyphQuad[] {
  const quads: GlyphQuad[] = [];
  let penX = 0;
  for (const ch of text) {
    const g = atlas.glyphs[ch];
    if (!g) { penX += 6 * scale; continue; } // space/unknown: nominal advance
    quads.push({
      x: penX + g.xoffset * scale,
      y: g.yoffset * scale,
      w: g.width * scale,
      h: g.height * scale,
      u0: g.x / atlas.scaleW,
      v0: g.y / atlas.scaleH,
      u1: (g.x + g.width) / atlas.scaleW,
      v1: (g.y + g.height) / atlas.scaleH,
    });
    penX += g.xadvance * scale;
  }
  return quads;
}
```

- [ ] **Step 5: Run → PASS.**

- [ ] **Step 6: Write `glyphs.wgsl` + wire the glyph pass**

MSDF fragment uses the median-of-3 trick:
```wgsl
@group(0) @binding(0) var samp: sampler;
@group(0) @binding(1) var atlas: texture_2d<f32>;
struct V { @builtin(position) pos: vec4f, @location(0) uv: vec2f, @location(1) color: vec4f };
// vertex buffer: per-quad instanced (pos rect in clip space + uv rect + colour) — packed by renderer
fn median(v: vec3f) -> f32 { return max(min(v.r,v.g), min(max(v.r,v.g), v.b)); }
@fragment fn fs(in: V) -> @location(0) vec4f {
  let s = textureSample(atlas, samp, in.uv).rgb;
  let d = median(s) - 0.5;
  let aa = fwidth(median(s));
  let a = clamp(d/aa + 0.5, 0., 1.);
  return vec4f(in.color.rgb, in.color.a * a);
}
```
In the renderer: load `atlas.png` into a `GPUTexture` (via `createImageBitmap` + `copyExternalImageToTexture`), build an instanced quad pipeline (alpha blend), and each frame produce quads for: speed value (`String(Math.round(state.speedKmh))`), gear (`state.gear`), `rpm` value, and the static labels (`KM/H`, `RPM`, `THR`, `BRK`, `G`). Positions come from `SPEC` (dial centre for speed/rpm, rail for gear/labels). Convert glyph-space → clip-space using canvas size.

- [ ] **Step 7: Visual check** — numbers render crisp; gear shows `3`, speed updates with `speedKmh`. Tune sizes.

- [ ] **Step 8: Commit**

```bash
git add client/src/instrument/core/msdf/ client/src/instrument/raw/passes/glyphs.wgsl client/src/instrument/raw/raw-renderer.ts client/src/instrument/core/msdf/layout.test.ts
git commit -m "feat(client/instrument): MSDF text — atlas, layout, glyph pass"
```

---

## Task 11: React host — `InstrumentCanvas.tsx`

**Files:**
- Create: `client/src/instrument/InstrumentCanvas.tsx`

- [ ] **Step 1: Implement the host**

```tsx
import { useEffect, useRef, useState } from "react";
import { useLiveStore } from "~/utils/live-store";
import { RawInstrumentRenderer } from "./raw/raw-renderer";
import { makeSmoother, stepSmoother, type Smoother } from "./core/physics";
import { targetsFromTick, buildInstrumentState } from "./core/state";

const STIFF = 90;

export function InstrumentCanvas() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const renderer = new RawInstrumentRenderer();
    let raf = 0;
    let disposed = false;

    // animated channels
    const ch: Record<string, Smoother> = {
      speed: makeSmoother(0), rpm: makeSmoother(0),
      thr: makeSmoother(0), brk: makeSmoother(0),
      gx: makeSmoother(0), gy: makeSmoother(0),
    };
    let last = performance.now();
    let gear = "N";
    let rmx = 8000;

    const sizeToBox = () => {
      const dpr = window.devicePixelRatio || 1;
      const r = canvas.getBoundingClientRect();
      canvas.width = Math.max(1, Math.round(r.width * dpr));
      canvas.height = Math.max(1, Math.round(r.height * dpr));
      renderer.resize(r.width, r.height, dpr);
    };

    const loop = () => {
      try {
        const now = performance.now();
        const dt = Math.min(0.05, (now - last) / 1000); last = now;
        const t = useLiveStore.getState().latest;
        if (t) {
          const tg = targetsFromTick(t, rmx); rmx = tg.rmx; gear = tg.gear;
          ch.speed = stepSmoother(ch.speed, tg.speedKmh, dt, STIFF);
          ch.rpm = stepSmoother(ch.rpm, tg.rpm, dt, STIFF);
          ch.thr = stepSmoother(ch.thr, tg.throttle, dt, STIFF);
          ch.brk = stepSmoother(ch.brk, tg.brake, dt, STIFF);
          ch.gx = stepSmoother(ch.gx, tg.gx, dt, STIFF);
          ch.gy = stepSmoother(ch.gy, tg.gy, dt, STIFF);
        }
        const state = buildInstrumentState({
          speedKmh: ch.speed.value, rpm: ch.rpm.value,
          throttle: ch.thr.value, brake: ch.brk.value,
          gx: ch.gx.value, gy: ch.gy.value, gear, rmx,
        });
        renderer.render(state);
      } catch (err) {
        if (!disposed) console.error("instrument render failed; loop continues", err);
      } finally {
        raf = requestAnimationFrame(loop);
      }
    };

    const ro = new ResizeObserver(sizeToBox);

    renderer
      .init(canvas, {})
      .then(() => { if (disposed) return; sizeToBox(); ro.observe(canvas); raf = requestAnimationFrame(loop); })
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));

    return () => { disposed = true; cancelAnimationFrame(raf); ro.disconnect(); renderer.destroy(); };
  }, []);

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 rounded-2xl bg-surface p-10 text-center shadow-surface">
        <span className="text-sm font-medium text-foreground">WebGPU required</span>
        <span className="text-xs text-muted">{error}</span>
        <a href="/live" className="text-xs text-accent underline">Use the HUD view instead</a>
      </div>
    );
  }
  return <canvas ref={canvasRef} className="aspect-[16/9] w-full rounded-2xl bg-surface shadow-surface" />;
}
```

- [ ] **Step 2: Type check** — `pnpm exec tsc --noEmit` → clean.

- [ ] **Step 3: Commit**

```bash
git add client/src/instrument/InstrumentCanvas.tsx
git commit -m "feat(client/instrument): react host with rAF physics loop + webgpu notice"
```

---

## Task 12: Route + HUD/Instrument toggle

**Files:**
- Create: `client/src/routes/live.instrument.tsx`
- Modify: `client/src/routes/live.tsx`

- [ ] **Step 1: Create `routes/live.instrument.tsx`**

Follow the existing route module pattern in `routes/live.tsx` (inspect it first for the `createFileRoute` import and export shape). Skeleton:
```tsx
import { createFileRoute } from "@tanstack/react-router";
import { InstrumentCanvas } from "~/instrument/InstrumentCanvas";
import { LiveViewToggle } from "./live"; // export the toggle from live.tsx in Step 2

export const Route = createFileRoute("/live/instrument")({ component: InstrumentRoute });

function InstrumentRoute() {
  return (
    <section className="flex flex-col gap-8">
      <header className="flex items-baseline justify-between gap-4">
        <div className="flex flex-col gap-1">
          <span className="text-xs font-medium tracking-wider text-muted uppercase">Realtime</span>
          <h1 className="text-3xl font-semibold tracking-tight text-foreground">Instrument</h1>
        </div>
        <LiveViewToggle active="instrument" />
      </header>
      <InstrumentCanvas />
    </section>
  );
}
```
(Confirm the file-route path string TanStack expects for a nested `live/instrument` — match the project's existing nested-route convention.)

- [ ] **Step 2: Add + export `LiveViewToggle` in `live.tsx`, render it in the HUD header**

Add a small segmented control and render it in the existing `LiveRoute` header next to `StatusPill`:
```tsx
import { Link } from "@tanstack/react-router";

export function LiveViewToggle({ active }: { active: "hud" | "instrument" }) {
  const base = "rounded-lg px-3 py-1 text-xs font-medium";
  return (
    <div className="flex gap-1 rounded-xl bg-surface p-1 shadow-surface">
      <Link to="/live" className={`${base} ${active === "hud" ? "bg-accent-soft text-foreground" : "text-muted"}`}>HUD</Link>
      <Link to="/live/instrument" className={`${base} ${active === "instrument" ? "bg-accent-soft text-foreground" : "text-muted"}`}>Instrument</Link>
    </div>
  );
}
```
In `LiveRoute`'s header, replace the lone `<StatusPill .../>` with a flex row containing `<LiveViewToggle active="hud" />` and the existing `<StatusPill .../>`.

- [ ] **Step 3: Type check + lint** — `pnpm exec tsc --noEmit` (clean) and `pnpm lint` (no new issues in your files).

- [ ] **Step 4: Commit**

```bash
git add client/src/routes/live.instrument.tsx client/src/routes/live.tsx
git commit -m "feat(client): /live/instrument route + HUD/Instrument toggle"
```

---

## Task 13: Visual verification with live/synthetic data

**Files:** none (verification only).

- [ ] **Step 1: Run dev** — `cd client && pnpm dev` (and the Go server if you want real data; otherwise inject).

- [ ] **Step 2: Navigate + inject** — via Chrome DevTools MCP, open `http://localhost:3000/live/instrument`. Inject synthetic `useLiveStore` state across the range (idle, mid, redline) using the pattern from the sparkline verification (build full `TickFrame` objects; set `store.setState({ latest })` and optionally neutralise `push`). Screenshot at:
  - idle (rpm ~900, speed 0)
  - mid (rpm ~4000, speed ~120, throttle 1, some `lg`)
  - redline (rpm ≥ 0.9·rmx) — confirm ring glow blooms, needle sweeps, gear/speed text crisp, g-dot offset.

- [ ] **Step 3: Confirm fallback** — in a non-WebGPU context (or temporarily force `acquireDevice` to fail), confirm the "WebGPU required" notice + HUD link render.

- [ ] **Step 4: Confirm no HUD regression** — `/live` still renders the DOM HUD with the new toggle.

- [ ] **Step 5: Final full check** — `cd client && pnpm exec tsc --noEmit && pnpm test && pnpm lint` (test green; lint shows only pre-existing warnings in other files). No commit (verification task), unless tuning changes were made — commit those per the instrument/pass they touch.

---

## Self-review notes (addressed)

- **Spec coverage:** concentric tach ring + speed dial/ticks/needle/digital (Tasks 7–8, 10), gear/throttle/brake/g (Task 8, 10), all-WebGPU incl. text (Task 10), bloom/spring fidelity (Tasks 9, 3, 11), `/live/instrument` + toggle (Task 12), WebGPU-required notice (Task 11), shared core + interface (Tasks 1–4), Vitest (Task 0), WGSL `?raw` (Task 0), MSDF assets (Task 10), visual verification (Task 13). Phase 2 (Three.js) intentionally excluded.
- **Type consistency:** `InstrumentState`/`Targets`/`Smoother`/`InstrumentRenderer`/`Palette`/`MsdfAtlas` names are used identically across tasks; uniform float offsets in Task 7 match the WGSL `Uniforms` struct.
- **Known iterative areas (not placeholders — visual tuning is expected):** shader appearance in Tasks 7–10 is brought up with real first-pass code then tuned against screenshots; bloom threshold/intensity and text sizes are explicitly tuned in their visual-check steps.
```
