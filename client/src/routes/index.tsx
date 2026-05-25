/* Hallmark · component: home-landing · genre: dashboard · theme: Glass */
import { Button } from "@heroui/react";
import { Icon } from "@iconify/react";
import { Link, createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/")({
  component: HomeRoute,
});

function HomeRoute() {
  return (
    <section className="flex flex-col gap-10">
      <header className="flex flex-col gap-3">
        <span className="text-xs font-medium uppercase tracking-wider text-muted">
          Local LAN telemetry
        </span>
        <h1 className="text-4xl font-semibold leading-tight tracking-tight text-foreground">
          Forza Telemetry
        </h1>
        <p className="max-w-prose text-base text-muted">
          Ingests Forza Horizon Data Out over UDP, splits the stream into stints,
          and surfaces live and historical analytics for solo race analysis.
        </p>
      </header>

      <div className="grid gap-4 sm:grid-cols-2">
        <Link
          to="/live"
          className="group flex flex-col gap-3 rounded-2xl bg-surface p-6 no-underline text-foreground shadow-surface transition-colors hover:bg-surface-hover"
        >
          <span className="grid size-9 place-items-center rounded-xl bg-accent-soft text-accent-soft-foreground">
            <Icon icon="gravity-ui:square-activity" className="size-5" />
          </span>
          <span className="text-lg font-semibold tracking-tight">Live HUD</span>
          <span className="text-sm text-muted">
            Realtime WebSocket view — speed, RPM, lateral G straight off the wire.
          </span>
        </Link>

        <Link
          to="/sessions"
          className="group flex flex-col gap-3 rounded-2xl bg-surface p-6 no-underline text-foreground shadow-surface transition-colors hover:bg-surface-hover"
        >
          <span className="grid size-9 place-items-center rounded-xl bg-accent-soft text-accent-soft-foreground">
            <Icon icon="gravity-ui:layers-3" className="size-5" />
          </span>
          <span className="text-lg font-semibold tracking-tight">Sessions</span>
          <span className="text-sm text-muted">
            Browse captured stints with hot-spots, corners, and per-lap summaries.
          </span>
        </Link>
      </div>

      <div className="flex items-center gap-3 pt-4">
        <Button variant="primary" onPress={() => {}}>
          <Link to="/live" className="no-underline text-inherit">
            Open Live HUD
          </Link>
        </Button>
        <Button variant="outline" onPress={() => {}}>
          <Link to="/sessions" className="no-underline text-inherit">
            Browse Sessions
          </Link>
        </Button>
      </div>
    </section>
  );
}
