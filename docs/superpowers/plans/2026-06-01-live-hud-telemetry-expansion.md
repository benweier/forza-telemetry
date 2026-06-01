# Live HUD Telemetry Expansion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface per-wheel + engine telemetry in the `/live` HUD as glanceable visualisations — a combined car diagram, a live dyno curve, an engine badge, and a boost gauge — within the existing Glass design language.

**Architecture:** Pure, unit-tested helper modules (`tire-scale.ts`, `engine.ts`) hold all classification/scaling/accumulation logic. Thin React/SVG/canvas components read live `TickFrame` data exactly as the existing HUD does (`latest` prop + `useLiveStore` ring for rAF-driven canvases). No server or schema changes — every field already exists on `TickFrame`.

**Tech Stack:** TypeScript, React 19, TanStack Start, Vitest, Tailwind v4 + HeroUI (Glass theme semantic tokens), SVG + Canvas 2D.

---

## File Structure

**New (pure logic + co-located tests, mirroring `src/instrument/core/`):**
- `client/src/components/hud/tire-scale.ts` — grip-window heat colour, combined-slip ring style, lockup/spin classification, slip-angle tick geometry.
- `client/src/components/hud/tire-scale.test.ts`
- `client/src/components/hud/engine.ts` — drivetrain/cylinder labels, driven-axle, kW conversion, boost fraction, `DynoEnvelope` accumulator.
- `client/src/components/hud/engine.test.ts`

**New (presentational components):**
- `client/src/components/hud/CarDiagram.tsx` — top-down SVG, 4 corners, 4 channels each.
- `client/src/components/hud/DynoCurve.tsx` — canvas power+torque envelope vs RPM (rAF, mirrors `Sparkline`).
- `client/src/components/hud/BoostGauge.tsx` — horizontal vacuum→positive gauge.
- `client/src/components/hud/EngineBadge.tsx` — cylinders · drivetrain pill.

**Modified:**
- `client/src/routes/live.tsx` — 3-column reflow, wire new components, trim `MetaPanel`.
- `docs/data-needed.md` — three open empirical questions.
- `~/.claude/projects/.../memory/data_needed_doc.md` — no change needed; pointer already exists.

All colour comes from semantic Glass tokens via CSS `var(...)` / `color-mix(...)` strings or `getComputedStyle` reads (the `Sparkline` pattern). No inline hex/OKLCH in components.

---

## Task 1: Tire per-wheel pure helpers

**Files:**
- Create: `client/src/components/hud/tire-scale.ts`
- Test: `client/src/components/hud/tire-scale.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
// client/src/components/hud/tire-scale.test.ts
import { expect, test } from "vitest";
import {
  heatScaleColor,
  ringFromCombinedSlip,
  classifyWheelSlip,
  slipAngleTick,
  TEMP_THRESHOLDS,
} from "./tire-scale";

test("heatScaleColor: cold is muted, grip window is success, overheat is danger", () => {
  expect(heatScaleColor(0)).toBe("var(--muted)");
  expect(heatScaleColor(TEMP_THRESHOLDS.cold)).toBe("var(--muted)");
  expect(heatScaleColor(TEMP_THRESHOLDS.optimalLo)).toBe("var(--success)");
  expect(heatScaleColor(TEMP_THRESHOLDS.optimalHi)).toBe("var(--success)");
  expect(heatScaleColor(TEMP_THRESHOLDS.overheat)).toBe("var(--danger)");
  expect(heatScaleColor(9999)).toBe("var(--danger)");
});

test("heatScaleColor: between bands returns a token color-mix", () => {
  const midCool = heatScaleColor((TEMP_THRESHOLDS.cold + TEMP_THRESHOLDS.optimalLo) / 2);
  expect(midCool).toContain("color-mix");
  expect(midCool).toContain("--muted");
  expect(midCool).toContain("--success");
});

test("classifyWheelSlip: negative slip ratio locks, positive spins, small is null", () => {
  expect(classifyWheelSlip(-0.5)).toBe("lock");
  expect(classifyWheelSlip(0.5)).toBe("spin");
  expect(classifyWheelSlip(0)).toBeNull();
  expect(classifyWheelSlip(0.01)).toBeNull();
});

test("ringFromCombinedSlip: widens and reddens as slip exceeds grip", () => {
  const grip = ringFromCombinedSlip(0);
  const limit = ringFromCombinedSlip(1.2);
  expect(grip.strokeWidth).toBeLessThan(limit.strokeWidth);
  expect(grip.color).toBe("var(--muted)");
  expect(limit.color).toContain("--danger");
});

test("slipAngleTick: sign sets direction, magnitude sets length, clamped", () => {
  expect(slipAngleTick(0)).toEqual({ length: 0, dir: 0 });
  expect(slipAngleTick(0.15).dir).toBe(1);
  expect(slipAngleTick(-0.15).dir).toBe(-1);
  expect(slipAngleTick(99).length).toBe(slipAngleTick(0.3).length); // clamped to max
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd client && pnpm exec vitest run src/components/hud/tire-scale.test.ts`
Expected: FAIL — cannot find module `./tire-scale`.

