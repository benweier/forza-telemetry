// Shared heat ramp for the track-path map. The deck.gl layer colors, the
// legend gradient, and the domain logic all read from this one module so they
// can't drift apart (the gradient used to be a second hand-written copy).

export const HEAT_STOPS: ReadonlyArray<readonly [number, readonly [number, number, number]]> = [
  [0.0, [40, 90, 200]],
  [0.25, [80, 200, 220]],
  [0.5, [120, 220, 100]],
  [0.75, [240, 200, 60]],
  [1.0, [240, 80, 60]],
];

export const heatGradientCSS = `linear-gradient(to right, ${HEAT_STOPS.map(
  ([, c]) => `rgb(${c[0]},${c[1]},${c[2]})`,
).join(", ")})`;

/**
 * Full-scale for a channel. Channels with a true absolute scale (brake is
 * genuinely 0..100%) pass their fixed max; relative channels pass null and
 * scale to the stint's own observed peak — so red always means "this stint's
 * fastest/hardest sections". A fixed 90 m/s speed scale used to mean red
 * required 324 km/h, leaving normal drives entirely blue-green.
 */
export function resolveDomain(absoluteMax: number | null, peak: number): number {
  if (absoluteMax !== null) return absoluteMax;
  return peak > 0 ? peak : 1; // all-zero channel → uniform cold end, never NaN
}

/** |raw| positioned on the cool→hot ramp against `domain`, clipped to [0,1]. */
export function valueToColor(raw: number, domain: number): [number, number, number, number] {
  const t = Math.min(Math.max(Math.abs(raw) / domain, 0), 1);
  for (let i = 1; i < HEAT_STOPS.length; i++) {
    const [tNext, cNext] = HEAT_STOPS[i];
    const [tPrev, cPrev] = HEAT_STOPS[i - 1];
    if (t <= tNext) {
      const local = (t - tPrev) / (tNext - tPrev);
      return [
        Math.round(cPrev[0] + (cNext[0] - cPrev[0]) * local),
        Math.round(cPrev[1] + (cNext[1] - cPrev[1]) * local),
        Math.round(cPrev[2] + (cNext[2] - cPrev[2]) * local),
        255,
      ];
    }
  }
  const last = HEAT_STOPS[HEAT_STOPS.length - 1][1];
  return [last[0], last[1], last[2], 255];
}
