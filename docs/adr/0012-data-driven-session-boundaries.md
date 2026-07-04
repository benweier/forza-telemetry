# Sessions are bound by data, not by process lifetime

A **Session** used to be one run of the `forza-telemetry serve` process: the
row was created at server start and closed at shutdown. That contradicted
CONTEXT.md's definition ("one continuous arrival of Data Out packets"), and
two whole bug classes were symptoms of it — empty sessions (server up, game
never launched) needed a startup sweep, and crashed sessions read as
"recording" forever until a startup backfill was added. A server left running
for a week produced one giant Session spanning every drive in it.

Sessions are now created and closed by the data itself. The Writer opens a
session row on the **first tick** (its ID is the first tick's arrival time)
and closes it on either boundary:

- **Silence**: no packets for ≥ 1 hour (`sessionGap`). The session's
  `ended_at_ns` is its last tick's arrival — not wall-clock at split time.
- **Game relaunch**: `GameTSMillis` (the game's own uptime clock) jumping
  backwards by more than 60 s. Out-of-order UDP jitters it backwards by
  fractions of a second; the tolerance absorbs that. A uint32 wrap at ~49.7
  days of continuous game uptime would also read as a relaunch — accepted as
  implausible (docs/data-needed.md).

A session whose stints were all discarded is deleted at close, so a
long-running server accumulates no empty rows between drives. Shutdown closes
the open session the same way.

## Considered Options

- **Keep process-lifetime sessions** — rejected: the model mismatch had
  already cost two band-aids (empty-session sweep, crashed-session backfill),
  and "what did I drive Tuesday?" was buried inside a row named after a
  server restart.
- **Game-launch-only boundaries (no silence gap)** — rejected: a game left
  paused overnight would fuse unrelated drives; silence is the stronger
  everyday signal and the relaunch check catches the no-gap case.

## Consequences

- No session exists until data arrives; the startup log no longer reports a
  session ID.
- The startup sweeps/backfill remain for databases written by older builds
  and for crash recovery; they are no longer load-bearing for normal
  operation.
- REST/WS and the client are untouched — sessions are the same rows with the
  same shape, just with meaningful boundaries.
