import { EmptyState } from "@heroui-pro/react";
/* Hallmark · component: stint-detail · genre: dashboard · theme: Glass */
import { Chip, Skeleton } from "@heroui/react";
import { Icon } from "@iconify/react";
import { useQuery } from "@tanstack/react-query";
import { Link, createFileRoute } from "@tanstack/react-router";
import { TickPreviewChart } from "~/components/TickPreviewChart";
import { TrackPathMap } from "~/components/TrackPathMap";
import { formatCount, formatDateTime, formatDurationNS } from "~/utils/format";
import {
  hotSpotsQuery,
  lapsQuery,
  pathQuery,
  previewQuery,
  stintQuery,
  straightsQuery,
  turnsQuery,
} from "~/utils/queries";
import type { HotSpot, Lap, StintDetail, StintSummary, Turn } from "~/utils/schemas";

export const Route = createFileRoute("/stints/$stintId")({
  component: StintDetailRoute,
  loader: ({ context, params }) => {
    // Prefetch the primary record + key sub-resources in parallel; failures
    // fall through to in-component error UI, not the root boundary.
    const id = params.stintId;
    return Promise.all([
      context.queryClient.prefetchQuery(stintQuery(id)),
      context.queryClient.prefetchQuery(previewQuery(id)),
      context.queryClient.prefetchQuery(pathQuery(id)),
      context.queryClient.prefetchQuery(hotSpotsQuery(id)),
      context.queryClient.prefetchQuery(turnsQuery(id)),
      context.queryClient.prefetchQuery(straightsQuery(id)),
      context.queryClient.prefetchQuery(lapsQuery(id)),
    ]);
  },
});

function StintDetailRoute() {
  const { stintId } = Route.useParams();
  const stint = useQuery(stintQuery(stintId));
  const preview = useQuery(previewQuery(stintId));
  const path = useQuery(pathQuery(stintId));
  const hotSpots = useQuery(hotSpotsQuery(stintId));
  const turns = useQuery(turnsQuery(stintId));
  const straights = useQuery(straightsQuery(stintId));
  const laps = useQuery(lapsQuery(stintId));

  return (
    <section className="flex flex-col gap-8">
      <Breadcrumb stintId={stintId} sessionId={stint.data?.session_id} />

      {stint.isLoading && <HeaderSkeleton />}
      {stint.isError && <DetailError stintId={stintId} />}
      {stint.data && <Header stint={stint.data} />}

      {stint.data?.summary && <SummaryStats summary={stint.data.summary} />}

      <div className="grid gap-4 lg:grid-cols-[1fr_minmax(0,20rem)]">
        <div className="flex flex-col gap-4">
          <SectionHeading>Track path</SectionHeading>
          {path.isLoading && <Skeleton className="aspect-[16/9] w-full rounded-2xl" />}
          {path.data && (
            <TrackPathMap
              path={path.data}
              hotSpots={hotSpots.data?.hot_spots}
              turns={turns.data?.turns}
              straights={straights.data?.straights}
            />
          )}

          <SectionHeading>Preview</SectionHeading>
          {preview.isLoading && <Skeleton className="h-80 w-full rounded-2xl" />}
          {preview.data && <TickPreviewChart samples={preview.data.samples} />}
        </div>

        <aside className="flex flex-col gap-4">
          <HotSpotsCard query={hotSpots} />
          <TurnsCard query={turns} />
          <LapsCard query={laps} />
        </aside>
      </div>
    </section>
  );
}

// ---------- Header + stats ----------

function Breadcrumb({ stintId, sessionId }: { stintId: string; sessionId?: string }) {
  return (
    <nav aria-label="Breadcrumb" className="text-sm text-muted">
      <Link to="/sessions" className="text-muted no-underline hover:text-foreground">
        Sessions
      </Link>
      <span aria-hidden className="px-2">
        /
      </span>
      {sessionId ? (
        <Link
          to="/sessions/$sessionId"
          params={{ sessionId }}
          className="font-mono text-muted no-underline hover:text-foreground"
        >
          {sessionId}
        </Link>
      ) : (
        <span className="font-mono text-muted">—</span>
      )}
      <span aria-hidden className="px-2">
        /
      </span>
      <span className="font-mono text-foreground">{stintId}</span>
    </nav>
  );
}

