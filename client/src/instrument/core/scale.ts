const RAD = Math.PI / 180;

export function valueToFraction(value: number, min: number, max: number): number {
  if (max === min) return 0;
  const f = (value - min) / (max - min);
  return f < 0 ? 0 : f > 1 ? 1 : f;
}

export function fractionToAngle(fraction: number, startDeg: number, extentDeg: number): number {
  return (startDeg + fraction * extentDeg) * RAD;
}

/** 0 below `threshold` fraction, linearly ramps to 1 at fraction 1. */
export function redlineFactor(fraction: number, threshold: number): number {
  if (fraction <= threshold) return 0;
  return Math.min(1, (fraction - threshold) / (1 - threshold));
}
