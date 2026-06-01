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
  // temp=60 is exactly halfway between cold(50) and optimalLo(70) → 50% weight on --success
  expect(heatScaleColor(60)).toBe("color-mix(in oklab, var(--muted) 50%, var(--success) 50%)");
});

test("classifyWheelSlip: negative slip ratio locks, positive spins, small is null", () => {
  expect(classifyWheelSlip(-0.5)).toBe("lock");
  expect(classifyWheelSlip(0.5)).toBe("spin");
  expect(classifyWheelSlip(0)).toBeNull();
  expect(classifyWheelSlip(0.01)).toBeNull();
});

test("classifyWheelSlip: honours a custom threshold", () => {
  expect(classifyWheelSlip(0.05, 0.04)).toBe("spin");
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
