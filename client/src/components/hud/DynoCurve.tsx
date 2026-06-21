// client/src/components/hud/DynoCurve.tsx
/* Hallmark · component: dyno-curve · genre: dashboard · theme: Glass */
import { useEffect, useRef } from "react";
import { displayTick, useLiveStore } from "~/utils/live-store";
import { DynoEnvelope } from "./engine";
import { EngineBadge } from "./EngineBadge";
import type { TickFrame } from "~/types/tick.generated";

/** Live power+torque envelope vs RPM. Power uses --accent, torque --warning. */
export function DynoCurve({ tick }: { tick: TickFrame }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const cs = getComputedStyle(document.documentElement);
    const powerColor = cs.getPropertyValue("--accent").trim() || "#F8F8F9";
    const torqueColor = cs.getPropertyValue("--warning").trim() || "#F5A524";
    const axisColor = cs.getPropertyValue("--separator").trim() || "rgba(255,255,255,0.18)";
    const cursorColor = cs.getPropertyValue("--muted").trim() || "rgba(255,255,255,0.5)";
    const dangerColor = cs.getPropertyValue("--danger").trim() || "#F31260";

    const env = new DynoEnvelope();
    let raf = 0;
    let lastNewest: TickFrame | null = null;
    let loggedError = false;

    const redraw = (t: TickFrame) => {
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

      env.update(t.rpm ?? 0, t.pw ?? 0, t.tq ?? 0, t.co ?? 0);
      const buckets = env.buckets();
      const rpmMax = Math.max(t.rmx ?? 0, t.rpm ?? 0, 1);

      const pad = 6;
      const xAt = (rpm: number) => pad + (Math.min(rpm, rpmMax) / rpmMax) * (w - 2 * pad);
      let maxPower = 1;
      let maxTorque = 1;
      for (const b of buckets) {
        if (b.powerKW > maxPower) maxPower = b.powerKW;
        if (b.torqueNm > maxTorque) maxTorque = b.torqueNm;
      }
      const yPower = (kw: number) => h - pad - (kw / maxPower) * (h - 2 * pad);
      const yTorque = (nm: number) => h - pad - (nm / maxTorque) * (h - 2 * pad);

      // redline zone (last 12% of the rev range)
      ctx.fillStyle = dangerColor;
      ctx.globalAlpha = 0.08;
      const redX = xAt(rpmMax * 0.88);
      ctx.fillRect(redX, 0, w - pad - redX, h);
      ctx.globalAlpha = 1;

      // axis baseline
      ctx.strokeStyle = axisColor;
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(pad, h - pad);
      ctx.lineTo(w - pad, h - pad);
      ctx.stroke();

      const drawCurve = (
        color: string,
        yOf: (b: number) => number,
        key: "powerKW" | "torqueNm",
      ) => {
        if (buckets.length < 2) return;
        ctx.strokeStyle = color;
        ctx.lineWidth = 2;
        ctx.lineJoin = "round";
        ctx.beginPath();
        buckets.forEach((b, j) => {
          const px = xAt(b.rpm);
          const py = yOf(b[key]);
          if (j === 0) ctx.moveTo(px, py);
          else ctx.lineTo(px, py);
        });
        ctx.stroke();
      };
      drawCurve(powerColor, yPower, "powerKW");
      drawCurve(torqueColor, yTorque, "torqueNm");

      // live cursor at current rpm
      const cx = xAt(t.rpm ?? 0);
      ctx.strokeStyle = cursorColor;
      ctx.setLineDash([3, 3]);
      ctx.beginPath();
      ctx.moveTo(cx, pad);
      ctx.lineTo(cx, h - pad);
      ctx.stroke();
      ctx.setLineDash([]);

      // dots where the cursor meets each curve (current live power/torque)
      const powerY = yPower((t.pw ?? 0) / 1000);
      const torqueY = yTorque(t.tq ?? 0);
      ctx.fillStyle = powerColor;
      ctx.beginPath();
      ctx.arc(cx, powerY, 3, 0, Math.PI * 2);
      ctx.fill();
      ctx.fillStyle = torqueColor;
      ctx.beginPath();
      ctx.arc(cx, torqueY, 3, 0, Math.PI * 2);
      ctx.fill();
    };

    const loop = () => {
      try {
        const newest = displayTick(useLiveStore.getState());
        if (newest && newest !== lastNewest) {
          lastNewest = newest;
          redraw(newest);
        }
      } catch (error) {
        if (!loggedError) {
          loggedError = true;
          console.error("DynoCurve redraw failed; loop continues", error);
        }
      } finally {
        raf = requestAnimationFrame(loop);
      }
    };
    raf = requestAnimationFrame(loop);
    return () => cancelAnimationFrame(raf);
  }, []);

  return (
    <div className="flex flex-col gap-2 rounded-2xl bg-surface p-5 shadow-surface">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium tracking-wider text-muted uppercase">
          Power &amp; torque
        </span>
        <EngineBadge tick={tick} />
      </div>
      <canvas
        ref={canvasRef}
        role="img"
        aria-label="Live power and torque against engine RPM"
        className="h-32 w-full"
      />
      <div className="flex gap-4 text-[10px]">
        <span className="text-accent">■ power (kW)</span>
        <span className="text-warning">■ torque (Nm)</span>
      </div>
    </div>
  );
}
