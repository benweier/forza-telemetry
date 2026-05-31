/**
 * Cluster geometry in a normalized 0..1 layout space (x right, y down),
 * mapped to the canvas by the renderer. Sweep angles in radians, measured
 * clockwise from the +x axis. The 270° sweep leaves a 90° notch at the bottom.
 */
export const SPEC = {
  gauge: { cx: 0.32, cy: 0.5, ringOuter: 0.46, ringInner: 0.42, dialOuter: 0.40 },
  sweep: { startDeg: 135, extentDeg: 270 },
  speedTicks: { minorEveryDeg: 13.5, majorEveryDeg: 54, minorR: [0.34, 0.38], majorR: [0.32, 0.38] },
  rail: {
    x: 0.78,
    gear: { cy: 0.22, size: 0.16 },
    bars: { cy: 0.5, w: 0.03, h: 0.26, gap: 0.04 },
    gforce: { cy: 0.8, r: 0.11 },
  },
  scales: { speedMaxKmh: 400, redlineFraction: 0.88 },
} as const;
