/* Hallmark · component: live-hud · genre: dashboard · theme: Glass */
import { Chip } from "@heroui/react";
import { Icon } from "@iconify/react";
import { Link, createFileRoute } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { Sparkline } from "~/components/Sparkline";
import { formatCount, gearLabel } from "~/utils/format";
import { useLiveStore } from "~/utils/live-store";
import { LiveSocket } from "~/utils/ws";
import type { TickFrame } from "~/types/tick.generated";

export const Route = createFileRoute("/live")({
  component: LiveRoute,
});

/** Tick is "fresh" if a frame arrived within this window. Forza streams at
 *  ~60Hz so 2 s of silence reliably means the game stopped sending. */
const STALE_AFTER_MS = 2000;

/** Owns the live WebSocket lifecycle for a route: opens on mount, closes on
 *  unmount. Both the HUD and Cluster routes call this so each maintains its own
 *  connection — switching between them reconnects rather than tearing the only
 *  socket down (the HUD route used to be the sole owner). */
export function useLiveSocket(): void {
  useEffect(() => {
    const socket = new LiveSocket("/api/v1/live");
    socket.start();
    return () => socket.stop();
  }, []);
}

/** Subscribes to connection state and re-evaluates staleness every 500 ms so
 *  the status pill updates without needing a new tick frame. */
export function useLiveStatus(): { connected: boolean; fresh: boolean } {
  const connected = useLiveStore((s) => s.connected);
  const lastPushedAt = useLiveStore((s) => s.lastPushedAt);

  const [, force] = useState(0);
  useEffect(() => {
    const id = setInterval(() => force((n) => n + 1), 500);
    return () => clearInterval(id);
  }, []);

  const fresh = lastPushedAt !== null && Date.now() - lastPushedAt < STALE_AFTER_MS;
  return { connected, fresh };
}

function LiveRoute() {
  // Subscribe individually to minimise re-render fan-out — push() touches
  // ring on every frame but only `latest` and `lastPushedAt` matter for the
  // HUD body, and `connected` is independent.
  const latest = useLiveStore((s) => s.latest);

  useLiveSocket();
  const { connected, fresh } = useLiveStatus();

  return (
    <section className="flex flex-col gap-8">
      <header className="flex items-baseline justify-between gap-4">
        <div className="flex flex-col gap-1">
          <span className="text-xs font-medium tracking-wider text-muted uppercase">Realtime</span>
          <h1 className="text-3xl font-semibold tracking-tight text-foreground">Live HUD</h1>
        </div>
        <div className="flex items-center gap-3">
          <LiveViewToggle active="hud" />
          <StatusPill connected={connected} fresh={fresh} />
        </div>
      </header>

      {!latest && <WaitingPanel connected={connected} />}
      {latest && <HUD tick={latest} fresh={fresh} />}
    </section>
  );
}

export function StatusPill({ connected, fresh }: { connected: boolean; fresh: boolean }) {
  if (connected && fresh) {
    return (
      <Chip size="sm" variant="soft" color="success">
        <span className="flex items-center gap-1.5">
          <span aria-hidden className="size-1.5 animate-pulse rounded-full bg-success" />
          Live
        </span>
      </Chip>
    );
  }
  if (connected) {
    return (
      <Chip size="sm" variant="soft" color="warning">
        Connected · no data
      </Chip>
    );
  }
  return (
    <Chip size="sm" variant="soft">
      <span className="flex items-center gap-1.5">
        <span aria-hidden className="size-1.5 rounded-full bg-muted" />
        Disconnected
      </span>
    </Chip>
  );
}

function WaitingPanel({ connected }: { connected: boolean }) {
  return (
    <div className="rounded-2xl bg-surface p-12 shadow-surface">
      <div className="mx-auto flex max-w-md flex-col items-center gap-4 text-center">
        <span className="grid size-12 place-items-center rounded-2xl bg-accent-soft text-accent-soft-foreground">
          <Icon icon="lucide:radio-tower" className="size-6" />
        </span>
        <h2 className="text-xl font-semibold tracking-tight text-foreground">
          {connected ? "Waiting for telemetry" : "Connecting…"}
        </h2>
        <p className="text-sm text-pretty text-muted">
          Open Forza Horizon, enable Data Out under HUD settings, point the IP at this host on UDP
          7100. Frames will appear here as soon as the game starts streaming.
        </p>
      </div>
    </div>
  );
}

