/* Hallmark · component: live-hud · genre: dashboard · theme: Glass */
import { Icon } from "@iconify/react";
import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/live")({
  component: LiveRoute,
});

// TODO: subscribe to `~/utils/ws` LiveSocket, render gauges from
// `~/utils/live-store` (speed, RPM, gear, lat/long G, throttle/brake).
function LiveRoute() {
  return (
    <section className="flex flex-col gap-8">
      <header className="flex items-baseline justify-between gap-4">
        <div className="flex flex-col gap-1">
          <span className="text-xs font-medium uppercase tracking-wider text-muted">
            Realtime
          </span>
          <h1 className="text-3xl font-semibold tracking-tight text-foreground">
            Live HUD
          </h1>
        </div>
        <span className="inline-flex items-center gap-2 rounded-full bg-surface px-3 py-1 text-xs text-muted shadow-surface">
          <span aria-hidden className="size-1.5 rounded-full bg-muted" />
          Idle
        </span>
      </header>

      <div className="rounded-2xl bg-surface p-12 shadow-surface">
        <div className="mx-auto flex max-w-md flex-col items-center gap-4 text-center">
          <span className="grid size-12 place-items-center rounded-2xl bg-accent-soft text-accent-soft-foreground">
            <Icon icon="lucide:radio-tower" className="size-6" />
          </span>
          <h2 className="text-xl font-semibold tracking-tight text-foreground">
            Waiting for telemetry
          </h2>
          <p className="text-sm text-muted text-pretty">
            Open Forza Horizon, enable Data Out under HUD settings, point the IP at this
            host on UDP 7100. Frames will appear here as soon as the game starts streaming.
          </p>
        </div>
      </div>
    </section>
  );
}
