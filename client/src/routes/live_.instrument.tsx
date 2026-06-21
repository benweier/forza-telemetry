import { createFileRoute } from "@tanstack/react-router";
import { PreviewShell, PreviewToggle } from "~/components/LivePreview";
import { InstrumentCanvas } from "~/instrument/InstrumentCanvas";
import { LiveViewToggle, StatusPill, useLiveSocket, useLiveStatus } from "./live";

export const Route = createFileRoute("/live_/instrument")({ component: InstrumentRoute });

function InstrumentRoute() {
  useLiveSocket();
  const { connected, fresh } = useLiveStatus();

  return (
    <section className="flex flex-col gap-8">
      <header className="flex items-baseline justify-between gap-4">
        <div className="flex flex-col gap-1">
          <span className="text-xs font-medium tracking-wider text-muted uppercase">Realtime</span>
          <h1 className="text-3xl font-semibold tracking-tight text-foreground">Instrument</h1>
        </div>
        <div className="flex items-center gap-3">
          <PreviewToggle />
          <LiveViewToggle active="instrument" />
          <StatusPill connected={connected} fresh={fresh} />
        </div>
      </header>
      <PreviewShell>
        <InstrumentCanvas />
      </PreviewShell>
    </section>
  );
}
