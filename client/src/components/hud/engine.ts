/**
 * Engine + dyno helpers for the live HUD. Power/torque are SI on the wire
 * (watts / Nm). The drivetrain enum follows the Forza convention 0=FWD, 1=RWD,
 * 2=AWD. Boost units are unconfirmed (see docs/data-needed.md) — the gauge shows
 * the raw value across a placeholder display range.
 */

export function drivetrainLabel(dt: number): string | null {
  switch (dt) {
    case 0:
      return "FWD";
    case 1:
      return "RWD";
    case 2:
      return "AWD";
    default:
      return null;
  }
}

export type DrivenAxle = "front" | "rear" | "both" | null;

export function drivenAxle(dt: number): DrivenAxle {
  switch (dt) {
    case 0:
      return "front";
    case 1:
      return "rear";
    case 2:
      return "both";
    default:
      return null;
  }
}

/** Forza telemetry exposes cylinder COUNT but not layout (V/I/flat). */
export function cylinderLabel(ncy: number): string | null {
  return ncy > 0 ? `${ncy}-cyl` : null;
}

export function powerKW(watts: number): number {
  return watts / 1000;
}

function clamp01(v: number): number {
  return v < 0 ? 0 : v > 1 ? 1 : v;
}

export function boostFraction(boost: number, min: number, max: number): number {
  if (max === min) return 0;
  return clamp01((boost - min) / (max - min));
}

export interface DynoBucket {
  rpm: number;
  powerKW: number;
  torqueNm: number;
}

/**
 * Accumulates the maximum power/torque seen per RPM bucket — the live "dyno
 * envelope" that fills in as the player revs. Resets automatically when the car
 * ordinal changes (a new car has a new curve). Buckets are fixed-width and
 * capped well above any real redline so a higher-revving car never clips.
 *
 * `carOrdinal` starts at 0. Ordinal 0 doubles as the "no car / unknown"
 * sentinel in Forza telemetry, so `update(..., 0)` is treated as "same car"
 * and never triggers a reset — by design.
 */
export class DynoEnvelope {
  private readonly maxPower: Float64Array;
  private readonly maxTorque: Float64Array;
  private readonly seen: boolean[];
  /** Ordinal 0 = "no car / unknown" sentinel; never triggers a reset. */
  private carOrdinal = 0;

  constructor(
    public readonly bucketRpm = 250,
    capRpm = 20_000,
  ) {
    const n = Math.ceil(capRpm / bucketRpm) + 1;
    this.maxPower = new Float64Array(n);
    this.maxTorque = new Float64Array(n);
    this.seen = new Array(n).fill(false);
  }

  reset(): void {
    this.maxPower.fill(0);
    this.maxTorque.fill(0);
    this.seen.fill(false);
  }

  update(rpm: number, powerW: number, torqueNm: number, carOrdinal = 0): void {
    if (carOrdinal !== this.carOrdinal) {
      this.carOrdinal = carOrdinal;
      this.reset();
    }
    if (rpm < 0) return;
    const i = Math.min(this.maxPower.length - 1, Math.floor(rpm / this.bucketRpm));
    const kw = powerW / 1000;
    if (!this.seen[i] || kw > this.maxPower[i]) this.maxPower[i] = kw;
    if (!this.seen[i] || torqueNm > this.maxTorque[i]) this.maxTorque[i] = torqueNm;
    this.seen[i] = true;
  }

  buckets(): DynoBucket[] {
    const out: DynoBucket[] = [];
    for (let i = 0; i < this.maxPower.length; i++) {
      if (this.seen[i]) {
        out.push({
          rpm: i * this.bucketRpm,
          powerKW: this.maxPower[i],
          torqueNm: this.maxTorque[i],
        });
      }
    }
    return out;
  }
}