- [ ] **Step 3: Write the implementation**

```ts
// client/src/components/hud/tire-scale.ts
/**
 * Per-wheel tire model for the live HUD car diagram. All colours are emitted as
 * CSS strings built from semantic Glass tokens (var(--muted/--success/--warning/
 * --danger)) so the diagram stays on-palette and theme-driven.
 *
 * NOTE: the numeric thresholds below are PLACEHOLDERS — Forza's tire-temp unit
 * and the slip-ratio/slip-angle scales are empirically unconfirmed. See
 * docs/data-needed.md. Tune these constants when a real capture resolves them.
 */

function clamp01(v: number): number {
  return v < 0 ? 0 : v > 1 ? 1 : v;
}

/** color-mix between two semantic tokens; `pct` is the weight of `b` in 0..1. */
function mix(a: string, b: string, pct: number): string {
  const p = Math.round(clamp01(pct) * 100);
  return `color-mix(in oklab, var(${a}) ${100 - p}%, var(${b}) ${p}%)`;
}

export interface TempThresholds {
  cold: number; // at/below: fully muted
  optimalLo: number; // start of grip window (full success)
  optimalHi: number; // end of grip window (full success)
  hot: number; // warning peak
  overheat: number; // at/above: fully danger
}

export const TEMP_THRESHOLDS: TempThresholds = {
  cold: 50,
  optimalLo: 70,
  optimalHi: 95,
  hot: 110,
  overheat: 120,
};

/** Tire temp → on-palette grip-window colour. Green = in the grip window. */
export function heatScaleColor(temp: number, t: TempThresholds = TEMP_THRESHOLDS): string {
  if (temp <= t.cold) return "var(--muted)";
  if (temp < t.optimalLo) return mix("--muted", "--success", (temp - t.cold) / (t.optimalLo - t.cold));
  if (temp <= t.optimalHi) return "var(--success)";
  if (temp < t.hot) return mix("--success", "--warning", (temp - t.optimalHi) / (t.hot - t.optimalHi));
  if (temp < t.overheat) return mix("--warning", "--danger", (temp - t.hot) / (t.overheat - t.hot));
  return "var(--danger)";
}

export interface RingStyle {
  strokeWidth: number;
  color: string;
}

/** Combined slip ~1.0 = at the grip limit; >1 = exceeding it. */
export const SLIP_RING = {
  grip: 0.8,
  limit: 1.0,
  max: 1.6,
  minWidth: 2,
  maxWidth: 7,
} as const;

export function ringFromCombinedSlip(tcs: number): RingStyle {
  const f = clamp01((tcs - SLIP_RING.grip) / (SLIP_RING.max - SLIP_RING.grip));
  const strokeWidth = SLIP_RING.minWidth + f * (SLIP_RING.maxWidth - SLIP_RING.minWidth);
  let color: string;
  if (tcs < SLIP_RING.grip) {
    color = "var(--muted)";
  } else if (tcs < SLIP_RING.limit) {
    color = mix("--muted", "--warning", (tcs - SLIP_RING.grip) / (SLIP_RING.limit - SLIP_RING.grip));
  } else {
    color = mix("--warning", "--danger", clamp01((tcs - SLIP_RING.limit) / (SLIP_RING.max - SLIP_RING.limit)));
  }
  return { strokeWidth, color };
}

export type WheelSlip = "lock" | "spin" | null;

/** PLACEHOLDER threshold — see docs/data-needed.md. */
export const TSR_SLIP_THRESHOLD = 0.15;

/** Slip ratio < 0 means the wheel turns slower than ground (lockup); > 0 spins. */
export function classifyWheelSlip(tsr: number, threshold = TSR_SLIP_THRESHOLD): WheelSlip {
  if (tsr <= -threshold) return "lock";
  if (tsr >= threshold) return "spin";
  return null;
}

export interface SlipTick {
  length: number;
  dir: -1 | 0 | 1;
}

/** PLACEHOLDERS — slip-angle scale unconfirmed (see docs/data-needed.md). */
export const SLIP_ANGLE_MAX = 0.3; // radians at full-length arrow
export const SLIP_TICK_MAX_PX = 14;

export function slipAngleTick(tsa: number): SlipTick {
  const length = clamp01(Math.abs(tsa) / SLIP_ANGLE_MAX) * SLIP_TICK_MAX_PX;
  const dir = tsa > 0 ? 1 : tsa < 0 ? -1 : 0;
  return { length, dir };
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd client && pnpm exec vitest run src/components/hud/tire-scale.test.ts`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add client/src/components/hud/tire-scale.ts client/src/components/hud/tire-scale.test.ts
git commit -m "feat(client): tire grip-window scale + slip helpers for HUD car diagram"
```

---

## Task 2: Engine / dyno pure helpers

**Files:**
- Create: `client/src/components/hud/engine.ts`
- Test: `client/src/components/hud/engine.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
// client/src/components/hud/engine.test.ts
import { expect, test } from "vitest";
import {
  drivetrainLabel,
  drivenAxle,
  cylinderLabel,
  powerKW,
  boostFraction,
  DynoEnvelope,
} from "./engine";

