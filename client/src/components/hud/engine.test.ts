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
  // zero-range guard: min === max must return 0, not NaN
  expect(boostFraction(0.5, 2, 2)).toBe(0);
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

test("DynoEnvelope ignores negative rpm", () => {
  const env = new DynoEnvelope(1000);
  env.update(-10, 100_000, 300, 1);
  expect(env.buckets()).toHaveLength(0);
});
