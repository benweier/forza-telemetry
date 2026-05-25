/* Hallmark · component: track-path-map · genre: dashboard · theme: Glass
 * 3D track-path minimap. Reads 1Hz preview_samples (pos_x, pos_y, pos_z, speed_ms)
 * and renders a speed-coloured polyline under an OrbitView. Per-segment colour
 * via LineLayer (one segment per consecutive sample pair) — gives an honest
 * gradient at 1Hz without needing TripsLayer / custom shaders.
 */
import { COORDINATE_SYSTEM, OrbitView } from "@deck.gl/core";
import { LineLayer } from "@deck.gl/layers";
import DeckGL from "@deck.gl/react";
import { useMemo } from "react";
import type { PreviewSample } from "~/utils/schemas";

interface TrackPathMapProps {
  samples: PreviewSample[];
}

interface Segment {
  source: [number, number, number];
  target: [number, number, number];
  speed: number;
}

/**
 * Forza axes → deck.gl OrbitView mapping:
 *  - Forza pos_x (lateral)      → deck x
 *  - Forza pos_y (vertical)     → deck y  (OrbitView treats +Y as up)
 *  - Forza pos_z (longitudinal) → deck z
 * Camera target is the path centroid so the track sits at the origin regardless
 * of where the world coords land (Forza world coords drift into the thousands).
 */
export function TrackPathMap({ samples }: TrackPathMapProps) {
  const { segments, target, extent } = useMemo(() => buildSegments(samples), [samples]);

  if (segments.length < 1) {
    return (
      <div
        role="img"
        aria-label="Track path unavailable"
        className="flex aspect-[16/9] w-full items-center justify-center rounded-2xl bg-surface text-xs text-muted shadow-surface"
      >
        Position data not available for this stint.
      </div>
    );
  }

  // OrbitView zoom is log2 scale: zoom = -log2(extent / viewport size). For a
  // ~600px-wide canvas, picking zoom so the bounding cube spans ~80% of the view.
  const zoom = Math.log2(480 / Math.max(extent, 1));

  const layers = [
    new LineLayer<Segment>({
      id: "track-path",
      data: segments,
      coordinateSystem: COORDINATE_SYSTEM.CARTESIAN,
      getSourcePosition: (s) => s.source,
      getTargetPosition: (s) => s.target,
      getColor: (s) => speedToColor(s.speed),
      getWidth: 3,
      widthUnits: "pixels",
      widthMinPixels: 2,
    }),
  ];

  const initialViewState = {
    target,
    zoom,
    rotationX: 35,
    rotationOrbit: -30,
    minZoom: -10,
    maxZoom: 10,
  };

  return (
    <div className="relative aspect-[16/9] w-full overflow-hidden rounded-2xl bg-surface shadow-surface">
      <DeckGL
        views={new OrbitView({ orbitAxis: "Y", fovy: 50 })}
        initialViewState={initialViewState}
        controller={{ inertia: 250 }}
        layers={layers}
      />
      <Legend />
      <Hint />
    </div>
  );
}

// ---------- helpers ----------

function buildSegments(samples: PreviewSample[]): {
  segments: Segment[];
  target: [number, number, number];
  extent: number;
} {
  const valid = samples.filter(
    (s): s is PreviewSample & { pos_x: number; pos_y: number; pos_z: number } =>
      s.pos_x !== null && s.pos_y !== null && s.pos_z !== null,
  );
  if (valid.length < 2) {
    return { segments: [], target: [0, 0, 0], extent: 1 };
  }

  let minX = Infinity,
    maxX = -Infinity,
    minY = Infinity,
    maxY = -Infinity,
    minZ = Infinity,
    maxZ = -Infinity;
  for (const s of valid) {
    if (s.pos_x < minX) minX = s.pos_x;
    if (s.pos_x > maxX) maxX = s.pos_x;
    if (s.pos_y < minY) minY = s.pos_y;
    if (s.pos_y > maxY) maxY = s.pos_y;
    if (s.pos_z < minZ) minZ = s.pos_z;
    if (s.pos_z > maxZ) maxZ = s.pos_z;
  }
  const cx = (minX + maxX) / 2;
  const cy = (minY + maxY) / 2;
  const cz = (minZ + maxZ) / 2;
  const extent = Math.max(maxX - minX, maxY - minY, maxZ - minZ);

  // Re-centre by subtracting centroid. Keeps OrbitView numerics happy and the
  // initial camera target at the origin.
  const segments: Segment[] = [];
  for (let i = 1; i < valid.length; i++) {
    const a = valid[i - 1];
    const b = valid[i];
    segments.push({
      source: [a.pos_x - cx, a.pos_y - cy, a.pos_z - cz],
      target: [b.pos_x - cx, b.pos_y - cy, b.pos_z - cz],
      speed: b.speed_ms ?? a.speed_ms ?? 0,
    });
  }
  return { segments, target: [0, 0, 0], extent };
}

/**
 * Speed → colour ramp. m/s into a cool-to-hot mapping:
 *   0 m/s   → blue (cool)
 *   90 m/s+ → red/orange (hot, ≈ 324 km/h ceiling for Forza top speeds)
 * Returned as [r,g,b,a] for deck.gl.
 */
function speedToColor(speedMS: number): [number, number, number, number] {
  const t = Math.min(Math.max(speedMS / 90, 0), 1);
  // 5-stop gradient: deep blue → cyan → green → yellow → red
  const stops: Array<[number, [number, number, number]]> = [
    [0.0, [40, 90, 200]],
    [0.25, [80, 200, 220]],
    [0.5, [120, 220, 100]],
    [0.75, [240, 200, 60]],
    [1.0, [240, 80, 60]],
  ];
  for (let i = 1; i < stops.length; i++) {
    const [tNext, cNext] = stops[i];
    const [tPrev, cPrev] = stops[i - 1];
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
  return [240, 80, 60, 255];
}

function Legend() {
  return (
    <div className="pointer-events-none absolute right-3 bottom-3 flex items-center gap-2 rounded-xl bg-surface-secondary/80 px-3 py-2 text-xs text-muted backdrop-blur">
      <span>slow</span>
      <span
        aria-hidden
        className="h-2 w-24 rounded-full"
        style={{
          background:
            "linear-gradient(to right, rgb(40,90,200), rgb(80,200,220), rgb(120,220,100), rgb(240,200,60), rgb(240,80,60))",
        }}
      />
      <span>fast</span>
    </div>
  );
}

function Hint() {
  return (
    <div className="pointer-events-none absolute top-3 left-3 rounded-xl bg-surface-secondary/80 px-3 py-1.5 text-xs text-muted backdrop-blur">
      drag to orbit · scroll to zoom
    </div>
  );
}