test("drivetrainLabel / drivenAxle map the Forza enum (0 FWD, 1 RWD, 2 AWD)", () => {
  expect(drivetrainLabel(0)).toBe("FWD");
  expect(drivetrainLabel(1)).toBe("RWD");
  expect(drivetrainLabel(2)).toBe("AWD");
  expect(drivetrainLabel(7)).toBeNull();
  expect(drivenAxle(0)).toBe("front");
  expect(drivenAxle(1)).toBe("rear");
  expect(drivenAxle(2)).toBe("both");
  expect(drivenAxle(7)).toBeNull();
});

test("cylinderLabel only renders for positive counts", () => {
  expect(cylinderLabel(8)).toBe("8-cyl");
  expect(cylinderLabel(0)).toBeNull();
});

test("powerKW converts watts to kilowatts", () => {
  expect(powerKW(412000)).toBe(412);
});

test("boostFraction maps value into 0..1 across the display range, clamped", () => {
  expect(boostFraction(-1, -1, 2)).toBe(0);
  expect(boostFraction(2, -1, 2)).toBe(1);
  expect(boostFraction(0.5, -1, 2)).toBeCloseTo(0.5, 5);
  expect(boostFraction(99, -1, 2)).toBe(1);
});

test("DynoEnvelope keeps the per-RPM-bucket maxima", () => {
  const env = new DynoEnvelope(1000); // 1000-rpm buckets
  env.update(2500, 100_000, 300, 1); // bucket 2 -> 100kW, 300Nm
  env.update(2700, 80_000, 350, 1); // same bucket, lower power, higher torque
  const buckets = env.buckets();
  expect(buckets).toHaveLength(1);
  expect(buckets[0].rpm).toBe(2000);
  expect(buckets[0].powerKW).toBe(100);
  expect(buckets[0].torqueNm).toBe(350);
});

