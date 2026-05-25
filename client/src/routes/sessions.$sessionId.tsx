/* Hallmark · component: session-detail · genre: dashboard · theme: Glass */
import { EmptyState } from "@heroui-pro/react";
import { Icon } from "@iconify/react";
import { Link, createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/sessions/$sessionId")({
  component: SessionDetailRoute,
});

// TODO: useQuery against GET /api/v1/sessions/{id}; render session metadata +
// stint list with links to /stints/{stintId}.
function SessionDetailRoute() {
  const { sessionId } = Route.useParams();

  return (
    <section className="flex flex-col gap-8">
      <nav aria-label="Breadcrumb" className="text-sm text-muted">
        <Link to="/sessions" className="no-underline text-muted hover:text-foreground">
          Sessions
        </Link>
        <span aria-hidden className="px-2">
          /
        </span>
        <span className="font-mono text-foreground">{sessionId}</span>
      </nav>

      <header className="flex flex-col gap-1">
        <span className="text-xs font-medium uppercase tracking-wider text-muted">
          Session
        </span>
        <h1 className="font-mono text-2xl font-semibold tracking-tight text-foreground">
          {sessionId}
        </h1>
      </header>

      <div className="rounded-2xl bg-surface shadow-surface">
        <EmptyState>
          <EmptyState.Header>
            <EmptyState.Media variant="icon">
              <Icon icon="lucide:list-tree" />
            </EmptyState.Media>
            <EmptyState.Title>Stint list pending</EmptyState.Title>
            <EmptyState.Description>
              Session detail will list its stints with type, car, distance, and best lap.
            </EmptyState.Description>
          </EmptyState.Header>
        </EmptyState>
      </div>
    </section>
  );
}
