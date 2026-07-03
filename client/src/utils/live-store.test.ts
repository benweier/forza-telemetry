import { afterEach, expect, test } from "vitest";
import { displayIndex, displayTick, getRing, readDisplayTick, useLiveStore } from "./live-store";
import { tickFixture } from "~/test/tick-fixture";
import type { TickFrame } from "~/types/tick.generated";

// Synthetic ring: one tick per 100 ms, sts in epoch-ns. Index i is at i*100 ms.
const MS = 1e6;
const ring = Array.from({ length: 10 }, (_, i) => tickFixture({ sts: i * 100 * MS }));

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

// ---------- store-level ring behavior (the ring lives OUTSIDE reactive state) ----------

afterEach(() => {
  useLiveStore.getState().clear();
  useLiveStore.getState().setPreviewEnabled(false);
  useLiveStore.getState().setOffsetMs(0);
});

test("push appends to the module ring and bumps latest", () => {
  const s = useLiveStore.getState();
  s.push(tickFixture({ sts: 1 * MS }));
  s.push(tickFixture({ sts: 2 * MS }));
  expect(getRing()).toHaveLength(2);
  expect(useLiveStore.getState().latest?.sts).toBe(2 * MS);
  expect(useLiveStore.getState().lastPushedAt).not.toBeNull();
});

test("ring caps at 3600 ticks, dropping the oldest", () => {
  const s = useLiveStore.getState();
  for (let i = 0; i < 3605; i++) s.push(tickFixture({ sts: i * MS }));
  expect(getRing()).toHaveLength(3600);
  expect(getRing()[0].sts).toBe(5 * MS); // 0..4 evicted
});

test("clear empties the ring and resets latest", () => {
  const s = useLiveStore.getState();
  s.push(tickFixture({ sts: 1 * MS }));
  s.clear();
  expect(getRing()).toHaveLength(0);
  expect(useLiveStore.getState().latest).toBeNull();
  expect(readDisplayTick()).toBeNull();
});

test("readDisplayTick reads the live ring: latest when preview off, delayed when on", () => {
  const s = useLiveStore.getState();
  for (let i = 0; i < 10; i++) s.push(tickFixture({ sts: i * 100 * MS }));
  expect(readDisplayTick()?.sts).toBe(900 * MS);
  s.setPreviewEnabled(true);
  s.setOffsetMs(250); // 900ms − 250ms → floor to the 600ms tick
  expect(readDisplayTick()?.sts).toBe(600 * MS);
});
