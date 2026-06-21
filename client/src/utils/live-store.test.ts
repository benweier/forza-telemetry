import { expect, test } from "vitest";
import { displayIndex, displayTick } from "./live-store";
import type { TickFrame } from "~/types/tick.generated";

// Synthetic ring: one tick per 100 ms, sts in epoch-ns. Index i is at i*100 ms.
const MS = 1e6;
const ring = Array.from({ length: 10 }, (_, i) => ({ sts: i * 100 * MS }) as unknown as TickFrame);

test("preview off returns the latest index", () => {
  expect(displayIndex({ ring, previewEnabled: false, offsetMs: 500 })).toBe(9);
});

test("zero offset returns the latest index even when preview is on", () => {
  expect(displayIndex({ ring, previewEnabled: true, offsetMs: 0 })).toBe(9);
});

test("offset walks back to the tick at-or-before (newest - offset)", () => {
  // newest = 900 ms; 250 ms back = 650 ms target → floor to the 600 ms tick (index 6).
  expect(displayIndex({ ring, previewEnabled: true, offsetMs: 250 })).toBe(6);
  // Exact hit: 300 ms back = 600 ms → index 6.
  expect(displayIndex({ ring, previewEnabled: true, offsetMs: 300 })).toBe(6);
});

test("offset beyond the buffered span clamps to the oldest tick", () => {
  expect(displayIndex({ ring, previewEnabled: true, offsetMs: 5000 })).toBe(0);
});

test("empty ring yields index -1 and a null tick", () => {
  const empty = { ring: [] as TickFrame[], previewEnabled: true, offsetMs: 100 };
  expect(displayIndex(empty)).toBe(-1);
  expect(displayTick(empty)).toBeNull();
});
