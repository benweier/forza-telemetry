/* Hallmark · component: live-preview · genre: dashboard · theme: Glass */
import { Button } from "@heroui/react";
import { Icon } from "@iconify/react";
import { Group, Panel, Separator } from "react-resizable-panels";
import { GamePreview } from "~/components/GamePreview";
import { MAX_OFFSET_MS, useLiveStore } from "~/utils/live-store";
import type { ReactNode } from "react";

/** Header control that toggles game-preview mode on/off (persisted). */
export function PreviewToggle() {
  const enabled = useLiveStore((s) => s.previewEnabled);
  const setEnabled = useLiveStore((s) => s.setPreviewEnabled);
  return (
    <Button
      size="sm"
      variant={enabled ? "primary" : "outline"}
      aria-pressed={enabled}
      onPress={() => setEnabled(!enabled)}
    >
      <Icon icon="lucide:monitor-play" className="mr-1.5 size-4" />
      Game preview
    </Button>
  );
}

/**
 * Wraps a live view. When preview is off, renders the dashboard full-width
 * (unchanged). When on, splits into a resizable [ video + settings | dashboard ]
 * — the dashboard runs delayed (via the store's display selector) to match the
 * video latency. Used by both the HUD and Instrument routes.
 */
export function PreviewShell({ children }: { children: ReactNode }) {
  const enabled = useLiveStore((s) => s.previewEnabled);
  if (!enabled) return <>{children}</>;
  return (
    <Group orientation="horizontal" className="items-stretch">
      <Panel defaultSize="45%" minSize="25%" className="pr-3">
        <PreviewPane />
      </Panel>
      <Separator className="mx-0.5 w-1 rounded-full bg-surface-secondary transition-colors hover:bg-accent-soft" />
      <Panel minSize="30%" className="overflow-auto pl-3">
        {children}
      </Panel>
    </Group>
  );
}

function PreviewPane() {
  const whepUrl = useLiveStore((s) => s.whepUrl);
  const setWhepUrl = useLiveStore((s) => s.setWhepUrl);
  const offsetMs = useLiveStore((s) => s.offsetMs);
  const setOffsetMs = useLiveStore((s) => s.setOffsetMs);

  return (
    <div className="flex flex-col gap-3">
      <GamePreview url={whepUrl} />
      <div className="flex flex-col gap-4 rounded-2xl bg-surface p-4 shadow-surface">
        <label className="flex flex-col gap-1.5">
          <span className="text-xs font-medium tracking-wider text-muted uppercase">WHEP URL</span>
          <input
            type="text"
            value={whepUrl}
            onChange={(e) => setWhepUrl(e.target.value)}
            placeholder="http://192.168.1.10:8889/mystream/whep"
            spellCheck={false}
            autoComplete="off"
            className="rounded-xl bg-surface-secondary px-3 py-2 text-sm text-foreground outline-none focus:ring-2 focus:ring-[var(--accent)]"
          />
        </label>
        <label className="flex flex-col gap-1.5">
          <div className="flex items-baseline justify-between">
            <span className="text-xs font-medium tracking-wider text-muted uppercase">
              Sync offset
            </span>
            <span className="text-sm font-semibold text-foreground tabular-nums">
              {offsetMs} ms
            </span>
          </div>
          <input
            type="range"
            min={0}
            max={MAX_OFFSET_MS}
            step={10}
            value={offsetMs}
            onChange={(e) => setOffsetMs(Number(e.target.value))}
            style={{ accentColor: "var(--accent)" }}
          />
          <span className="text-xs text-pretty text-muted">
            Holds telemetry back to match the video. Nudge until the speedo lines up.
          </span>
        </label>
      </div>
    </div>
  );
}
