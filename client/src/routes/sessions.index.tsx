/* Hallmark · component: sessions-index · genre: dashboard · theme: Glass */
import { Button } from "@heroui/react";
import { EmptyState } from "@heroui-pro/react";
import { Icon } from "@iconify/react";
import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/sessions/")({
  component: SessionsIndexRoute,
});

// TODO: useQuery against GET /api/v1/sessions; render list with stint counts,
// pin toggle, and link to /sessions/{id}.
function SessionsIndexRoute() {
  return (
    <section className="flex flex-col gap-8">
      <header className="flex items-baseline justify-between gap-4">
        <div className="flex flex-col gap-1">
          <span className="text-xs font-medium uppercase tracking-wider text-muted">
            History
          </span>
          <h1 className="text-3xl font-semibold tracking-tight text-foreground">
            Sessions
          </h1>
        </div>
        <span className="text-sm tabular-nums text-muted">0 captured</span>
      </header>

      <div className="rounded-2xl bg-surface shadow-surface">
        <EmptyState size="lg">
          <EmptyState.Header>
            <EmptyState.Media variant="icon">
              <Icon icon="lucide:layers-3" />
            </EmptyState.Media>
            <EmptyState.Title>No sessions yet</EmptyState.Title>
            <EmptyState.Description>
              Each game launch becomes a session. Start driving and stints will appear
              here grouped by car, type, and lap.
            </EmptyState.Description>
          </EmptyState.Header>
          <EmptyState.Content>
            <Button variant="outline" size="sm" onPress={() => {}}>
              <Icon icon="lucide:circle-help" className="mr-1.5 size-4" />
              Setup guide
            </Button>
          </EmptyState.Content>
        </EmptyState>
      </div>
    </section>
  );
}
