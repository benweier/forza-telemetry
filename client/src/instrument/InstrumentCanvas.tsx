import { useEffect, useRef, useState } from "react";
import { displayTick, useLiveStore } from "~/utils/live-store";
import { makeSmoother, stepSmoother, type Smoother } from "./core/physics";
import { targetsFromTick, buildInstrumentState } from "./core/state";
import { RawInstrumentRenderer } from "./raw/raw-renderer";

const STIFF = 90;

export function InstrumentCanvas() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const renderer = new RawInstrumentRenderer();
    let raf = 0;
    let disposed = false;

    const ch: Record<string, Smoother> = {
      speed: makeSmoother(0),
      rpm: makeSmoother(0),
      thr: makeSmoother(0),
      brk: makeSmoother(0),
      gx: makeSmoother(0),
      gy: makeSmoother(0),
    };
    let last = performance.now();
    let gear = "N";
    let rmx = 8000;

    const sizeToBox = () => {
      const dpr = window.devicePixelRatio || 1;
      const r = canvas.getBoundingClientRect();
      canvas.width = Math.max(1, Math.round(r.width * dpr));
      canvas.height = Math.max(1, Math.round(r.height * dpr));
      renderer.resize(r.width, r.height, dpr);
    };

    const loop = () => {
      try {
        const now = performance.now();
        const dt = Math.min(0.05, (now - last) / 1000);
        last = now;
        const t = displayTick(useLiveStore.getState());
        if (t) {
          const tg = targetsFromTick(t, rmx);
          rmx = tg.rmx;
          gear = tg.gear;
          ch.speed = stepSmoother(ch.speed, tg.speedKmh, dt, STIFF);
          ch.rpm = stepSmoother(ch.rpm, tg.rpm, dt, STIFF);
          ch.thr = stepSmoother(ch.thr, tg.throttle, dt, STIFF);
          ch.brk = stepSmoother(ch.brk, tg.brake, dt, STIFF);
          ch.gx = stepSmoother(ch.gx, tg.gx, dt, STIFF);
          ch.gy = stepSmoother(ch.gy, tg.gy, dt, STIFF);
        }
        const state = buildInstrumentState({
          speedKmh: ch.speed.value,
          rpm: ch.rpm.value,
          throttle: ch.thr.value,
          brake: ch.brk.value,
          gx: ch.gx.value,
          gy: ch.gy.value,
          gear,
          rmx,
        });
        renderer.render(state);
      } catch (err) {
        if (!disposed) console.error("instrument render failed; loop continues", err);
      } finally {
        raf = requestAnimationFrame(loop);
      }
    };

    const ro = new ResizeObserver(sizeToBox);

    renderer
      .init(canvas, {})
      .then(() => {
        if (disposed) return;
        sizeToBox();
        ro.observe(canvas);
        raf = requestAnimationFrame(loop);
      })
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));

    return () => {
      disposed = true;
      cancelAnimationFrame(raf);
      ro.disconnect();
      renderer.destroy();
    };
  }, []);

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 rounded-2xl bg-surface p-10 text-center shadow-surface">
        <span className="text-sm font-medium text-foreground">WebGPU required</span>
        <span className="text-xs text-muted">{error}</span>
        <a href="/live" className="text-xs text-accent underline">
          Use the HUD view instead
        </a>
      </div>
    );
  }
  return (
    <canvas
      ref={canvasRef}
      className="aspect-[16/9] w-full rounded-2xl bg-surface shadow-surface"
    />
  );
}