function Header({ stint }: { stint: StintDetail }) {
  const inProgress = stint.ended_at_ns === null;
  return (
    <header className="flex flex-col gap-3">
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <span className="text-xs font-medium tracking-wider text-muted uppercase">Stint</span>
        {stint.stint_type && (
          <Chip size="sm" variant="soft">
            {stint.stint_type}
          </Chip>
        )}
        {stint.car.ordinal !== null && stint.car.ordinal !== 0 && (
          <span className="text-xs text-muted tabular-nums">car #{stint.car.ordinal}</span>
        )}
        {inProgress && (
          <Chip size="sm" variant="soft" color="success">
            Live
          </Chip>
        )}
      </div>
      <h1 className="font-mono text-2xl font-semibold tracking-tight text-foreground">
        {stint.id}
      </h1>
      <dl className="flex flex-wrap items-center gap-x-6 gap-y-1 text-sm">
        <StatPair label="Started" value={formatDateTime(stint.started_at_ns)} />
        <StatPair
          label="Duration"
          value={formatDurationNS(stint.started_at_ns, stint.ended_at_ns)}
        />
        <StatPair label="Ticks" value={formatCount(stint.tick_count)} />
      </dl>
    </header>
  );
}

function StatPair({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline gap-2">
      <dt className="text-muted">{label}</dt>
      <dd className="font-medium text-foreground tabular-nums">{value}</dd>
    </div>
  );
}

function SummaryStats({ summary }: { summary: StintSummary }) {
  const cells: { label: string; value: string | null; sub?: string }[] = [
    {
      label: "Top speed",
      sub: "km/h",
      value: summary.top_speed_ms !== null ? `${(summary.top_speed_ms * 3.6).toFixed(0)}` : null,
    },
    {
      label: "Distance",
      sub: "km",
      value: summary.distance_m !== null ? `${(summary.distance_m / 1000).toFixed(2)}` : null,
    },
    {
      label: "Peak lat G",
      sub: "G",
      value: summary.peak_lateral_g !== null ? summary.peak_lateral_g.toFixed(2) : null,
    },
    {
      label: "Peak brake",
      sub: "%",
      value: summary.peak_brake_pct !== null ? `${Math.round(summary.peak_brake_pct * 100)}` : null,
    },
    {
      label: "Max RPM",
      value: summary.max_rpm !== null ? formatCount(Math.round(summary.max_rpm)) : null,
    },
    {
      label: "Gear shifts",
      value: summary.gear_shift_count !== null ? formatCount(summary.gear_shift_count) : null,
    },
  ];
  const shown = cells.filter((c) => c.value !== null);
  if (shown.length === 0) {
    return null;
  }

  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
      {shown.map((c) => (
        <div
          key={c.label}
          className="flex flex-col gap-1 rounded-2xl bg-surface px-5 py-4 shadow-surface"
        >
          <span className="text-xs text-muted">{c.label}</span>
          <div className="flex items-baseline gap-1.5">
            <span className="text-2xl leading-none font-semibold text-foreground tabular-nums">
              {c.value}
            </span>
            {c.sub && <span className="text-xs text-muted">{c.sub}</span>}
          </div>
        </div>
      ))}
    </div>
  );
}

function HeaderSkeleton() {
  return (
    <div className="flex flex-col gap-3" aria-busy="true">
      <Skeleton className="h-3 w-20 rounded-md" />
      <Skeleton className="h-8 w-80 rounded-md" />
      <Skeleton className="h-4 w-96 rounded-md" />
    </div>
  );
}

function DetailError({ stintId }: { stintId: string }) {
  return (
    <div className="rounded-2xl bg-surface p-8 text-center shadow-surface">
      <Icon icon="lucide:circle-alert" className="mx-auto size-6 text-danger" />
      <h2 className="mt-3 text-base font-semibold text-foreground">Couldn't load stint</h2>
      <p className="mt-1 text-sm text-muted">
        <span className="font-mono">{stintId}</span> may not exist, or the server is unreachable.
      </p>
    </div>
  );
}

function SectionHeading({ children }: { children: React.ReactNode }) {
  return <h2 className="text-xs font-medium tracking-wider text-muted uppercase">{children}</h2>;
}

// ---------- Side panel cards ----------

type QueryResult<T> = ReturnType<typeof useQuery<T>>;

