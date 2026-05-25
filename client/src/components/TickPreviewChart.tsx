/* Hallmark · component: tick-preview-chart · genre: dashboard · theme: Glass
 * states: default · empty
 * contrast: pass (axis labels resolved via CSS vars)
 */
import { useEffect, useRef } from "react";
import uPlot from "uplot";
import "uplot/dist/uPlot.min.css";

import type { PreviewSample } from "~/utils/schemas";

interface TickPreviewChartProps {
  samples: PreviewSample[];
}

/**
 * Stacked time-series for the 1Hz preview: speed (m/s), absolute lateral G,
 * and brake percentage. uPlot is canvas-backed so this scales to tens of
 * thousands of samples without React reconciliation cost.
 *
 * Theme integration: uPlot strokes/fills are CSS-var-driven where possible,
 * so dark / light mode picks up from `:root` without re-rendering.
 */
export function TickPreviewChart({ samples }: TickPreviewChartProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<uPlot | null>(null);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    if (samples.length < 2) return;

    const data: uPlot.AlignedData = [
      samples.map((s) => s.second_index),
      samples.map((s) => s.speed_ms ?? null),
      samples.map((s) => (s.lateral_g === null ? null : Math.abs(s.lateral_g))),
      samples.map((s) => (s.brake_pct === null ? null : s.brake_pct * 100)),
    ];

    const cs = getComputedStyle(document.documentElement);
    const foreground = cs.getPropertyValue("--foreground").trim() || "#FCFCFC";
    const muted = cs.getPropertyValue("--muted").trim() || "#9DA0A4";
    const separator = cs.getPropertyValue("--separator").trim() || "rgba(255,255,255,0.12)";
    const accent = cs.getPropertyValue("--accent").trim() || "#F8F8F9";
    const warning = cs.getPropertyValue("--warning").trim() || "#F7B750";
    const danger = cs.getPropertyValue("--danger").trim() || "#DB3B3E";

    const opts: uPlot.Options = {
      width: container.clientWidth,
      height: 320,
      padding: [12, 16, 0, 0],
      legend: { show: true, markers: { width: 2 } },
      cursor: { drag: { x: true, y: false }, focus: { prox: 30 } },
      scales: {
        x: { time: false },
        speed: {},
        lat: {},
        brake: { range: [0, 100] },
      },
      axes: [
        {
          stroke: muted,
          grid: { stroke: separator, width: 1 },
          ticks: { stroke: separator },
          font: "12px Inter, system-ui",
          label: "Second",
          labelGap: 4,
          labelSize: 22,
          labelFont: "12px Inter, system-ui",
        },
        {
          scale: "speed",
          stroke: muted,
          grid: { stroke: separator, width: 1 },
          ticks: { stroke: separator },
          font: "12px Inter, system-ui",
          label: "Speed (m/s)",
          labelGap: 4,
          labelSize: 22,
          labelFont: "12px Inter, system-ui",
        },
        {
          scale: "lat",
          side: 1,
          stroke: muted,
          grid: { show: false },
          ticks: { stroke: separator },
          font: "12px Inter, system-ui",
          label: "Lateral G / Brake %",
          labelGap: 4,
          labelSize: 22,
          labelFont: "12px Inter, system-ui",
        },
      ],
      series: [
        { label: "second" },
        {
          label: "Speed",
          stroke: accent,
          width: 2,
          scale: "speed",
          value: (_u, v) => (v == null ? "—" : `${v.toFixed(1)} m/s`),
        },
        {
          label: "Lateral G",
          stroke: warning,
          width: 1.5,
          scale: "lat",
          value: (_u, v) => (v == null ? "—" : `${v.toFixed(2)}G`),
        },
        {
          label: "Brake %",
          stroke: danger,
          width: 1.5,
          scale: "lat",
          value: (_u, v) => (v == null ? "—" : `${v.toFixed(0)}%`),
        },
      ],
    };

    const plot = new uPlot(opts, data, container);
    chartRef.current = plot;

    const ro = new ResizeObserver(() => {
      plot.setSize({ width: container.clientWidth, height: 320 });
    });
    ro.observe(container);

    // Apply CSS-var driven colors to the legend table after mount, since uPlot
    // injects its own structure outside React's reach.
    const legend = container.querySelector(".u-legend");
    if (legend instanceof HTMLElement) {
      legend.style.color = foreground;
      legend.style.fontFamily = "Inter, system-ui";
      legend.style.fontSize = "12px";
    }

    return () => {
      ro.disconnect();
      plot.destroy();
      chartRef.current = null;
    };
  }, [samples]);

  if (samples.length < 2) {
    return (
      <div className="grid h-[320px] place-items-center rounded-2xl bg-surface text-sm text-muted shadow-surface">
        Need at least 2 samples to plot
      </div>
    );
  }

  return (
    <div className="rounded-2xl bg-surface p-4 shadow-surface">
      <div ref={containerRef} className="w-full" />
    </div>
  );
}
