/* Hallmark · component: track-path-map · genre: dashboard · theme: Glass
 * 3D track-path minimap. Reads /stints/{id}/path (column-oriented, ~10Hz by
 * default) and renders a speed-coloured polyline under an OrbitView. Per-
 * segment colour via LineLayer (one segment per consecutive sample pair).
 */
import { COORDINATE_SYSTEM, OrbitView } from "@deck.gl/core";
import { LineLayer } from "@deck.gl/layers";
import DeckGL from "@deck.gl/react";
import { useMemo } from "react";
import type { PathResponse } from "~/utils/schemas";

interface TrackPathMapProps {
  path: PathResponse;
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
export function TrackPathMap({ path }: TrackPathMapProps) {
  const { segments, extent } = useMemo(() => buildSegments(path), [path]);

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
    target: [0, 0, 0] as [number, number, number],
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
      <Legend hz={path.sample_hz} />
      <Hint />
    </div>
  );
}

// ---------- helpers ----------

// Column indices match the server's fixed pathColumns order.
const COL_POS_X = 1;
const COL_POS_Y = 2;
const COL_POS_Z = 3;
const COL_SPEED = 4;

function buildSegments(path: PathResponse): { segments: Segment[]; extent: number } {
  // Materialise valid (pos_x, pos_y, pos_z, speed) tuples in one pass.
  const pts: { x: number; y: number; z: number; speed: number }[] = [];
  for (const row of path.rows) {
    const x = row[COL_POS_X];
    const y = row[COL_POS_Y];
    const z = row[COL_POS_Z];
    if (x === null || y === null || z === null) continue;
    pts.push({ x, y, z, speed: row[COL_SPEED] ?? 0 });
  }
  if (pts.length < 2) {
    return { segments: [], extent: 1 };
  }

  let minX = Infinity,
    maxX = -Infinity,
    minY = Infinity,
    maxY = -Infinity,
    minZ = Infinity,
    maxZ = -Infinity;
  for (const p of pts) {
    if (p.x < minX) minX = p.x;
    if (p.x > maxX) maxX = p.x;
    if (p.y < minY) minY = p.y;
    if (p.y > maxY) maxY = p.y;
    if (p.z < minZ) minZ = p.z;
    if (p.z > maxZ) maxZ = p.z;
  }
  const cx = (minX + maxX) / 2;
  const cy = (minY + maxY) / 2;
  const cz = (minZ + maxZ) / 2;
  const extent = Math.max(maxX - minX, maxY - minY, maxZ - minZ);

  const segments: Segment[] = new Array(pts.length - 1);
  for (let i = 1; i < pts.length; i++) {
    const a = pts[i - 1];
    const b = pts[i];
    segments[i - 1] = {
      source: [a.x - cx, a.y - cy, a.z - cz],
      target: [b.x - cx, b.y - cy, b.z - cz],
      speed: b.speed,
    };
  }
  return { segments, extent };
}

/**
 * Speed → colour ramp. m/s into a cool-to-hot mapping:
 *   0 m/s   → blue (cool)
 *   90 m/s+ → red/orange (hot, ≈ 324 km/h ceiling for Forza top speeds)
 * Returned as [r,g,b,a] for deck.gl.
 */
function speedToColor(speedMS: number): [number, number, number, number] {
  const t = Math.min(Math.max(speedMS / 90, 0), 1);
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

function Legend({ hz }: { hz: number }) {
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
      <span className="ml-1 tabular-nums opacity-60">{hz.toFixed(0)}Hz</span>
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
