/* Hallmark · component: sparkline · genre: dashboard · theme: Glass
 * states: default · empty
 */
import { useEffect, useRef } from "react";
import { displayIndex, readDisplaySlice } from "~/utils/live-store";
import type { TickFrame } from "~/types/tick.generated";

/**
 * Forza Data Out streams at ~60 Hz. The window is sized by sample count, not
 * wall-clock, so each sample maps to a fixed x slot (constant px-per-sample) —
 * that is what keeps the trace from compressing as the buffer fills. If your
 * Data Out rate differs, change this and the "last Ns" label stays honest.
 */
const SAMPLE_HZ = 60;

interface SparklineProps {
  label: string;
  unit?: string;
  /** CSS custom property name for the stroke, e.g. "--accent". */
  colorVar: string;
  accessor: (t: TickFrame) => number;
  format: (v: number) => string;
  /** Symmetric domain around 0 with a zero baseline (for signed channels). */
  signed?: boolean;
  windowSec?: number;
}

/**
 * Canvas sparkline of the last `windowSec` of a single channel, sourced from
 * the live ring buffer. Drives itself off a rAF loop reading the ring
 * imperatively (`readDisplaySlice()` — the ring lives outside the store, by
 * design), keeping 60 Hz redraws off React's render path entirely.
 */
export function Sparkline({
  label,
  unit,
  colorVar,
  accessor,
  format,
  signed = false,
  windowSec = 30,
}: SparklineProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const valueRef = useRef<HTMLSpanElement>(null);
  // Latest props reachable from the rAF loop without restarting it each render.
  const accRef = useRef(accessor);
  accRef.current = accessor;
  const fmtRef = useRef(format);
  fmtRef.current = format;

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return undefined;
    const ctx = canvas.getContext("2d");
    if (!ctx) return undefined;

    const cs = getComputedStyle(document.documentElement);
    const stroke = cs.getPropertyValue(colorVar).trim() || "#F8F8F9";
    const baseline = cs.getPropertyValue("--separator").trim() || "rgba(255,255,255,0.18)";

    let raf = 0;
    let lastNewest: TickFrame | null = null;
    let loggedError = false;

    // Fixed-capacity sample window: the trace spans `capacity` equal-width
    // slots. Fewer samples → it fills from the left; once full, each new sample
    // drops the oldest (FIFO), so it scrolls left without ever rescaling.
    const capacity = Math.max(2, Math.round(windowSec * SAMPLE_HZ));

    const redraw = (ring: readonly TickFrame[]) => {
      const dpr = window.devicePixelRatio || 1;
      const w = canvas.clientWidth;
      const h = canvas.clientHeight;
      if (w === 0 || h === 0) return;
      if (canvas.width !== Math.round(w * dpr) || canvas.height !== Math.round(h * dpr)) {
        canvas.width = Math.round(w * dpr);
        canvas.height = Math.round(h * dpr);
      }
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.clearRect(0, 0, w, h);

      // Last `capacity` samples, oldest first. `start` walks forward as the
      // ring grows past capacity, so the window slides one sample per frame.
      const count = Math.min(ring.length, capacity);
      const start = ring.length - count;
      const newest = ring[ring.length - 1];
      if (valueRef.current) valueRef.current.textContent = fmtRef.current(accRef.current(newest));
      if (count < 2) return;

      const vals: number[] = [];
      for (let j = 0; j < count; j++) vals.push(accRef.current(ring[start + j]));

      let min: number;
      let max: number;
      if (signed) {
        const m = Math.max(0.1, ...vals.map((v) => Math.abs(v)));
        min = -m;
        max = m;
      } else {
        min = Math.min(...vals);
        max = Math.max(...vals);
        if (max - min < 1e-6) max = min + 1;
      }

      // One slot per sample against the fixed capacity, oldest pinned to x=0.
      // Width-per-sample is constant, so the trace fills from the left and
      // never compresses; the windowSec mark is just "buffer full", not a
      // rescale.
      const xAt = (i: number) => (i / (capacity - 1)) * w;
      const yAt = (v: number) => h - 2 - ((v - min) / (max - min)) * (h - 4);

      if (signed) {
        ctx.strokeStyle = baseline;
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(0, yAt(0));
        ctx.lineTo(w, yAt(0));
        ctx.stroke();
      }

      ctx.strokeStyle = stroke;
      ctx.lineWidth = 1.5;
      ctx.lineJoin = "round";
      ctx.beginPath();
      for (let j = 0; j < count; j++) {
        const px = xAt(j);
        const py = yAt(vals[j]);
        if (j === 0) ctx.moveTo(px, py);
        else ctx.lineTo(px, py);
      }
      ctx.stroke();
    };

    const loop = () => {
      // A throw in redraw must not kill the loop permanently — reschedule in
      // `finally` so one bad frame can't blank the canvas for the session. Log
      // the first failure (not every frame) so it isn't swallowed silently.
      try {
        const s = readDisplaySlice();
        // `end` is the delayed "now" index in preview mode, else the latest.
        const end = displayIndex(s);
        if (end >= 0) {
          // Redraw only when the displayed-newest changed (a fresh object ref),
          // so the graph advances per message, not per animation frame. Slice
          // only when delayed — the off path passes the ring as-is (no copy).
          const newest = s.ring[end];
          if (newest !== lastNewest) {
            lastNewest = newest;
            redraw(end === s.ring.length - 1 ? s.ring : s.ring.slice(0, end + 1));
          }
        }
      } catch (error) {
        if (!loggedError) {
          loggedError = true;
          console.error(`Sparkline "${label}" redraw failed; loop continues`, error);
        }
      } finally {
        raf = requestAnimationFrame(loop);
      }
    };
    raf = requestAnimationFrame(loop);

    return () => cancelAnimationFrame(raf);
  }, [colorVar, signed, windowSec, label]);

  return (
    <div className="flex flex-col gap-2 rounded-2xl bg-surface p-5 shadow-surface">
      <div className="flex items-baseline justify-between">
        <span className="text-xs font-medium tracking-wider text-muted uppercase">{label}</span>
        <div className="flex items-baseline gap-1">
          <span ref={valueRef} className="text-base font-semibold text-foreground tabular-nums">
            —
          </span>
          {unit && <span className="text-xs text-muted">{unit}</span>}
        </div>
      </div>
      <canvas
        ref={canvasRef}
        role="img"
        aria-label={`${label} over the last ${windowSec} seconds`}
        className="h-12 w-full"
      />
      <span className="text-xs text-muted">last {windowSec}s</span>
    </div>
  );
}
