import { EmptyState } from "@heroui-pro/react";
/* Hallmark · component: session-detail · genre: dashboard · theme: Glass */
import { Chip, Skeleton } from "@heroui/react";
import { Icon } from "@iconify/react";
import { useQuery } from "@tanstack/react-query";
import { Link, createFileRoute } from "@tanstack/react-router";
import { DeleteSessionButton, DownsampleButton, PinToggle } from "~/components/SessionActions";
import { formatCount, formatDateTime, formatDurationNS } from "~/utils/format";
import { sessionQuery } from "~/utils/queries";
import type { StintListRow } from "~/utils/schemas";

export const Route = createFileRoute("/sessions/$sessionId")({
  component: SessionDetailRoute,
  loader: ({ context, params }) =>
    context.queryClient.prefetchQuery(sessionQuery(params.sessionId)),
});

function SessionDetailRoute() {
  const { sessionId } = Route.useParams();
  const { data, isLoading, isError } = useQuery(sessionQuery(sessionId));

  return (
    <section className="flex flex-col gap-8">
      <nav aria-label="Breadcrumb" className="text-sm text-muted">
        <Link to="/sessions" className="text-muted no-underline hover:text-foreground">
          Sessions
        </Link>
        <span aria-hidden className="px-2">
          /
        </span>
        <span className="font-mono text-foreground">{sessionId}</span>
      </nav>

      {isLoading && <DetailSkeleton />}
      {isError && <DetailError sessionId={sessionId} />}
      {data && (
        <>
          <SessionHeader
            sessionId={sessionId}
            startedAtNS={data.started_at_ns}
            endedAtNS={data.ended_at_ns}
            pinned={data.pinned}
            downsampled={data.downsampled}
          />
          <StintsSection stints={data.stints} />
        </>
      )}
    </section>
  );
}

function SessionHeader({
  sessionId,
  startedAtNS,
  endedAtNS,
  pinned,
  downsampled,
}: {
  sessionId: string;
  startedAtNS: number;
  endedAtNS: number | null;
  pinned: boolean;
  downsampled: boolean;
}) {
  const inProgress = endedAtNS === null;
  return (
    <header className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
        <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
          <span className="text-xs font-medium tracking-wider text-muted uppercase">Session</span>
          {inProgress && (
            <Chip size="sm" variant="soft" color="success">
              Live
            </Chip>
          )}
          {downsampled && (
            <Chip size="sm" variant="soft">
              Downsampled
            </Chip>
          )}
        </div>
        <div className="flex items-center gap-2">
          <DownsampleButton id={sessionId} downsampled={downsampled} />
          <DeleteSessionButton id={sessionId} disabled={inProgress} />
          <PinToggle id={sessionId} pinned={pinned} />
        </div>
      </div>
      <h1 className="font-mono text-2xl font-semibold tracking-tight text-foreground">
        {sessionId}
      </h1>
      <dl className="flex flex-wrap items-center gap-x-6 gap-y-1 text-sm">
        <Stat label="Started" value={formatDateTime(startedAtNS)} />
        <Stat label="Duration" value={formatDurationNS(startedAtNS, endedAtNS)} />
      </dl>
    </header>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline gap-2">
      <dt className="text-muted">{label}</dt>
      <dd className="font-medium text-foreground tabular-nums">{value}</dd>
    </div>
  );
}

function StintsSection({ stints }: { stints: StintListRow[] }) {
  if (stints.length === 0) {
    return (
      <div className="rounded-2xl bg-surface shadow-surface">
        <EmptyState>
          <EmptyState.Header>
            <EmptyState.Media variant="icon">
              <Icon icon="lucide:list-tree" />
            </EmptyState.Media>
            <EmptyState.Title>No stints in this session</EmptyState.Title>
            <EmptyState.Description>
              Sub-2-second stints are discarded; a session with only menu time will appear empty.
            </EmptyState.Description>
          </EmptyState.Header>
        </EmptyState>
      </div>
    );
  }
  return (
    <div className="flex flex-col gap-3">
      <h2 className="text-xs font-medium tracking-wider text-muted uppercase">Stints</h2>
      <ul className="flex flex-col gap-2">
        {stints.map((s) => (
          <li key={s.id}>
            <StintRow stint={s} />
          </li>
        ))}
      </ul>
    </div>
  );
}

function StintRow({ stint }: { stint: StintListRow }) {
  const inProgress = stint.ended_at_ns === null;
  return (
    <Link
      to="/stints/$stintId"
      params={{ stintId: stint.id }}
      className="group flex items-center gap-4 rounded-2xl bg-surface px-5 py-4 text-foreground no-underline shadow-surface transition-colors hover:bg-surface-hover"
    >
      <div
        aria-hidden
        className="grid size-9 shrink-0 place-items-center rounded-xl bg-accent-soft font-mono text-sm font-medium text-accent-soft-foreground tabular-nums"
      >
        {stint.ordinal.toString().padStart(2, "0")}
      </div>
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <div className="flex items-center gap-2">
          {stint.stint_type ? (
            <Chip size="sm" variant="soft">
              {stint.stint_type}
            </Chip>
          ) : (
            <Chip size="sm" variant="soft">
              pending
            </Chip>
          )}
          {stint.car_ordinal !== null && (
            <span className="text-xs text-muted tabular-nums">car #{stint.car_ordinal}</span>
          )}
          {inProgress && (
            <Chip size="sm" variant="soft" color="success">
              Live
            </Chip>
          )}
        </div>
        <span className="text-xs text-muted">
          {formatDateTime(stint.started_at_ns)} ·{" "}
          {formatDurationNS(stint.started_at_ns, stint.ended_at_ns)}
        </span>
      </div>
      <div className="flex flex-col items-end gap-0.5">
        <span className="text-sm font-medium text-foreground tabular-nums">
          {formatCount(stint.tick_count)}
        </span>
        <span className="text-xs text-muted">ticks</span>
      </div>
      <Icon
        icon="lucide:chevron-right"
        className="size-4 text-muted transition-transform group-hover:translate-x-0.5"
      />
    </Link>
  );
}

function DetailSkeleton() {
  return (
    <div className="flex flex-col gap-8" aria-busy="true">
      <Skeleton className="h-24 w-full max-w-md rounded-2xl" />
      <div className="flex flex-col gap-2">
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} className="h-16 w-full rounded-2xl" />
        ))}
      </div>
    </div>
  );
}

function DetailError({ sessionId }: { sessionId: string }) {
  return (
    <div className="rounded-2xl bg-surface p-8 text-center shadow-surface">
      <Icon icon="lucide:circle-alert" className="mx-auto size-6 text-danger" />
      <h2 className="mt-3 text-base font-semibold text-foreground">Couldn't load session</h2>
      <p className="mt-1 text-sm text-muted">
        <span className="font-mono">{sessionId}</span> may not exist, or the server is unreachable.
      </p>
    </div>
  );
}