test("DynoEnvelope resets when the car ordinal changes", () => {
  const env = new DynoEnvelope(1000);
  env.update(3000, 100_000, 300, 1);
  expect(env.buckets()).toHaveLength(1);
  env.update(5000, 90_000, 280, 2); // new car -> reset, then record
  const buckets = env.buckets();
  expect(buckets).toHaveLength(1);
  expect(buckets[0].rpm).toBe(5000);
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd client && pnpm exec vitest run src/components/hud/engine.test.ts`
Expected: FAIL — cannot find module `./engine`.

- [ ] **Step 3: Write the implementation**

```ts
// client/src/components/hud/engine.ts
/**
 * Engine + dyno helpers for the live HUD. Power/torque are SI on the wire
 * (watts / Nm). The drivetrain enum follows the Forza convention 0=FWD, 1=RWD,
 * 2=AWD. Boost units are unconfirmed (see docs/data-needed.md) — the gauge shows
 * the raw value across a placeholder display range.
 */

export function drivetrainLabel(dt: number): string | null {
  switch (dt) {
    case 0:
      return "FWD";
    case 1:
      return "RWD";
    case 2:
      return "AWD";
    default:
      return null;
  }
}

export type DrivenAxle = "front" | "rear" | "both" | null;

export function drivenAxle(dt: number): DrivenAxle {
  switch (dt) {
    case 0:
      return "front";
    case 1:
      return "rear";
    case 2:
      return "both";
    default:
      return null;
  }
}

/** Forza telemetry exposes cylinder COUNT but not layout (V/I/flat). */
export function cylinderLabel(ncy: number): string | null {
  return ncy > 0 ? `${ncy}-cyl` : null;
}

export function powerKW(watts: number): number {
  return watts / 1000;
}

function clamp01(v: number): number {
  return v < 0 ? 0 : v > 1 ? 1 : v;
}

export function boostFraction(boost: number, min: number, max: number): number {
  if (max === min) return 0;
  return clamp01((boost - min) / (max - min));
}

export interface DynoBucket {
  rpm: number;
  powerKW: number;
  torqueNm: number;
}

/**
 * Accumulates the maximum power/torque seen per RPM bucket — the live "dyno
 * envelope" that fills in as the player revs. Resets automatically when the car
 * ordinal changes (a new car has a new curve). Buckets are fixed-width and
 * capped well above any real redline so a higher-revving car never clips.
 */
export class DynoEnvelope {
  private readonly maxPower: Float64Array;
  private readonly maxTorque: Float64Array;
  private readonly seen: boolean[];
  private carOrdinal = 0;

  constructor(
    public readonly bucketRpm = 250,
    capRpm = 20_000,
  ) {
    const n = Math.ceil(capRpm / bucketRpm) + 1;
    this.maxPower = new Float64Array(n);
    this.maxTorque = new Float64Array(n);
    this.seen = new Array(n).fill(false);
  }

  reset(): void {
    this.maxPower.fill(0);
    this.maxTorque.fill(0);
    this.seen.fill(false);
  }

  update(rpm: number, powerW: number, torqueNm: number, carOrdinal = 0): void {
    if (carOrdinal !== this.carOrdinal) {
      this.carOrdinal = carOrdinal;
      this.reset();
    }
    if (rpm < 0) return;
    const i = Math.min(this.maxPower.length - 1, Math.floor(rpm / this.bucketRpm));
    const kw = powerW / 1000;
    if (!this.seen[i] || kw > this.maxPower[i]) this.maxPower[i] = kw;
    if (!this.seen[i] || torqueNm > this.maxTorque[i]) this.maxTorque[i] = torqueNm;
    this.seen[i] = true;
  }

  buckets(): DynoBucket[] {
    const out: DynoBucket[] = [];
    for (let i = 0; i < this.maxPower.length; i++) {
      if (this.seen[i]) {
        out.push({ rpm: i * this.bucketRpm, powerKW: this.maxPower[i], torqueNm: this.maxTorque[i] });
      }
    }
    return out;
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd client && pnpm exec vitest run src/components/hud/engine.test.ts`
Expected: PASS (6 tests).

- [ ] **Step 5: Commit**

```bash
git add client/src/components/hud/engine.ts client/src/components/hud/engine.test.ts
git commit -m "feat(client): engine labels + DynoEnvelope accumulator for HUD"
```

---

## Task 3: CarDiagram component

**Files:**
- Create: `client/src/components/hud/CarDiagram.tsx`

No unit test (SVG render) — logic is already covered by Task 1; verify visually in Task 7's gate.

- [ ] **Step 1: Write the component**

```tsx
// client/src/components/hud/CarDiagram.tsx
/* Hallmark · component: car-diagram · genre: dashboard · theme: Glass */
import type { TickFrame } from "~/types/tick.generated";
import {
  classifyWheelSlip,
  heatScaleColor,
  ringFromCombinedSlip,
  slipAngleTick,
} from "./tire-scale";
import { drivenAxle } from "./engine";

/** Per-wheel order is FL, FR, RL, RR everywhere in this codebase. */
const WHEELS = [
  { i: 0, cx: 70, cy: 112, barX: 28, barSide: "left" as const },
  { i: 1, cx: 234, cy: 112, barX: 271, barSide: "right" as const },
  { i: 2, cx: 70, cy: 290, barX: 28, barSide: "left" as const },
  { i: 3, cx: 234, cy: 290, barX: 271, barSide: "right" as const },
];

const WHEEL_W = 36;
const WHEEL_H = 52;
const BAR_W = 5;

function Corner({ tick, wheel }: { tick: TickFrame; wheel: (typeof WHEELS)[number] }) {
  const { i, cx, cy, barX } = wheel;
  const temp = tick.tt[i] ?? 0;
  const ring = ringFromCombinedSlip(tick.tcs[i] ?? 0);
  const slip = classifyWheelSlip(tick.tsr[i] ?? 0);
  const arrow = slipAngleTick(tick.tsa[i] ?? 0);
  // Normalised suspension travel 0..1 → bar fills upward from the wheel centre.
  const travel = Math.max(0, Math.min(1, tick.stn[i] ?? 0));
  const barH = travel * WHEEL_H;

  const x = cx - WHEEL_W / 2;
  const y = cy - WHEEL_H / 2;

  return (
    <g>
      {/* combined-slip ring */}
      <circle
        cx={cx}
        cy={cy}
        r={WHEEL_W / 2 + 6}
        fill="none"
        stroke={ring.color}
        strokeWidth={ring.strokeWidth}
        opacity={0.9}
      />
      {/* tire body, grip-window heat fill */}
      <rect x={x} y={y} width={WHEEL_W} height={WHEEL_H} rx={11} fill={heatScaleColor(temp)} />
      <text x={cx} y={cy + 4} textAnchor="middle" fontSize={11} className="fill-background">
        {Math.round(temp)}°
      </text>
      {/* slip-angle tick from the wheel centre (horizontal, signed) */}
      {arrow.length > 0 && (
        <line
          x1={cx}
          y1={cy - WHEEL_H / 2 - 4}
          x2={cx + arrow.dir * arrow.length}
          y2={cy - WHEEL_H / 2 - 4}
          stroke="var(--muted)"
          strokeWidth={2}
        />
      )}
      {/* lockup / spin tag */}
      {slip && (
        <text
          x={cx}
          y={cy + WHEEL_H / 2 + 14}
          textAnchor="middle"
          fontSize={8}
          style={{ fill: slip === "spin" ? "var(--danger)" : "var(--warning)" }}
        >
          {slip === "spin" ? "SPIN" : "LOCK"}
        </text>
      )}
      {/* suspension-travel side bar */}
      <rect x={barX} y={y} width={BAR_W} height={WHEEL_H} rx={BAR_W / 2} fill="var(--surface-secondary)" />
      <rect
        x={barX}
        y={y + (WHEEL_H - barH)}
        width={BAR_W}
        height={barH}
        rx={BAR_W / 2}
        fill="var(--muted)"
      />
    </g>
  );
}

export function CarDiagram({ tick, fresh }: { tick: TickFrame; fresh: boolean }) {
  const axle = drivenAxle(tick.dt);

  return (
    <div className="flex h-full flex-col gap-3 rounded-2xl bg-surface p-5 shadow-surface">
      <span className="text-xs font-medium tracking-wider text-muted uppercase">Tires &amp; chassis</span>
      <svg
        viewBox="0 0 304 402"
        role="img"
        aria-label="Top-down tire and chassis diagram"
        className="mx-auto my-auto w-full max-w-[300px]"
        style={{ opacity: fresh ? 1 : 0.5 }}
      >
        {/* body + cabin */}
        <rect x={92} y={36} width={120} height={330} rx={42} fill="var(--surface-secondary)" />
        <rect x={118} y={88} width={68} height={112} rx={12} fill="var(--surface-tertiary)" />
        {/* driven-axle highlight */}
        {(axle === "front" || axle === "both") && (
          <rect x={144} y={96} width={16} height={70} rx={6} fill="var(--success-soft)" />
        )}
        {(axle === "rear" || axle === "both") && (
          <rect x={144} y={236} width={16} height={70} rx={6} fill="var(--success-soft)" />
        )}
        {WHEELS.map((w) => (
          <Corner key={w.i} tick={tick} wheel={w} />
        ))}
      </svg>
      <div className="flex flex-wrap gap-x-4 gap-y-1 text-[10px] text-muted">
        <span>Fill: temp</span>
        <span>Ring: slip</span>
        <span>Bar: suspension</span>
        <span>Arrow: slip angle</span>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Type-check**

Run: `cd client && pnpm exec tsc --noEmit`
Expected: no errors. (If the harness shows stale "cannot find module" diagnostics, trust `tsc`.)

- [ ] **Step 3: Commit**

```bash
git add client/src/components/hud/CarDiagram.tsx
git commit -m "feat(client): CarDiagram SVG centerpiece for live HUD"
```

---

## Task 4: DynoCurve component

**Files:**
- Create: `client/src/components/hud/DynoCurve.tsx`

Mirrors the `Sparkline` rAF pattern (`client/src/components/Sparkline.tsx`): an imperative loop reads the live ring, redraws only when a new frame arrives, and survives a bad frame via `try/finally`.

- [ ] **Step 1: Write the component**

```tsx
// client/src/components/hud/DynoCurve.tsx
/* Hallmark · component: dyno-curve · genre: dashboard · theme: Glass */
import { useEffect, useRef } from "react";
import type { TickFrame } from "~/types/tick.generated";
import { useLiveStore } from "~/utils/live-store";
import { DynoEnvelope } from "./engine";
import { EngineBadge } from "./EngineBadge";

/** Live power+torque envelope vs RPM. Power uses --accent, torque --warning. */
export function DynoCurve({ tick }: { tick: TickFrame }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const cs = getComputedStyle(document.documentElement);
    const powerColor = cs.getPropertyValue("--accent").trim() || "#F8F8F9";
    const torqueColor = cs.getPropertyValue("--warning").trim() || "#F5A524";
    const axisColor = cs.getPropertyValue("--separator").trim() || "rgba(255,255,255,0.18)";
    const cursorColor = cs.getPropertyValue("--muted").trim() || "rgba(255,255,255,0.5)";
    const dangerColor = cs.getPropertyValue("--danger").trim() || "#F31260";

    const env = new DynoEnvelope();
    let raf = 0;
    let lastNewest: TickFrame | null = null;
    let loggedError = false;

    const redraw = (t: TickFrame) => {
      const dpr = window.devicePixelRatio || 1;
      const w = canvas.clientWidth;
      const h = canvas.clientHeight;
      if (w === 0 || h === 0) return;
      if (canvas.width !== Math.round(w * dpr) || canvas.height !== Math.round(h * dpr)) {
        canvas.width = Math.round(w * dpr);
        canvas.height = Math.round(h * dpr);
      }
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.clearRect(0, 0, w, h);

      env.update(t.rpm ?? 0, t.pw ?? 0, t.tq ?? 0, t.co ?? 0);
      const buckets = env.buckets();
      const rpmMax = Math.max(t.rmx ?? 0, t.rpm ?? 0, 1);

      const pad = 6;
      const xAt = (rpm: number) => pad + (Math.min(rpm, rpmMax) / rpmMax) * (w - 2 * pad);
      let maxPower = 1;
      let maxTorque = 1;
      for (const b of buckets) {
        if (b.powerKW > maxPower) maxPower = b.powerKW;
        if (b.torqueNm > maxTorque) maxTorque = b.torqueNm;
      }
      const yPower = (kw: number) => h - pad - (kw / maxPower) * (h - 2 * pad);
      const yTorque = (nm: number) => h - pad - (nm / maxTorque) * (h - 2 * pad);

      // redline zone (last 12% of the rev range)
      ctx.fillStyle = dangerColor;
      ctx.globalAlpha = 0.08;
      const redX = xAt(rpmMax * 0.88);
      ctx.fillRect(redX, 0, w - pad - redX, h);
      ctx.globalAlpha = 1;

      // axis baseline
      ctx.strokeStyle = axisColor;
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(pad, h - pad);
      ctx.lineTo(w - pad, h - pad);
      ctx.stroke();

      const drawCurve = (color: string, yOf: (b: number) => number, key: "powerKW" | "torqueNm") => {
        if (buckets.length < 2) return;
        ctx.strokeStyle = color;
        ctx.lineWidth = 2;
        ctx.lineJoin = "round";
        ctx.beginPath();
        buckets.forEach((b, j) => {
          const px = xAt(b.rpm);
          const py = yOf(b[key]);
          if (j === 0) ctx.moveTo(px, py);
          else ctx.lineTo(px, py);
        });
        ctx.stroke();
      };
      drawCurve(powerColor, yPower, "powerKW");
      drawCurve(torqueColor, yTorque, "torqueNm");

      // live cursor at current rpm
      const cx = xAt(t.rpm ?? 0);
      ctx.strokeStyle = cursorColor;
      ctx.setLineDash([3, 3]);
      ctx.beginPath();
      ctx.moveTo(cx, pad);
      ctx.lineTo(cx, h - pad);
      ctx.stroke();
      ctx.setLineDash([]);
    };

    const loop = () => {
      try {
        const ring = useLiveStore.getState().ring;
        if (ring.length > 0) {
          const newest = ring[ring.length - 1];
          if (newest !== lastNewest) {
            lastNewest = newest;
            redraw(newest);
          }
        }
      } catch (error) {
        if (!loggedError) {
          loggedError = true;
          console.error("DynoCurve redraw failed; loop continues", error);
        }
      } finally {
        raf = requestAnimationFrame(loop);
      }
    };
    raf = requestAnimationFrame(loop);
    return () => cancelAnimationFrame(raf);
  }, []);

  return (
    <div className="flex flex-col gap-2 rounded-2xl bg-surface p-5 shadow-surface">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium tracking-wider text-muted uppercase">Power &amp; torque</span>
        <EngineBadge tick={tick} />
      </div>
      <canvas
        ref={canvasRef}
        role="img"
        aria-label="Live power and torque against engine RPM"
        className="h-32 w-full"
      />
      <div className="flex gap-4 text-[10px]">
        <span className="text-accent">■ power (kW)</span>
        <span className="text-warning">■ torque (Nm)</span>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Type-check**

Run: `cd client && pnpm exec tsc --noEmit`
Expected: no errors. (`EngineBadge` is created in Task 5 — if running tasks out of order, do Task 5 first or expect a transient missing-module error here.)

- [ ] **Step 3: Commit**

```bash
git add client/src/components/hud/DynoCurve.tsx
git commit -m "feat(client): live DynoCurve (power+torque envelope vs RPM)"
```

---

## Task 5: EngineBadge component

**Files:**
- Create: `client/src/components/hud/EngineBadge.tsx`

- [ ] **Step 1: Write the component**

```tsx
// client/src/components/hud/EngineBadge.tsx
/* Hallmark · component: engine-badge · genre: dashboard · theme: Glass */
import type { TickFrame } from "~/types/tick.generated";
import { cylinderLabel, drivetrainLabel } from "./engine";

/** Compact identity pill: cylinder count · drivetrain. Renders nothing useful
 *  on FH5/unknown packets where both fields are 0. */
export function EngineBadge({ tick }: { tick: TickFrame }) {
  const parts = [cylinderLabel(tick.ncy), drivetrainLabel(tick.dt)].filter(
    (p): p is string => p !== null,
  );
  if (parts.length === 0) return null;
  return (
    <span className="rounded-full border border-separator bg-surface-secondary px-3 py-1 text-xs font-medium text-foreground">
      {parts.join(" · ")}
    </span>
  );
}
```

- [ ] **Step 2: Type-check**

Run: `cd client && pnpm exec tsc --noEmit`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add client/src/components/hud/EngineBadge.tsx
git commit -m "feat(client): EngineBadge (cylinders + drivetrain pill)"
```

---

## Task 6: BoostGauge component

**Files:**
- Create: `client/src/components/hud/BoostGauge.tsx`

- [ ] **Step 1: Write the component**

```tsx
// client/src/components/hud/BoostGauge.tsx
/* Hallmark · component: boost-gauge · genre: dashboard · theme: Glass */
import type { TickFrame } from "~/types/tick.generated";
import { boostFraction } from "./engine";

/** Display range is a PLACEHOLDER — boost units are unconfirmed (see
 *  docs/data-needed.md). Vacuum end is muted (no blue token in the palette);
 *  positive boost ramps toward --accent. */
const BOOST_MIN = -1;
const BOOST_MAX = 2;

export function BoostGauge({ tick, fresh }: { tick: TickFrame; fresh: boolean }) {
  const boost = tick.bo ?? 0;
  const markerPct = boostFraction(boost, BOOST_MIN, BOOST_MAX) * 100;
  const zeroPct = boostFraction(0, BOOST_MIN, BOOST_MAX) * 100;

  return (
    <div className="flex flex-col gap-2 rounded-2xl bg-surface p-5 shadow-surface">
      <div className="flex items-baseline justify-between">
        <span className="text-xs font-medium tracking-wider text-muted uppercase">Boost</span>
        <span
          className="text-lg font-semibold text-foreground tabular-nums"
          style={{ opacity: fresh ? 1 : 0.5 }}
        >
          {boost >= 0 ? "+" : "−"}
          {Math.abs(boost).toFixed(1)}
        </span>
      </div>
      <div className="relative h-3 overflow-hidden rounded-full bg-surface-secondary">
        {/* zero baseline marker */}
        <span aria-hidden className="absolute top-0 h-full w-px bg-muted/60" style={{ left: `${zeroPct}%` }} />
        {/* positive-boost fill from zero to current */}
        {boost > 0 && (
          <span
            className="absolute top-0 h-full"
            style={{
              left: `${zeroPct}%`,
              width: `${markerPct - zeroPct}%`,
              background: "var(--accent)",
            }}
          />
        )}
        {/* current-value marker */}
        <span
          aria-hidden
          className="absolute -top-0.5 h-4 w-0.5 rounded-full bg-foreground"
          style={{ left: `${markerPct}%` }}
        />
      </div>
      <div className="flex justify-between text-[10px] text-muted">
        <span>vacuum</span>
        <span>0</span>
        <span>max</span>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Type-check**

Run: `cd client && pnpm exec tsc --noEmit`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add client/src/components/hud/BoostGauge.tsx
git commit -m "feat(client): BoostGauge with vacuum/positive zones"
```

---

## Task 7: Wire components into the HUD route

**Files:**
- Modify: `client/src/routes/live.tsx`

- [ ] **Step 1: Add imports**

At the top of `client/src/routes/live.tsx`, after the existing `Sparkline` import (line 6), add:

```tsx
import { BoostGauge } from "~/components/hud/BoostGauge";
import { CarDiagram } from "~/components/hud/CarDiagram";
import { DynoCurve } from "~/components/hud/DynoCurve";
```

- [ ] **Step 2: Replace the `HUD` body grid**

Replace the entire `HUD` function (currently `client/src/routes/live.tsx:125-168`) with:

```tsx
function HUD({ tick, fresh }: { tick: TickFrame; fresh: boolean }) {
  // Speed wire value is m/s; convert for the big readout.
  const kmh = (tick.sp ?? 0) * 3.6;
  // RPM bar, normalised against max.
  const rpm = tick.rpm ?? 0;
  const rpmMax = Math.max(tick.rmx ?? 0, rpm);
  const rpmPct = rpmMax > 0 ? Math.min(1, rpm / rpmMax) : 0;
  const redlinePct = 0.88;

  return (
    <div className="grid items-stretch gap-4 lg:grid-cols-[1.25fr_0.95fr_0.95fr]" data-stale={!fresh}>
      {/* Column 1 — drive inputs */}
      <div className="flex flex-col gap-4">
        <SpeedCard kmh={kmh} gear={tick.g ?? 0} fresh={fresh} />
        <RpmBar rpm={rpm} rpmMax={tick.rmx ?? 0} pct={rpmPct} redlinePct={redlinePct} />
        <div className="grid grid-cols-2 gap-4">
          <InputBar label="Throttle" value={tick.tp ?? 0} tone="success" />
          <InputBar label="Brake" value={tick.bp ?? 0} tone="danger" />
        </div>
        <div className="grid grid-cols-2 gap-4">
          <Sparkline
            label="Speed"
            unit="km/h"
            colorVar="--accent"
            accessor={(t) => (t.sp ?? 0) * 3.6}
            format={(v) => `${Math.round(v)}`}
          />
          <Sparkline
            label="Lateral G"
            unit="G"
            colorVar="--warning"
            signed
            accessor={(t) => t.lg ?? 0}
            format={(v) => `${v >= 0 ? "+" : "−"}${Math.abs(v).toFixed(2)}`}
          />
        </div>
      </div>

      {/* Column 2 — chassis centerpiece */}
      <CarDiagram tick={tick} fresh={fresh} />

      {/* Column 3 — engine + forces */}
      <aside className="flex flex-col gap-4">
        <DynoCurve tick={tick} />
        <BoostGauge tick={tick} fresh={fresh} />
        <GForcePanel latG={tick.lg ?? 0} longG={tick.lng ?? 0} />
        <MetaPanel tick={tick} />
      </aside>
    </div>
  );
}
```

- [ ] **Step 3: Trim the cylinders row from `MetaPanel`**

Cylinders now live in the `EngineBadge`. In `MetaPanel` (`client/src/routes/live.tsx`), remove the cylinders row from the `rows` array:

```tsx
    ["Car PI", tick.cpi !== 0 ? formatCount(tick.cpi) : "—"],
```

Delete the line immediately after it:

```tsx
    ["Cylinders", tick.ncy !== 0 ? formatCount(tick.ncy) : "—"],
```

- [ ] **Step 4: Type-check**

Run: `cd client && pnpm exec tsc --noEmit`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add client/src/routes/live.tsx
git commit -m "feat(client): reflow live HUD into 3 columns with car diagram + dyno + boost"
```

---

## Task 8: Log open empirical questions

**Files:**
- Modify: `docs/data-needed.md`

- [ ] **Step 1: Append the open questions**

Add this section to `docs/data-needed.md` (match the file's existing heading style — read the file first and mirror its format; the entries below are the content):

```markdown
## Live HUD visualisation thresholds (added 2026-06-01)

These power the `/live` HUD car diagram, dyno curve, and boost gauge. All are
placeholders in `client/src/components/hud/tire-scale.ts` / `engine.ts` and need a
real capture to confirm. Update or remove these entries when resolved.

- **Tire temp (`tt`) unit + grip-window thresholds.** `TEMP_THRESHOLDS` assumes a
  Celsius-ish scale (cold 50, optimal 70–95, hot 110, overheat 120). Confirm the
  unit and tune the band edges against a capture with known tire state.
- **Boost (`bo`) unit + range.** `BoostGauge` displays raw `bo` across a placeholder
  −1..2 range. Confirm whether the unit is bar / PSI / atm and fix the display range
  + zero/vacuum convention.
- **Slip-ratio (`tsr`) lockup/spin threshold.** `TSR_SLIP_THRESHOLD = 0.15` and
  `SLIP_ANGLE_MAX = 0.3` rad are guesses for the ring LOCK/SPIN tags and slip-angle
  arrow scale. Confirm against a capture with deliberate lockup/wheelspin.
```

- [ ] **Step 2: Commit**

```bash
git add docs/data-needed.md
git commit -m "docs: log HUD visualisation threshold unknowns to data-needed"
```

---

## Task 9: Final verification gate

**Files:** none (verification only).

- [ ] **Step 1: Full type-check**

Run: `cd client && pnpm exec tsc --noEmit`
Expected: no errors. If deps appear missing, run `cd client && pnpm install` first (a stale `tsc`-triggered install can prune devDeps — see CLAUDE.md gotchas).

- [ ] **Step 2: Full test suite**

Run: `cd client && pnpm test`
Expected: all tests pass, including the new `hud/tire-scale.test.ts` (5) and `hud/engine.test.ts` (6).

- [ ] **Step 3: Lint + format**

Run: `cd client && pnpm lint && pnpm format`
Expected: clean. Commit any formatting changes:

```bash
git add -A client/src
git commit -m "style(client): format HUD telemetry components"
```

- [ ] **Step 4: Visual verification (Chrome DevTools MCP against Vite dev)**

Start the client dev server (`cd client && pnpm dev`, serves `:3000`), open `/live`, then inject a representative frame so all corners exercise the visuals (vary per-wheel values so FL/FR/RL/RR differ — cold front, hot spinning RR, etc.):

```js
// via mcp__chrome-devtools__evaluate_script on http://localhost:3000/live
const base = useLiveStore.getState().latest ?? {};
const latest = {
  ...base,
  gv: 2, pv: 4, race: true, lap: 3, sp: 50, g: 5,
  rpm: 6800, rmx: 7800, pw: 412000, tq: 540, bo: 0.9,
  dt: 1, ncy: 8, co: 12345, cc: 6, cpi: 798,
  tp: 0.96, bp: 0, lg: 1.12, lng: -0.34,
  tt: [61, 88, 79, 96], tcs: [0.3, 0.9, 0.4, 1.3],
  tsr: [0.02, 0.05, -0.02, 0.4], tsa: [0.05, 0.18, -0.04, 0.1],
  stn: [0.4, 0.7, 0.45, 0.6], stm: [0, 0, 0, 0], wrs: [40, 40, 40, 95],
};
useLiveStore.setState({ latest, lastPushedAt: Date.now(), push: () => {} });
```

Confirm: car diagram shows differing corner colours, the RR ring is red with a "SPIN" tag, suspension bars differ, the engine badge reads "8-cyl · RWD", the dyno cursor sits near the right under the redline zone, and the boost marker sits right of zero. Screenshot to `.ai/` (gitignored). No console errors.

- [ ] **Step 5: Final confirmation**

Confirm the whole branch is committed (`git status` clean) and summarise what shipped.

---

## Self-Review notes

- **Spec coverage:** car diagram (Task 3) covers temp/slip/suspension/slip-angle/lockup-spin/drivetrain; dyno (Task 4) + badge (Task 5) cover power/torque/cylinders/drivetrain; boost gauge (Task 6); grip-window scale + thresholds (Task 1); layout reflow with `items-stretch` equal heights (Task 7); empirical unknowns logged (Task 8); Vitest + DevTools testing (Tasks 1–2, 9). All spec sections map to a task.
- **Type consistency:** `heatScaleColor`, `ringFromCombinedSlip`, `classifyWheelSlip`, `slipAngleTick`, `drivenAxle`, `drivetrainLabel`, `cylinderLabel`, `powerKW`, `boostFraction`, `DynoEnvelope`/`DynoBucket` names are identical across definition (Tasks 1–2) and use (Tasks 3–6). Per-wheel arrays indexed FL=0/FR=1/RL=2/RR=3 consistently.
- **No new blue token:** boost vacuum + suspension bars use `--muted`; heat scale uses muted→success→warning→danger only.
