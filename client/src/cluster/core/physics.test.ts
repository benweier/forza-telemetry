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
  expect(maxV).toBeLessThanOrEqual(100.5);
});

test("stable for large dt (no NaN/explosion)", () => {
  let s = makeSmoother(0);
  for (let i = 0; i < 50; i++) s = stepSmoother(s, 100, 0.5, 120);
  expect(Number.isFinite(s.value)).toBe(true);
  expect(s.value).toBeGreaterThan(0);
  expect(s.value).toBeLessThanOrEqual(101);
});
