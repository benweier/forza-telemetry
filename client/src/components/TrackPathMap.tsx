/* Hallmark · component: track-path-map · genre: dashboard · theme: Glass
 * 3D track-path minimap. The polyline can be coloured by any of the path
 * channels the server ships (speed / brake / lateral G); pick via the
 * `channel` prop.
 */
import { COORDINATE_SYSTEM, OrbitView, type PickingInfo } from "@deck.gl/core";
import { LineLayer } from "@deck.gl/layers";
import DeckGL from "@deck.gl/react";
import { useMemo } from "react";
import type { PathResponse } from "~/utils/schemas";

export type PathChannel = "speed" | "brake" | "lateral_g";

interface TrackPathMapProps {
  path: PathResponse;
  channel?: PathChannel;
}

interface Segment {
  source: [number, number, number];
  target: [number, number, number];
  value: number; // the channel's value at this segment's end
  lap: number | null;
  tickNS: number;
}

export function TrackPathMap({ path, channel = "speed" }: TrackPathMapProps) {
  const cfg = channelConfig[channel];

  const { segments, extent } = useMemo(
    () => buildScene(path, cfg.colIndex),
    [path, cfg.colIndex],
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
      getColor: (s) => valueToColor(s.value, cfg.max),
      getWidth: 3,
      widthUnits: "pixels",
      widthMinPixels: 2,
      pickable: true,
      updateTriggers: { getColor: channel },
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
        getTooltip={(info) => tooltipFor(info, channel)}
      />
      <Legend hz={path.sample_hz} channel={channel} />
      <Hint />
    </div>
  );
}

// ---------- channel config ----------

interface ChannelConfig {
  label: string;
  colIndex: number;
  max: number;
  legendLow: string;
  legendHigh: string;
  format: (v: number) => string;
}

// Column indices match the server's pathColumns order (server/internal/api/path.go).
const channelConfig: Record<PathChannel, ChannelConfig> = {
  speed: {
    label: "Speed",
    colIndex: 4,
    max: 90,
    legendLow: "slow",
    legendHigh: "fast",
    format: (v) => `${(v * 3.6).toFixed(0)} km/h`,
  },
  brake: {
    label: "Brake",
    colIndex: 6,
    max: 1.0,
    legendLow: "off",
    legendHigh: "100%",
    format: (v) => `${Math.round(v * 100)}% brake`,
  },
  lateral_g: {
    label: "Lateral G",
    colIndex: 7,
    max: 2.0,
    legendLow: "none",
    legendHigh: "hard",
    format: (v) => `${Math.abs(v).toFixed(2)} G`,
  },
};

// ---------- scene assembly ----------

const COL_TICK_NS = 0;
const COL_POS_X = 1;
const COL_POS_Y = 2;
const COL_POS_Z = 3;
const COL_LAP = 5;

interface PathPoint {
  tickNS: number;
  x: number;
  y: number;
  z: number;
  channelValue: number;
  lap: number | null;
}

function buildScene(
  path: PathResponse,
  channelColIndex: number,
): { segments: Segment[]; extent: number } {
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
      channelValue: row[channelColIndex] ?? 0,
      lap: row[COL_LAP],
    });
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
      value: b.channelValue,
      lap: b.lap,
      tickNS: b.tickNS,
    };
  }

  return { segments, extent };
}

// ---------- visuals ----------

/**
 * Normalised value → 5-stop cool→hot ramp. Each channel supplies its own
 * `max`; the value is clipped to [0, 1].
 */
function valueToColor(raw: number, max: number): [number, number, number, number] {
  const t = Math.min(Math.max(Math.abs(raw) / max, 0), 1);
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

// ---------- tooltip ----------

function tooltipFor(info: PickingInfo, channel: PathChannel): string | null {
  if (!info.object) return null;
  if (info.layer?.id !== "track-path") return null;
  const seg = info.object as Segment;
  const cfg = channelConfig[channel];
  const channelDisplay = cfg.format(seg.value);
  const lap = seg.lap !== null ? `Lap ${seg.lap}` : "—";
  return `${channelDisplay} · ${lap}`;
}

// ---------- chrome ----------

function Legend({ hz, channel }: { hz: number; channel: PathChannel }) {
  const cfg = channelConfig[channel];
  return (
    <div className="pointer-events-none absolute right-3 bottom-3 flex items-center gap-2 rounded-xl bg-surface-secondary/80 px-3 py-2 text-xs text-muted backdrop-blur">
      <span className="font-medium text-foreground/80">{cfg.label}</span>
      <span>{cfg.legendLow}</span>
      <span
        aria-hidden
        className="h-2 w-24 rounded-full"
        style={{
          background:
            "linear-gradient(to right, rgb(40,90,200), rgb(80,200,220), rgb(120,220,100), rgb(240,200,60), rgb(240,80,60))",
        }}
      />
      <span>{cfg.legendHigh}</span>
      <span className="tabular-nums opacity-60">{hz.toFixed(0)}Hz</span>
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
