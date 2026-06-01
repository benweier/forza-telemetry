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
  if (temp < t.optimalLo)
    return mix("--muted", "--success", (temp - t.cold) / (t.optimalLo - t.cold));
  if (temp <= t.optimalHi) return "var(--success)";
  if (temp < t.hot)
    return mix("--success", "--warning", (temp - t.optimalHi) / (t.hot - t.optimalHi));
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
  // strokeWidth ramps over the full grip→max range — a continuous width signal.
  // Colour transitions faster (muted→warning over grip→limit, then warning→danger
  // over limit→max) because colour is the primary urgency cue; width reinforces it.
  const f = clamp01((tcs - SLIP_RING.grip) / (SLIP_RING.max - SLIP_RING.grip));
  const strokeWidth = SLIP_RING.minWidth + f * (SLIP_RING.maxWidth - SLIP_RING.minWidth);
  let color: string;
  if (tcs < SLIP_RING.grip) {
    color = "var(--muted)";
  } else if (tcs < SLIP_RING.limit) {
    color = mix(
      "--muted",
      "--warning",
      (tcs - SLIP_RING.grip) / (SLIP_RING.limit - SLIP_RING.grip),
    );
  } else {
    color = mix(
      "--warning",
      "--danger",
      clamp01((tcs - SLIP_RING.limit) / (SLIP_RING.max - SLIP_RING.limit)),
    );
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
