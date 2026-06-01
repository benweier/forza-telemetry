// client/src/components/hud/BoostGauge.tsx
/* Hallmark · component: boost-gauge · genre: dashboard · theme: Glass */
import type { TickFrame } from "~/types/tick.generated";
import { boostFraction } from "./engine";

/** Display range is a PLACEHOLDER — boost units are unconfirmed (see
 *  docs/data-needed.md). Vacuum end is muted (no blue token in the palette);
 *  positive boost ramps toward --accent. */
const BOOST_MIN = -1;
const BOOST_MAX = 2;

export function BoostGauge({ tick, fresh }: { tick: TickFrame; fresh: boolean }) {
  const boost = tick.bo ?? 0;
  const markerPct = boostFraction(boost, BOOST_MIN, BOOST_MAX) * 100;
  const zeroPct = boostFraction(0, BOOST_MIN, BOOST_MAX) * 100;

  return (
    <div className="flex flex-col gap-2 rounded-2xl bg-surface p-5 shadow-surface">
      <div className="flex items-baseline justify-between">
        <span className="text-xs font-medium tracking-wider text-muted uppercase">Boost</span>
        <span
          className="text-lg font-semibold text-foreground tabular-nums"
          style={{ opacity: fresh ? 1 : 0.5 }}
        >
          {boost >= 0 ? "+" : "−"}
          {Math.abs(boost).toFixed(1)}
        </span>
      </div>
      <div className="relative h-3 overflow-hidden rounded-full bg-surface-secondary">
        {/* zero baseline marker */}
        <span aria-hidden className="absolute top-0 h-full w-px bg-muted/60" style={{ left: `${zeroPct}%` }} />
        {/* positive-boost fill from zero to current */}
        {boost > 0 && (
          <span
            className="absolute top-0 h-full"
            style={{
              left: `${zeroPct}%`,
              width: `${markerPct - zeroPct}%`,
              background: "var(--accent)",
            }}
          />
        )}
        {/* current-value marker */}
        <span
          aria-hidden
          className="absolute -top-0.5 h-4 w-0.5 rounded-full bg-foreground"
          style={{ left: `${markerPct}%` }}
        />
      </div>
      <div className="flex justify-between text-[10px] text-muted">
        <span>vacuum</span>
        <span>0</span>
        <span>max</span>
      </div>
    </div>
  );
}
