/* Hallmark · component: track-path-map · genre: dashboard · theme: Glass
 * 3D track-path minimap. Renders the speed-coloured polyline plus picked
 * hot-spot markers (lateral G peak / braking peak / top speed) under an
 * OrbitView. Hover tooltips on either layer surface the underlying data
 * (speed + lap on segments, label + magnitude on hot-spots).
 */
import { COORDINATE_SYSTEM, OrbitView, type PickingInfo } from "@deck.gl/core";
import { LineLayer, ScatterplotLayer } from "@deck.gl/layers";
import DeckGL from "@deck.gl/react";
import { useMemo } from "react";
import type { HotSpot, PathResponse } from "~/utils/schemas";

interface TrackPathMapProps {
  path: PathResponse;
  hotSpots?: HotSpot[];
}

interface Segment {
  source: [number, number, number];
  target: [number, number, number];
  speed: number;
  lap: number | null;
  tickNS: number;
}

interface HotSpotMarker {
  id: string;
  pos: [number, number, number];
  type: string;
  label: string;
  value: number;
}

export function TrackPathMap({ path, hotSpots }: TrackPathMapProps) {
  const { segments, markers, extent } = useMemo(
    () => buildScene(path, hotSpots ?? []),
    [path, hotSpots],
  );

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
      pickable: true,
    }),
    new ScatterplotLayer<HotSpotMarker>({
      id: "hot-spots",
      data: markers,
      coordinateSystem: COORDINATE_SYSTEM.CARTESIAN,
      getPosition: (m) => m.pos,
      getFillColor: (m) => hotSpotColor(m.type),
      getLineColor: [255, 255, 255, 220],
      getRadius: 6,
      radiusUnits: "pixels",
      radiusMinPixels: 4,
      radiusMaxPixels: 10,
      stroked: true,
      lineWidthUnits: "pixels",
      getLineWidth: 1.5,
      pickable: true,
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
        getTooltip={tooltipFor}
      />
      <Legend hz={path.sample_hz} markerCount={markers.length} />
      <Hint />
    </div>
  );
}

// ---------- scene assembly ----------

// Column indices match the server's fixed pathColumns order.
const COL_TICK_NS = 0;
const COL_POS_X = 1;
const COL_POS_Y = 2;
const COL_POS_Z = 3;
const COL_SPEED = 4;
const COL_LAP = 5;

interface PathPoint {
  tickNS: number;
  x: number;
  y: number;
  z: number;
  speed: number;
  lap: number | null;
}

function buildScene(
  path: PathResponse,
  hotSpots: HotSpot[],
): { segments: Segment[]; markers: HotSpotMarker[]; extent: number } {
  const pts: PathPoint[] = [];
  for (const row of path.rows) {
    const tick = row[COL_TICK_NS];
    const x = row[COL_POS_X];
    const y = row[COL_POS_Y];
    const z = row[COL_POS_Z];
    if (tick === null || x === null || y === null || z === null) continue;
    pts.push({
      tickNS: tick,
      x,
      y,
      z,
      speed: row[COL_SPEED] ?? 0,
      lap: row[COL_LAP],
    });
  }
  if (pts.length < 2) {
    return { segments: [], markers: [], extent: 1 };
  }

  // One pass for bounds.
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

  // Segments between consecutive samples — colour the segment by the speed
  // at its end (matches "what speed did we reach by this point").
  const segments: Segment[] = new Array(pts.length - 1);
  for (let i = 1; i < pts.length; i++) {
    const a = pts[i - 1];
    const b = pts[i];
    segments[i - 1] = {
      source: [a.x - cx, a.y - cy, a.z - cz],
      target: [b.x - cx, b.y - cy, b.z - cz],
      speed: b.speed,
      lap: b.lap,
      tickNS: b.tickNS,
    };
  }

  // Hot-spot 3D positions = the path point nearest to peak_tick_ns. Path
  // rows are already ORDER BY server_recv_ns, so binary search is safe.
  const markers: HotSpotMarker[] = [];
  for (const h of hotSpots) {
    const p = nearestPoint(pts, h.peak_tick_ns);
    if (!p) continue;
    markers.push({
      id: h.id,
      pos: [p.x - cx, p.y - cy, p.z - cz],
      type: h.type,
      label: h.label,
      value: h.peak_value,
    });
  }

  return { segments, markers, extent };
}

function nearestPoint(pts: PathPoint[], tickNS: number): PathPoint | null {
  if (pts.length === 0) return null;
  let lo = 0;
  let hi = pts.length - 1;
  while (lo < hi) {
    const mid = (lo + hi) >>> 1;
    if (pts[mid].tickNS < tickNS) lo = mid + 1;
    else hi = mid;
  }
  // lo now indexes the first >= tickNS; compare with previous to pick the
  // closer of the two candidates.
  const cand = pts[lo];
  if (lo === 0) return cand;
  const prev = pts[lo - 1];
  return Math.abs(cand.tickNS - tickNS) < Math.abs(prev.tickNS - tickNS) ? cand : prev;
}

// ---------- visuals ----------

/**
 * Speed → colour ramp. m/s into a cool-to-hot mapping:
 *   0 m/s   → blue (cool)
 *   90 m/s+ → red/orange (hot, ≈ 324 km/h ceiling for Forza top speeds)
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

function hotSpotColor(type: string): [number, number, number, number] {
  switch (type) {
    case "peak_lateral_g": {
      return [255, 200, 60, 230]; // amber — cornering
    }
    case "peak_brake": {
      return [240, 80, 80, 230]; // red — braking
    }
    case "top_speed": {
      return [80, 180, 255, 230]; // blue — speed
    }
    default: {
      return [200, 200, 200, 230];
    }
  }
}

function hotSpotTypeLabel(t: string): string {
  switch (t) {
    case "peak_lateral_g": {
      return "Lateral G peak";
    }
    case "peak_brake": {
      return "Brake peak";
    }
    case "top_speed": {
      return "Top speed";
    }
    default: {
      return t;
    }
  }
}

// ---------- tooltip ----------

function tooltipFor(info: PickingInfo): string | null {
  if (!info.object) return null;
  if (info.layer?.id === "track-path") {
    const seg = info.object as Segment;
    const kmh = (seg.speed * 3.6).toFixed(0);
    const lap = seg.lap !== null ? `Lap ${seg.lap}` : "—";
    return `${kmh} km/h · ${lap}`;
  }
  if (info.layer?.id === "hot-spots") {
    const m = info.object as HotSpotMarker;
    return `${m.label}\n${hotSpotTypeLabel(m.type)}`;
  }
  return null;
}

// ---------- chrome ----------

function Legend({ hz, markerCount }: { hz: number; markerCount: number }) {
  return (
    <div className="pointer-events-none absolute right-3 bottom-3 flex items-center gap-3 rounded-xl bg-surface-secondary/80 px-3 py-2 text-xs text-muted backdrop-blur">
      <div className="flex items-center gap-2">
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
        <span className="tabular-nums opacity-60">{hz.toFixed(0)}Hz</span>
      </div>
      {markerCount > 0 && (
        <span className="border-l border-foreground/10 pl-3 tabular-nums">
          {markerCount} hot-spot{markerCount === 1 ? "" : "s"}
        </span>
      )}
    </div>
  );
}

function Hint() {
  return (
    <div className="pointer-events-none absolute top-3 left-3 rounded-xl bg-surface-secondary/80 px-3 py-1.5 text-xs text-muted backdrop-blur">
      drag to orbit · scroll to zoom · hover for detail
    </div>
  );
}
