import { createFileRoute } from "@tanstack/react-router";
import { ClusterCanvas } from "~/cluster/ClusterCanvas";
import { LiveViewToggle } from "./live";

export const Route = createFileRoute("/live_/cluster")({ component: ClusterRoute });

function ClusterRoute() {
  return (
    <section className="flex flex-col gap-8">
      <header className="flex items-baseline justify-between gap-4">
        <div className="flex flex-col gap-1">
          <span className="text-xs font-medium tracking-wider text-muted uppercase">Realtime</span>
          <h1 className="text-3xl font-semibold tracking-tight text-foreground">Cluster</h1>
        </div>
        <LiveViewToggle active="cluster" />
      </header>
      <ClusterCanvas />
    </section>
  );
}
