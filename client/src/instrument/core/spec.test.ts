import { expect, test } from "vitest";
import { SPEC } from "./spec";

test("sweep leaves a 90° bottom notch", () => {
  expect(SPEC.sweep.extentDeg).toBe(270);
  expect((SPEC.sweep.startDeg + SPEC.sweep.extentDeg) % 360).toBe(45);
});

test("redline fraction is within the dial", () => {
  expect(SPEC.scales.redlineFraction).toBeGreaterThan(0.5);
  expect(SPEC.scales.redlineFraction).toBeLessThan(1);
});
