import type { TickFrame } from "~/types/tick.generated";
import type { ClusterState } from "../renderer";
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

export function targetsFromTick(t: TickFrame, fallbackRmx = 8000): Targets {
  const rmx = t.rmx && t.rmx > 0 ? t.rmx : fallbackRmx;
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
export function buildClusterState(a: Targets): ClusterState {
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
