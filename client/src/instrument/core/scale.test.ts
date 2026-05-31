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
