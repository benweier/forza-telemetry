import { expect, test } from "vitest";
import type { TickFrame } from "~/types/tick.generated";
import { targetsFromTick, buildClusterState, gearLabel } from "./state";

const RAD = Math.PI / 180;
const tick = (p: Partial<TickFrame>): TickFrame => p as unknown as TickFrame;

test("speed converts m/s → km/h", () => {
  expect(targetsFromTick(tick({ sp: 50 }), 8000).speedKmh).toBeCloseTo(180, 1);
});

test("gearLabel maps neutral and reverse", () => {
  expect(gearLabel(0)).toBe("N");
  expect(gearLabel(-1)).toBe("R");
  expect(gearLabel(3)).toBe("3");
});

test("buildClusterState produces angles at the sweep start when values are zero", () => {
  const cs = buildClusterState({ speedKmh: 0, rpm: 0, throttle: 0, brake: 0, gx: 0, gy: 0, gear: "N", rmx: 8000 });
  expect(cs.speedAngle).toBeCloseTo(135 * RAD, 5);
  expect(cs.rpmAngle).toBeCloseTo(135 * RAD, 5);
});

test("targetsFromTick clamps g to ±1", () => {
  const t = targetsFromTick(tick({ lg: 10, lng: -10 }), 8000);
  expect(t.gx).toBe(1);
  expect(t.gy).toBe(-1);
});
