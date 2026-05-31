/* Hallmark · component: sparkline · genre: dashboard · theme: Glass
 * states: default · empty
 */
import { useEffect, useRef } from "react";
import type { TickFrame } from "~/types/tick.generated";
import { useLiveStore } from "~/utils/live-store";

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
 * the live ring buffer. Drives itself off a rAF loop reading the store
 * imperatively — `push()` mutates the ring in place (stable ref), so a Zustand
 * subscription wouldn't fire; polling the ring per frame sidesteps that and
 * keeps redraws off React's render path entirely.
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
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const cs = getComputedStyle(document.documentElement);
    const stroke = cs.getPropertyValue(colorVar).trim() || "#F8F8F9";
    const baseline = cs.getPropertyValue("--separator").trim() || "rgba(255,255,255,0.18)";

    let raf = 0;
    let lastTs = Number.NaN;
    let loggedError = false;

    const redraw = (ring: TickFrame[]) => {
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

      const newest = ring[ring.length - 1];
      if (valueRef.current) valueRef.current.textContent = fmtRef.current(accRef.current(newest));

      // `ts` is ns on the wire (epoch ~1.78e18); guard for a ms fallback.
      const unitPerSec = newest.sts > 1e15 ? 1e9 : 1e3;
      const cutoff = newest.sts - windowSec * unitPerSec;
      const pts: TickFrame[] = [];
      for (let i = ring.length - 1; i >= 0; i--) {
        if (ring[i].sts < cutoff) break;
        pts.push(ring[i]);
      }
      pts.reverse();
      if (pts.length < 2) return;

      const vals = pts.map((p) => accRef.current(p));
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

      const t0 = pts[0].sts;
      const span = Math.max(1, newest.sts - t0);
      const xAt = (ts: number) => ((ts - t0) / span) * w;
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
      for (let i = 0; i < pts.length; i++) {
        const px = xAt(pts[i].sts);
        const py = yAt(vals[i]);
        if (i === 0) ctx.moveTo(px, py);
        else ctx.lineTo(px, py);
      }
      ctx.stroke();
    };

    const loop = () => {
      // A throw in redraw must not kill the loop permanently — reschedule in
      // `finally` so one bad frame can't blank the canvas for the session. Log
      // the first failure (not every frame) so it isn't swallowed silently.
      try {
        const ring = useLiveStore.getState().ring;
        if (ring.length > 0) {
          const newestTs = ring[ring.length - 1].sts;
          if (newestTs !== lastTs) {
            lastTs = newestTs;
            redraw(ring);
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
