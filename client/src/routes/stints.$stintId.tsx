/* Hallmark · component: stint-detail · genre: dashboard · theme: Glass */
import { EmptyState } from "@heroui-pro/react";
import { Icon } from "@iconify/react";
import { Link, createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/stints/$stintId")({
  component: StintDetailRoute,
});

// TODO: useQuery for GET /api/v1/stints/{id} (detail + summary), then sub-
// resources: laps, hot-spots, corners, preview, and on-demand tick windows.
// Render: time-series chart, mini-map, hot-spot pins, lap table.
function StintDetailRoute() {
  const { stintId } = Route.useParams();

  return (
    <section className="flex flex-col gap-8">
      <nav aria-label="Breadcrumb" className="text-sm text-muted">
        <Link to="/sessions" className="no-underline text-muted hover:text-foreground">
          Sessions
        </Link>
        <span aria-hidden className="px-2">
          /
        </span>
        <span className="font-mono text-foreground">{stintId}</span>
      </nav>

      <header className="flex items-baseline justify-between gap-4">
        <div className="flex flex-col gap-1">
          <span className="text-xs font-medium uppercase tracking-wider text-muted">
            Stint
          </span>
          <h1 className="font-mono text-2xl font-semibold tracking-tight text-foreground">
            {stintId}
          </h1>
        </div>
      </header>

      <div className="grid gap-4 lg:grid-cols-[1fr_minmax(0,18rem)]">
        <div className="rounded-2xl bg-surface shadow-surface">
          <EmptyState>
            <EmptyState.Header>
              <EmptyState.Media variant="icon">
                <Icon icon="gravity-ui:chart-line" />
              </EmptyState.Media>
              <EmptyState.Title>Charts pending</EmptyState.Title>
              <EmptyState.Description>
                Time-series channels (speed, RPM, lat/long G, throttle, brake) and the
                track-path mini-map will render here.
              </EmptyState.Description>
            </EmptyState.Header>
          </EmptyState>
        </div>
        <div className="flex flex-col gap-4">
          <div className="rounded-2xl bg-surface p-5 shadow-surface">
            <span className="text-xs font-medium uppercase tracking-wider text-muted">
              Hot-spots
            </span>
            <p className="pt-2 text-sm text-muted">
              Auto-detected peaks (lateral G, brake, top speed) appear here.
            </p>
          </div>
          <div className="rounded-2xl bg-surface p-5 shadow-surface">
            <span className="text-xs font-medium uppercase tracking-wider text-muted">
              Corners
            </span>
            <p className="pt-2 text-sm text-muted">
              Curvature-derived corners with apex tick and direction.
            </p>
          </div>
        </div>
      </div>
    </section>
  );
}
