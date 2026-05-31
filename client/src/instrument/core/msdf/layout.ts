export interface RawGlyph {
  char: string;
  x: number;
  y: number;
  width: number;
  height: number;
  xoffset: number;
  yoffset: number;
  xadvance: number;
}

export interface RawAtlas {
  common: { scaleW: number; scaleH: number; lineHeight: number; base: number };
  chars: RawGlyph[];
}

export interface GlyphQuad {
  x: number;
  y: number;
  w: number;
  h: number;
  u0: number;
  v0: number;
  u1: number;
  v1: number;
}

export function buildGlyphMap(atlas: RawAtlas): Map<string, RawGlyph> {
  const m = new Map<string, RawGlyph>();
  for (const g of atlas.chars) m.set(g.char, g);
  return m;
}

/**
 * Lay out left-to-right at `scale` px per atlas unit; origin = pen start, y down from the line top.
 * Unknown characters advance by a default amount (40% of base) and emit no quad.
 */
export function layoutText(
  text: string,
  glyphs: Map<string, RawGlyph>,
  atlas: RawAtlas,
  scale: number,
): GlyphQuad[] {
  const quads: GlyphQuad[] = [];
  let penX = 0;
  for (const ch of text) {
    const g = glyphs.get(ch);
    if (!g) {
      penX += atlas.common.base * 0.4 * scale;
      continue;
    }
    quads.push({
      x: penX + g.xoffset * scale,
      y: g.yoffset * scale,
      w: g.width * scale,
      h: g.height * scale,
      u0: g.x / atlas.common.scaleW,
      v0: g.y / atlas.common.scaleH,
      u1: (g.x + g.width) / atlas.common.scaleW,
      v1: (g.y + g.height) / atlas.common.scaleH,
    });
    penX += g.xadvance * scale;
  }
  return quads;
}

export function measureText(text: string, glyphs: Map<string, RawGlyph>, scale: number): number {
  let w = 0;
  for (const ch of text) {
    const g = glyphs.get(ch);
    w += g ? g.xadvance * scale : 0;
  }
  return w;
}
