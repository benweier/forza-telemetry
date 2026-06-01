// client/src/components/hud/CarDiagram.tsx
/* Hallmark · component: car-diagram · genre: dashboard · theme: Glass */
import type { TickFrame } from "~/types/tick.generated";
import {
  classifyWheelSlip,
  heatScaleColor,
  ringFromCombinedSlip,
  slipAngleTick,
} from "./tire-scale";
import { drivenAxle } from "./engine";

/** Per-wheel order is FL, FR, RL, RR everywhere in this codebase. */
const WHEELS = [
  { i: 0, cx: 70, cy: 112, barX: 28, barSide: "left" as const },
  { i: 1, cx: 234, cy: 112, barX: 271, barSide: "right" as const },
  { i: 2, cx: 70, cy: 290, barX: 28, barSide: "left" as const },
  { i: 3, cx: 234, cy: 290, barX: 271, barSide: "right" as const },
];

const WHEEL_W = 36;
const WHEEL_H = 52;
const BAR_W = 5;

function Corner({ tick, wheel }: { tick: TickFrame; wheel: (typeof WHEELS)[number] }) {
  const { i, cx, cy, barX } = wheel;
  const temp = tick.tt[i] ?? 0;
  const ring = ringFromCombinedSlip(tick.tcs[i] ?? 0);
  const slip = classifyWheelSlip(tick.tsr[i] ?? 0);
  const arrow = slipAngleTick(tick.tsa[i] ?? 0);
  // Normalised suspension travel 0..1 → bar fills upward from the wheel centre.
  const travel = Math.max(0, Math.min(1, tick.stn[i] ?? 0));
  const barH = travel * WHEEL_H;

  const x = cx - WHEEL_W / 2;
  const y = cy - WHEEL_H / 2;

  return (
    <g>
      {/* combined-slip ring */}
      <circle
        cx={cx}
        cy={cy}
        r={WHEEL_W / 2 + 6}
        fill="none"
        stroke={ring.color}
        strokeWidth={ring.strokeWidth}
        opacity={0.9}
      />
      {/* tire body, grip-window heat fill */}
      <rect x={x} y={y} width={WHEEL_W} height={WHEEL_H} rx={11} fill={heatScaleColor(temp)} />
      <text x={cx} y={cy + 4} textAnchor="middle" fontSize={11} className="fill-background">
        {Math.round(temp)}°
      </text>
      {/* slip-angle tick from the wheel centre (horizontal, signed) */}
      {arrow.length > 0 && (
        <line
          x1={cx}
          y1={cy - WHEEL_H / 2 - 4}
          x2={cx + arrow.dir * arrow.length}
          y2={cy - WHEEL_H / 2 - 4}
          stroke="var(--muted)"
          strokeWidth={2}
        />
      )}
      {/* lockup / spin tag */}
      {slip && (
        <text
          x={cx}
          y={cy + WHEEL_H / 2 + 14}
          textAnchor="middle"
          fontSize={8}
          style={{ fill: slip === "spin" ? "var(--danger)" : "var(--warning)" }}
        >
          {slip === "spin" ? "SPIN" : "LOCK"}
        </text>
      )}
      {/* suspension-travel side bar */}
      <rect x={barX} y={y} width={BAR_W} height={WHEEL_H} rx={BAR_W / 2} fill="var(--surface-secondary)" />
      <rect
        x={barX}
        y={y + (WHEEL_H - barH)}
        width={BAR_W}
        height={barH}
        rx={BAR_W / 2}
        fill="var(--muted)"
      />
    </g>
  );
}

export function CarDiagram({ tick, fresh }: { tick: TickFrame; fresh: boolean }) {
  const axle = drivenAxle(tick.dt);

  return (
    <div className="flex h-full flex-col gap-3 rounded-2xl bg-surface p-5 shadow-surface">
      <span className="text-xs font-medium tracking-wider text-muted uppercase">Tires &amp; chassis</span>
      <svg
        viewBox="0 0 304 402"
        role="img"
        aria-label="Top-down tire and chassis diagram"
        className="mx-auto my-auto w-full max-w-[300px]"
        style={{ opacity: fresh ? 1 : 0.5 }}
      >
        {/* body + cabin */}
        <rect x={92} y={36} width={120} height={330} rx={42} fill="var(--surface-secondary)" />
        <rect x={118} y={88} width={68} height={112} rx={12} fill="var(--surface-tertiary)" />
        {/* driven-axle highlight */}
        {(axle === "front" || axle === "both") && (
          <rect x={144} y={96} width={16} height={70} rx={6} fill="var(--success-soft)" />
        )}
        {(axle === "rear" || axle === "both") && (
          <rect x={144} y={236} width={16} height={70} rx={6} fill="var(--success-soft)" />
        )}
        {WHEELS.map((w) => (
          <Corner key={w.i} tick={tick} wheel={w} />
        ))}
      </svg>
      <div className="flex flex-wrap gap-x-4 gap-y-1 text-[10px] text-muted">
        <span>Fill: temp</span>
        <span>Ring: slip</span>
        <span>Bar: suspension</span>
        <span>Arrow: slip angle</span>
      </div>
    </div>
  );
}