function HotSpotsCard({ query }: { query: QueryResult<{ hot_spots: HotSpot[] }> }) {
  const hot = query.data?.hot_spots ?? [];
  return (
    <Card title="Hot-spots" icon="lucide:flame" count={hot.length}>
      {query.isLoading && <ListSkeleton rows={2} />}
      {!query.isLoading && hot.length === 0 && (
        <EmptyLine>No peaks above default thresholds.</EmptyLine>
      )}
      {hot.length > 0 && (
        <ul className="flex flex-col gap-2">
          {hot.map((h) => (
            <li
              key={h.id}
              className="flex items-center justify-between gap-3 rounded-xl bg-surface-secondary px-3 py-2"
            >
              <div className="flex min-w-0 flex-col gap-0.5">
                <span className="truncate text-sm font-medium text-foreground">{h.label}</span>
                <span className="text-xs text-muted">{hotSpotTypeLabel(h.type)}</span>
              </div>
              <span className="text-xs text-muted tabular-nums">
                {formatDurationNS(h.started_at_ns, h.ended_at_ns)}
              </span>
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}

function TurnsCard({ query }: { query: QueryResult<{ turns: Turn[] }> }) {
  const turns = query.data?.turns ?? [];
  return (
    <Card title="Turns" icon="lucide:rotate-3d" count={turns.length}>
      {query.isLoading && <ListSkeleton rows={2} />}
      {!query.isLoading && turns.length === 0 && (
        <EmptyLine>No turns detected (likely a freeroam or idle stint).</EmptyLine>
      )}
      {turns.length > 0 && (
        <ul className="flex flex-col gap-2">
          {turns.map((c) => (
            <li
              key={c.id}
              className="flex items-center justify-between gap-3 rounded-xl bg-surface-secondary px-3 py-2"
            >
              <div className="flex min-w-0 flex-col gap-0.5">
                <span className="text-sm font-medium text-foreground">
                  Turn {c.turn_index}
                  {c.shape && (
                    <span className="ml-1 text-muted">· {c.shape}</span>
                  )}
                </span>
                <span className="text-xs text-muted">
                  {c.direction} · Δθ {(c.peak_delta_theta * (180 / Math.PI)).toFixed(0)}°
                </span>
              </div>
              <span className="text-xs text-muted tabular-nums">
                κ {Math.abs(c.peak_curvature).toFixed(3)}
              </span>
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}

function LapsCard({ query }: { query: QueryResult<{ laps: Lap[] }> }) {
  // Lap 0 is the pre-race / out-lap chunk before LapNumber increments — skip
  // It in the summary panel since its lap_time_s is not a complete lap.
  const laps = (query.data?.laps ?? []).filter((l) => l.lap_number > 0);
  return (
    <Card title="Laps" icon="lucide:flag" count={laps.length}>
      {query.isLoading && <ListSkeleton rows={2} />}
      {!query.isLoading && laps.length === 0 && (
        <EmptyLine>No completed laps in this stint.</EmptyLine>
      )}
      {laps.length > 0 && (
        <ul className="flex flex-col gap-2">
          {laps.map((l) => (
            <li
              key={l.lap_number}
              className="flex items-center justify-between gap-3 rounded-xl bg-surface-secondary px-3 py-2"
            >
              <span className="text-sm font-medium text-foreground tabular-nums">
                Lap {l.lap_number}
              </span>
              <span className="text-sm font-medium text-foreground tabular-nums">
                {l.lap_time_s !== null ? `${l.lap_time_s.toFixed(2)}s` : "—"}
              </span>
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}

function Card({
  title,
  icon,
  count,
  children,
}: {
  title: string;
  icon: string;
  count: number;
  children: React.ReactNode;
}) {
  return (
    <section className="flex flex-col gap-3 rounded-2xl bg-surface p-5 shadow-surface">
      <header className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Icon icon={icon} className="size-4 text-muted" />
          <span className="text-xs font-medium tracking-wider text-muted uppercase">{title}</span>
        </div>
        <span className="text-xs text-muted tabular-nums">{count}</span>
      </header>
      {children}
    </section>
  );
}

function EmptyLine({ children }: { children: React.ReactNode }) {
  return <p className="text-xs text-muted">{children}</p>;
}

function ListSkeleton({ rows }: { rows: number }) {
  return (
    <div className="flex flex-col gap-2" aria-busy="true">
      {Array.from({ length: rows }).map((_, i) => (
        <Skeleton key={i} className="h-10 w-full rounded-xl" />
      ))}
    </div>
  );
}

function hotSpotTypeLabel(t: string) {
  switch (t) {
    case "peak_lateral_g": {
      return "Lateral G peak";
    }
    case "peak_brake": {
      return "Brake peak";
    }
    case "top_speed": {
      return "Top speed";
    }
    default: {
      return t;
    }
  }
}
