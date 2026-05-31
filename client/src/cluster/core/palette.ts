/** All colours as [r,g,b] in 0..1 linear-ish sRGB. No CSS tokens reach the GPU. */
export interface Palette {
  ringLow: [number, number, number];
  ringMid: [number, number, number];
  ringRed: [number, number, number];
  needle: [number, number, number];
  gearGlow: [number, number, number];
  gDot: [number, number, number];
  throttle: [number, number, number];
  brake: [number, number, number];
  panel: [number, number, number];
  tick: [number, number, number];
}

export const DEFAULT_PALETTE: Palette = {
  ringLow: [0.18, 0.83, 0.75],
  ringMid: [1.0, 0.81, 0.23],
  ringRed: [1.0, 0.35, 0.30],
  needle: [1.0, 0.35, 0.30],
  gearGlow: [0.35, 0.82, 1.0],
  gDot: [0.61, 0.55, 1.0],
  throttle: [0.21, 0.82, 0.48],
  brake: [1.0, 0.35, 0.30],
  panel: [0.07, 0.08, 0.10],
  tick: [0.78, 0.82, 0.87],
};
