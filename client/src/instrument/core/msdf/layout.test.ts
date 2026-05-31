import { expect, test } from "vitest";
import { buildGlyphMap, layoutText, measureText, type RawAtlas } from "./layout";

const atlas: RawAtlas = {
  common: { scaleW: 100, scaleH: 100, lineHeight: 50, base: 40 },
  chars: [
    { char: "1", x: 0, y: 0, width: 10, height: 20, xoffset: 1, yoffset: 2, xadvance: 12 },
    { char: "2", x: 10, y: 0, width: 10, height: 20, xoffset: 1, yoffset: 2, xadvance: 14 },
  ],
};

test("buildGlyphMap indexes by char", () => {
  const m = buildGlyphMap(atlas);
  expect(m.get("1")?.xadvance).toBe(12);
  expect(m.has("9")).toBe(false);
});

test("layoutText advances per glyph and emits a quad each with normalized uv", () => {
  const m = buildGlyphMap(atlas);
  const q = layoutText("12", m, atlas, 1);
  expect(q).toHaveLength(2);
  expect(q[0].x).toBeCloseTo(1);          // xoffset*scale
  expect(q[1].x).toBeCloseTo(12 + 1);     // first xadvance + second xoffset
  expect(q[0].u0).toBeCloseTo(0);
  expect(q[0].u1).toBeCloseTo(0.1);       // (x+width)/scaleW = 10/100
  expect(q[0].v1).toBeCloseTo(0.2);       // (y+height)/scaleH = 20/100
});

test("measureText sums advances", () => {
  const m = buildGlyphMap(atlas);
  expect(measureText("12", m, 1)).toBeCloseTo(26); // 12 + 14
  expect(measureText("12", m, 2)).toBeCloseTo(52);
});

test("unknown glyph advances by a default and emits no quad", () => {
  const m = buildGlyphMap(atlas);
  const q = layoutText("1 2", m, atlas, 1); // space unknown
  expect(q).toHaveLength(2);
});