// ---------- HUD body ----------

function HUD({ tick, fresh }: { tick: TickFrame; fresh: boolean }) {
  // Speed wire value is m/s; convert for the big readout.
  const kmh = (tick.sp ?? 0) * 3.6;
  // RPM bar, normalised against max.
  const rpm = tick.rpm ?? 0;
  const rpmMax = Math.max(tick.rmx ?? 0, rpm);
  const rpmPct = rpmMax > 0 ? Math.min(1, rpm / rpmMax) : 0;
  const redlinePct = 0.88;

  return (
    <div className="grid gap-4 lg:grid-cols-[1.4fr_1fr]" data-stale={!fresh}>
      <div className="flex flex-col gap-4">
        <SpeedCard kmh={kmh} gear={tick.g ?? 0} fresh={fresh} />
        <RpmBar rpm={rpm} rpmMax={tick.rmx ?? 0} pct={rpmPct} redlinePct={redlinePct} />
        <div className="grid grid-cols-2 gap-4">
          <InputBar label="Throttle" value={tick.tp ?? 0} tone="success" />
          <InputBar label="Brake" value={tick.bp ?? 0} tone="danger" />
        </div>
        <div className="grid grid-cols-2 gap-4">
          <Sparkline
            label="Speed"
            unit="km/h"
            colorVar="--accent"
            accessor={(t) => (t.sp ?? 0) * 3.6}
            format={(v) => `${Math.round(v)}`}
          />
          <Sparkline
            label="Lateral G"
            unit="G"
            colorVar="--warning"
            signed
            accessor={(t) => t.lg ?? 0}
            format={(v) => `${v >= 0 ? "+" : "−"}${Math.abs(v).toFixed(2)}`}
          />
        </div>
      </div>

      <aside className="flex flex-col gap-4">
        <GForcePanel latG={tick.lg ?? 0} longG={tick.lng ?? 0} />
        <MetaPanel tick={tick} />
      </aside>
    </div>
  );
}

function SpeedCard({ kmh, gear, fresh }: { kmh: number; gear: number; fresh: boolean }) {
  return (
    <div className="rounded-2xl bg-surface p-6 shadow-surface">
      <div className="flex items-end justify-between gap-6">
        <div className="flex flex-col">
          <span className="text-xs font-medium tracking-wider text-muted uppercase">Speed</span>
          <div className="flex items-baseline gap-2 leading-none">
            <span
              className="text-7xl font-semibold tracking-tight text-foreground tabular-nums"
              style={{ opacity: fresh ? 1 : 0.5 }}
            >
              {Math.round(kmh)}
            </span>
            <span className="text-sm text-muted">km/h</span>
          </div>
        </div>
        <div className="flex flex-col items-end">
          <span className="text-xs font-medium tracking-wider text-muted uppercase">Gear</span>
          <span
            className="text-5xl leading-none font-semibold text-foreground tabular-nums"
            style={{ opacity: fresh ? 1 : 0.5 }}
          >
            {gearLabel(gear)}
          </span>
        </div>
      </div>
    </div>
  );
}

function RpmBar({
  rpm,
  rpmMax,
  pct,
  redlinePct,
}: {
  rpm: number;
  rpmMax: number;
  pct: number;
  redlinePct: number;
}) {
  const overRedline = pct >= redlinePct;
  return (
    <div className="flex flex-col gap-2 rounded-2xl bg-surface p-5 shadow-surface">
      <div className="flex items-baseline justify-between">
        <span className="text-xs font-medium tracking-wider text-muted uppercase">RPM</span>
        <div className="flex items-baseline gap-1.5">
          <span
            className={
              overRedline
                ? "animate-pulse text-xl font-semibold text-danger tabular-nums"
                : "text-xl font-semibold text-foreground tabular-nums"
            }
          >
            {formatCount(Math.round(rpm))}
          </span>
          {rpmMax > 0 && (
            <span className="text-xs text-muted tabular-nums">
              / {formatCount(Math.round(rpmMax))}
            </span>
          )}
        </div>
      </div>
      <div className="relative h-2.5 overflow-hidden rounded-full bg-surface-secondary">
        <div
          className={
            overRedline
              ? "h-full animate-pulse rounded-full transition-[width] duration-100"
              : "h-full rounded-full transition-[width] duration-100"
          }
          style={{
            width: `${pct * 100}%`,
            background: overRedline ? "var(--danger)" : "var(--accent)",
            boxShadow: overRedline ? "0 0 12px var(--danger)" : undefined,
          }}
        />
        <span
          aria-hidden
          className="absolute top-0 h-full w-px bg-warning/60"
          style={{ left: `${redlinePct * 100}%` }}
        />
      </div>
    </div>
  );
}

