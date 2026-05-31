/** Per-frame, renderer-agnostic description of the cluster. */
export interface ClusterState {
  speedKmh: number;       // for the digital readout
  rpm: number;            // raw, for the rpm caption
  speedAngle: number;     // radians within the dial sweep (0 at sweep start)
  rpmAngle: number;       // radians within the ring sweep
  redline: number;        // 0..1, depth into redline for glow
  gear: string;           // "R" | "N" | "1".."n"
  throttle: number;       // 0..1
  brake: number;          // 0..1
  gx: number;             // g-ball displacement -1..1 (felt force = −lateral accel)
  gy: number;             // g-ball displacement -1..1 (felt force = −longitudinal accel)
}

export interface RendererOpts {
  /** Optional colour overrides sampled from CSS custom properties at init. */
  colors?: Partial<import("./core/palette").Palette>;
}

export interface ClusterRenderer {
  init(canvas: HTMLCanvasElement, opts: RendererOpts): Promise<void>;
  render(state: ClusterState): void;
  resize(width: number, height: number, dpr: number): void;
  destroy(): void;
}
