/* Hallmark · component: tick-preview-chart · genre: dashboard · theme: Glass
 * states: default · empty · zoomed (60Hz)
 * contrast: pass (axis labels resolved via CSS vars)
 */
import { Button, Chip } from "@heroui/react";
import { Icon } from "@iconify/react";
import { useEffect, useRef } from "react";
import uPlot from "uplot";
import "uplot/dist/uPlot.min.css";
import type { PreviewSample, TicksResponse } from "~/utils/schemas";

interface TickPreviewChartProps {
  /** 1Hz aggregate preview spanning the whole stint. */
  samples: PreviewSample[];
  /** Stint start in server_recv ns — the x-axis origin for both resolutions. */
  startedAtNs: number;
  /** Full-rate (60Hz) window from the tick endpoint; when present, replaces the preview line. */
  detail?: TicksResponse | null;
  /** Whether the detail window is currently fetching. */
  isFetchingDetail?: boolean;
  /** Called with an absolute ns window when the user drag-selects a region. */
  onWindowSelect: (fromNs: number, toNs: number) => void;
  /** Whether a zoom window is active (controls the Reset affordance). */
  isZoomed: boolean;
  /** Clears the zoom window back to the full preview. */
  onReset: () => void;
  /** Optional note shown when the selection can't be upgraded (e.g. > 60s). */
  note?: string;
}

interface Normalized {
  x: number[];
  speed: (number | null)[];
  lat: (number | null)[];
  brake: (number | null)[];
}

function fromPreview(samples: PreviewSample[], startNs: number): Normalized {
  return {
    x: samples.map((s) => (s.tick_ns - startNs) / 1e9),
    speed: samples.map((s) => s.speed_ms),
    lat: samples.map((s) => (s.lateral_g === null ? null : Math.abs(s.lateral_g))),
    brake: samples.map((s) => (s.brake_pct === null ? null : s.brake_pct * 100)),
  };
}

function fromDetail(detail: TicksResponse, startNs: number): Normalized {
  const idx = (name: string) => detail.columns.indexOf(name);
  const ix = idx("server_recv_ns");
  const is = idx("speed_ms");
  const il = idx("lateral_g");
  const ib = idx("brake_pct");
  const x: number[] = [];
  const speed: (number | null)[] = [];
  const lat: (number | null)[] = [];
  const brake: (number | null)[] = [];
  for (const row of detail.rows) {
    const ns = row[ix];
    x.push(ns === null ? Number.NaN : (ns - startNs) / 1e9);
    speed.push(is < 0 ? null : row[is]);
    const lg = il < 0 ? null : row[il];
    lat.push(lg === null ? null : Math.abs(lg));
    const bk = ib < 0 ? null : row[ib];
    brake.push(bk === null ? null : bk * 100);
  }
  return { x, speed, lat, brake };
}

/**
 * Stacked time-series for the stint: speed (m/s), absolute lateral G, and brake
 * percentage. Defaults to the 1Hz aggregate preview spanning the whole stint;
 * drag-selecting a region (≤60s) asks the parent to fetch the full-rate 60Hz
 * window, which then replaces the preview line in place.
 *
 * uPlot is canvas-backed so this scales to tens of thousands of samples without
 * React reconciliation cost. `drag.setScale` is disabled so the selection
 * drives a data swap (1Hz → 60Hz) rather than an in-place visual zoom.
 */
export function TickPreviewChart({
  samples,
  startedAtNs,
  detail,
  isFetchingDetail,
  onWindowSelect,
  isZoomed,
  onReset,
  note,
}: TickPreviewChartProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<uPlot | null>(null);
  // Keep the latest callback reachable from uPlot's imperative hook without
  // rebuilding the plot on every render.
  const selectRef = useRef(onWindowSelect);
  selectRef.current = onWindowSelect;

  const active = detail && detail.rows.length >= 2 ? detail : null;

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const norm = active ? fromDetail(active, startedAtNs) : fromPreview(samples, startedAtNs);
    if (norm.x.length < 2) return;

    const data: uPlot.AlignedData = [norm.x, norm.speed, norm.lat, norm.brake];

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
      // setScale:false → drag selects a window for the parent to fetch rather
      // than zooming the existing series in place.
      cursor: { drag: { x: true, y: false, setScale: false }, focus: { prox: 30 } },
      hooks: {
        setSelect: [
          (u) => {
            if (u.select.width <= 0) return;
            const start = u.posToVal(u.select.left, "x");
            const end = u.posToVal(u.select.left + u.select.width, "x");
            // Clear the highlight; the data swap (or note) is the feedback.
            u.setSelect({ left: 0, top: 0, width: 0, height: 0 }, false);
            selectRef.current(
              Math.round(startedAtNs + start * 1e9),
              Math.round(startedAtNs + end * 1e9),
            );
          },
        ],
      },
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
  }, [samples, active, startedAtNs]);

  if (samples.length < 2) {
    return (
      <div className="grid h-[320px] place-items-center rounded-2xl bg-surface text-sm text-muted shadow-surface">
        Need at least 2 samples to plot
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-3 rounded-2xl bg-surface p-4 shadow-surface">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Chip size="sm" variant="soft" color={active ? "success" : "default"}>
            {isFetchingDetail ? "Loading 60 Hz…" : active ? "60 Hz" : "1 Hz preview"}
          </Chip>
          {note && <span className="text-xs text-muted">{note}</span>}
          {!isZoomed && !note && (
            <span className="text-xs text-muted">Drag to load a ≤60s window at full rate</span>
          )}
        </div>
        {isZoomed && (
          <Button size="sm" variant="tertiary" onPress={onReset}>
            <Icon icon="lucide:rotate-ccw" className="mr-1.5 size-4" />
            Reset
          </Button>
        )}
      </div>
      <div ref={containerRef} className="w-full" />
    </div>
  );
}
