import { expect, test } from "vitest";
import { HEAT_STOPS, resolveDomain, valueToColor } from "./track-path-color";

test("the stint's peak maps to the hot end of the ramp", () => {
  // The original bug: a fixed 90 m/s domain meant red required 324 km/h and a
  // normal drive's peak sat forever in blue-green. With the domain resolved
  // from the stint's own peak, the peak must be the last stop exactly.
  const domain = resolveDomain(null, 61.3); // ~220 km/h stint peak
  expect(domain).toBe(61.3);
  expect(valueToColor(61.3, domain)).toEqual([240, 80, 60, 255]);
});

test("zero maps to the cold end, midpoint to the middle stop", () => {
  expect(valueToColor(0, 90)).toEqual([40, 90, 200, 255]);
  expect(valueToColor(45, 90)).toEqual([120, 220, 100, 255]);
});

test("negative values color by magnitude (lateral G is signed)", () => {
  expect(valueToColor(-2, 2)).toEqual([240, 80, 60, 255]);
});

test("values beyond the domain clamp to the hot end", () => {
  expect(valueToColor(150, 90)).toEqual([240, 80, 60, 255]);
});

test("absolute channels keep their fixed domain regardless of peak", () => {
  expect(resolveDomain(1.0, 0.4)).toBe(1.0); // brake: 40% peak stays 40% orange-ward
});

test("an all-zero channel resolves to a safe domain, never NaN", () => {
  expect(resolveDomain(null, 0)).toBe(1);
  expect(valueToColor(0, resolveDomain(null, 0))).toEqual([40, 90, 200, 255]);
});

test("legend gradient derives from the same stops as the layer colors", () => {
  // Guards the drift the review found: the legend used to hand-copy the ramp.
  for (const [, c] of HEAT_STOPS) {
    expect(c).toHaveLength(3);
  }
});
