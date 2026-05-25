/* Hallmark · component: sessions-index · genre: dashboard · theme: Glass */
import { Button, Chip, Skeleton } from "@heroui/react";
import { EmptyState } from "@heroui-pro/react";
import { useQuery } from "@tanstack/react-query";
import { Icon } from "@iconify/react";
import { Link, createFileRoute } from "@tanstack/react-router";
import { formatCount, formatDateTime, formatDurationNS } from "~/utils/format";
import { sessionsListQuery } from "~/utils/queries";
import type { Session } from "~/utils/schemas";

export const Route = createFileRoute("/sessions/")({
  // prefetchQuery (vs ensureQueryData) does NOT throw on failure — the
  // in-component `isError` branch handles the API down case with a friendly
  // UI rather than tripping the root error boundary.
  loader: ({ context }) => context.queryClient.prefetchQuery(sessionsListQuery()),
  component: SessionsIndexRoute,
});

function SessionsIndexRoute() {
  const { data, isLoading, isError } = useQuery(sessionsListQuery());

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
        {data && (
          <span className="text-sm tabular-nums text-muted">
            {formatCount(data.total)} captured
          </span>
        )}
      </header>

      {isLoading && <SessionListSkeleton />}
      {isError && <SessionsError />}
      {data &&
        (data.sessions.length === 0 ? (
          <EmptySessions />
        ) : (
          <SessionList sessions={data.sessions} />
        ))}
    </section>
  );
}

function SessionList({ sessions }: { sessions: Session[] }) {
  return (
    <ul className="flex flex-col gap-2" role="list">
      {sessions.map((s) => (
        <li key={s.id}>
          <SessionRow session={s} />
        </li>
      ))}
    </ul>
  );
}

function SessionRow({ session }: { session: Session }) {
  const inProgress = session.ended_at_ns === null;
  return (
    <Link
      to="/sessions/$sessionId"
      params={{ sessionId: session.id }}
      className="group flex items-center gap-4 rounded-2xl bg-surface px-5 py-4 no-underline text-foreground shadow-surface transition-colors hover:bg-surface-hover"
    >
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="font-mono text-sm text-foreground truncate">{session.id}</span>
        <span className="text-xs text-muted">{formatDateTime(session.started_at_ns)}</span>
      </div>

      <div className="hidden items-center gap-6 sm:flex">
        <Metric label="Stints" value={formatCount(session.stint_count)} />
        <Metric label="Duration" value={formatDurationNS(session.started_at_ns, session.ended_at_ns)} />
      </div>

      <div className="flex items-center gap-2">
        {inProgress && (
          <Chip size="sm" variant="soft" color="success">
            Live
          </Chip>
        )}
        {session.pinned && (
          <Chip size="sm" variant="soft" color="warning">
            Pinned
          </Chip>
        )}
        {session.downsampled && (
          <Chip size="sm" variant="soft">
            Downsampled
          </Chip>
        )}
      </div>

      <Icon
        icon="lucide:chevron-right"
        className="size-4 text-muted transition-transform group-hover:translate-x-0.5"
      />
    </Link>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col items-end gap-0.5">
      <span className="text-sm font-medium tabular-nums text-foreground">{value}</span>
      <span className="text-xs text-muted">{label}</span>
    </div>
  );
}

function SessionListSkeleton() {
  return (
    <div className="flex flex-col gap-2" aria-busy="true" aria-label="Loading sessions">
      {Array.from({ length: 3 }).map((_, i) => (
        <Skeleton key={i} className="h-16 w-full rounded-2xl" />
      ))}
    </div>
  );
}

function EmptySessions() {
  return (
    <div className="rounded-2xl bg-surface shadow-surface">
      <EmptyState size="lg">
        <EmptyState.Header>
          <EmptyState.Media variant="icon">
            <Icon icon="lucide:layers-3" />
          </EmptyState.Media>
          <EmptyState.Title>No sessions yet</EmptyState.Title>
          <EmptyState.Description>
            Each game launch becomes a session. Start driving and stints will appear here
            grouped by car, type, and lap.
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
  );
}

function SessionsError() {
  return (
    <div className="rounded-2xl bg-surface p-8 text-center shadow-surface">
      <Icon icon="lucide:circle-alert" className="mx-auto size-6 text-danger" />
      <h2 className="mt-3 text-base font-semibold text-foreground">
        Couldn't load sessions
      </h2>
      <p className="mt-1 text-sm text-muted">
        Check the server is running and the proxy / API is reachable.
      </p>
    </div>
  );
}