function InputBar({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
  tone: "success" | "danger";
}) {
  const pct = Math.max(0, Math.min(1, value));
  return (
    <div className="flex flex-col gap-2 rounded-2xl bg-surface p-5 shadow-surface">
      <div className="flex items-baseline justify-between">
        <span className="text-xs font-medium tracking-wider text-muted uppercase">{label}</span>
        <span className="text-base font-semibold text-foreground tabular-nums">
          {Math.round(pct * 100)}%
        </span>
      </div>
      <div className="relative h-2 overflow-hidden rounded-full bg-surface-secondary">
        <div
          className="h-full rounded-full transition-[width] duration-100"
          style={{
            width: `${pct * 100}%`,
            background: tone === "danger" ? "var(--danger)" : "var(--success)",
          }}
        />
      </div>
    </div>
  );
}

function GForcePanel({ latG, longG }: { latG: number; longG: number }) {
  return (
    <div className="flex flex-col gap-3 rounded-2xl bg-surface p-5 shadow-surface">
      <span className="text-xs font-medium tracking-wider text-muted uppercase">G-force</span>
      <div className="grid grid-cols-2 gap-4">
        <GMetric label="Lateral" value={latG} />
        <GMetric label="Longitudinal" value={longG} />
      </div>
    </div>
  );
}

function GMetric({ label, value }: { label: string; value: number }) {
  const sign = value >= 0 ? "+" : "−";
  return (
    <div className="flex flex-col gap-1">
      <span className="text-xs text-muted">{label}</span>
      <div className="flex items-baseline gap-1">
        <span className="text-3xl leading-none font-semibold text-foreground tabular-nums">
          {sign}
          {Math.abs(value).toFixed(2)}
        </span>
        <span className="text-xs text-muted">G</span>
      </div>
    </div>
  );
}

export function LiveViewToggle({ active }: { active: "hud" | "cluster" }) {
  const base = "rounded-lg px-3 py-1 text-xs font-medium";
  return (
    <div className="flex gap-1 rounded-xl bg-surface p-1 shadow-surface">
      <Link
        to="/live"
        className={`${base} ${active === "hud" ? "bg-accent-soft text-foreground" : "text-muted"}`}
      >
        HUD
      </Link>
      <Link
        to="/live/cluster"
        className={`${base} ${active === "cluster" ? "bg-accent-soft text-foreground" : "text-muted"}`}
      >
        Cluster
      </Link>
    </div>
  );
}

function MetaPanel({ tick }: { tick: TickFrame }) {
  const rows: Array<[string, string]> = [
    ["Race state", tick.race ? "on" : "off"],
    ["Lap", formatCount(tick.lap ?? 0)],
    ["Car ord", tick.co !== 0 ? formatCount(tick.co) : "—"],
    ["Car class", tick.cc !== 0 ? formatCount(tick.cc) : "—"],
    ["Car PI", tick.cpi !== 0 ? formatCount(tick.cpi) : "—"],
    ["Cylinders", tick.ncy !== 0 ? formatCount(tick.ncy) : "—"],
  ];
  return (
    <div className="flex flex-col gap-3 rounded-2xl bg-surface p-5 shadow-surface">
      <span className="text-xs font-medium tracking-wider text-muted uppercase">Vehicle</span>
      <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
        {rows.map(([k, v]) => (
          <div key={k} className="flex items-baseline justify-between gap-2">
            <dt className="text-muted">{k}</dt>
            <dd className="font-medium text-foreground tabular-nums">{v}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}
