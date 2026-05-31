export interface Smoother {
  value: number;
  velocity: number;
}

export function makeSmoother(value: number): Smoother {
  return { value, velocity: 0 };
}

/**
 * Critically-damped spring step (semi-implicit Euler). `stiffness` higher =
 * snappier. Large dt is sub-stepped so it never overshoots or explodes.
 */
export function stepSmoother(s: Smoother, target: number, dt: number, stiffness: number): Smoother {
  const damping = 2 * Math.sqrt(stiffness);
  const maxStep = 1 / 120;
  let { value, velocity } = s;
  let remaining = dt;
  while (remaining > 0) {
    const h = Math.min(maxStep, remaining);
    const accel = stiffness * (target - value) - damping * velocity;
    velocity += accel * h;
    value += velocity * h;
    remaining -= h;
  }
  return { value, velocity };
}
