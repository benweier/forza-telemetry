import { expect, test } from "vitest";
import type { TickFrame } from "~/types/tick.generated";
import { targetsFromTick, buildInstrumentState } from "./state";

const RAD = Math.PI / 180;
const tick = (p: Partial<TickFrame>): TickFrame => p as unknown as TickFrame;

test("speed converts m/s → km/h", () => {
  expect(targetsFromTick(tick({ sp: 50 }), 8000).speedKmh).toBeCloseTo(180, 1);
});

test("buildInstrumentState produces angles at the sweep start when values are zero", () => {
  const cs = buildInstrumentState({ speedKmh: 0, rpm: 0, throttle: 0, brake: 0, gx: 0, gy: 0, gear: "N", rmx: 8000 });
  expect(cs.speedAngle).toBeCloseTo(135 * RAD, 5);
  expect(cs.rpmAngle).toBeCloseTo(135 * RAD, 5);
});

test("targetsFromTick negates both g axes (felt force) and clamps to ±1", () => {
  const t = targetsFromTick(tick({ lg: 10, lng: -10 }), 8000);
  expect(t.gx).toBe(-1); // hard right → ball thrown left
  expect(t.gy).toBe(1); // braking (lng<0) → ball thrown forward
});

test("targetsFromTick: g axes are the negation of the raw acceleration", () => {
  const right = targetsFromTick(tick({ lg: 1.25 }), 8000); // half of G_LIMIT
  expect(right.gx).toBeCloseTo(-0.5, 5); // right turn → dot opposite
  const accel = targetsFromTick(tick({ lng: 1.25 }), 8000);
  expect(accel.gy).toBeCloseTo(-0.5, 5); // accelerating → dot thrown back
});
