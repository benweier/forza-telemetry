import { createFileRoute } from "@tanstack/react-router";
import { ClusterCanvas } from "~/cluster/ClusterCanvas";
import { LiveViewToggle, StatusPill, useLiveSocket, useLiveStatus } from "./live";

export const Route = createFileRoute("/live_/cluster")({ component: ClusterRoute });

function ClusterRoute() {
  useLiveSocket();
  const { connected, fresh } = useLiveStatus();

  return (
    <section className="flex flex-col gap-8">
      <header className="flex items-baseline justify-between gap-4">
        <div className="flex flex-col gap-1">
          <span className="text-xs font-medium tracking-wider text-muted uppercase">Realtime</span>
          <h1 className="text-3xl font-semibold tracking-tight text-foreground">Cluster</h1>
        </div>
        <div className="flex items-center gap-3">
          <LiveViewToggle active="cluster" />
          <StatusPill connected={connected} fresh={fresh} />
        </div>
      </header>
      <ClusterCanvas />
    </section>
  );
}
