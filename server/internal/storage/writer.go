package storage

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/parquet-go/parquet-go"

	"github.com/benweier/forza-telemetry/server/internal/stream"
	"github.com/benweier/forza-telemetry/server/internal/tick"
)

// Writer owns the Session + Stint lifecycle: it reads ticks from a broker
// subscription and splits them into Stints on the rules from ADR 0013:
//
//   - a packet-arrival gap of >= gapThreshold
//   - an IsRaceOn flip (gameplay <-> menus/loading/pause)
//   - a change of Car (CarOrdinal)
//
// Each Stint is written to its own Parquet segment dir under
// parquet/hot/<session>/, with metadata rows maintained in DuckDB. Stints
// shorter than minDuration are discarded at close (files removed, row
// deleted).
type Writer struct {
	store        *Store
	logger       *slog.Logger
	gapThreshold time.Duration
	sessionGap   time.Duration
	minDuration  time.Duration
	minTicks     int64

	// Session state (ADR 0012): created lazily on the first tick, split on a
	// sessionGap silence or a GameTSMillis regression (game reboot). All
	// fields are touched only by the Run goroutine.
	sessionID     string
	sessionLastNS int64
	sessionLastTS uint32
	// rotateEvery bounds crash loss: the current Parquet segment is closed
	// (footer + fsync — durable) and a new one opened whenever the segment
	// spans this much tick time. A crash costs at most the open segment
	// (ADR 0011). Zero disables rotation.
	rotateEvery time.Duration

	// aggWG tracks in-flight async stint aggregations; shutdown waits on it
	// so the final stint's summaries land before the store closes.
	aggWG sync.WaitGroup

	cur *stintState
}

type stintState struct {
	id      string
	ordinal int
	// dir holds the stint's Parquet segment files (0001.parquet, …); path is
	// the glob over them, stored in stints.parquet_path — DuckDB read_parquet
	// accepts the glob directly, so readers never care about segmentation.
	dir         string
	path        string
	seg         int
	segStartNS  int64
	file        *os.File
	pq          *parquet.GenericWriter[parquetRow]
	tickCount   int64
	startedAtNS int64
	lastTickNS  int64

	firstGameTSMS uint32
	lastGameTSMS  uint32
	carOrdinal    int32
	carClass      int32
	carPI         int32
	// raceOn is uniform for the stint's whole span (an IsRaceOn flip splits);
	// sawRace records whether any tick had CurrentRaceTime > 0 — it classifies
	// the stint at close but never splits it (ADR 0013).
	raceOn  bool
	sawRace bool
	lapMin  uint16
	lapMax  uint16
}

// maxConsecutiveFailures is how many back-to-back tick-handling errors the
// writer rides out (closing the broken stint and starting fresh each time)
// before concluding persistence is truly broken and shutting the server down.
const maxConsecutiveFailures = 10

// Run consumes ticks from sub until ctx is cancelled or the channel closes.
// The active Stint (if any) is flushed before return.
//
// A single failed write no longer kills the process: the broken stint is
// closed best-effort and the next tick opens a fresh one. Only sustained
// failure (disk full, DB gone) propagates out.
func (w *Writer) Run(ctx context.Context, sub *stream.Subscription) error {
	defer w.shutdown()

	var failures int
	var lastDropped uint64
	for {
		select {
		case <-ctx.Done():
			return nil
		case t, ok := <-sub.C():
			if !ok {
				return nil
			}
			// A drop on this subscription is a permanent gap in the raw
			// capture — the one thing this tool exists to prevent. Say so.
			if d := sub.Dropped(); d != lastDropped {
				w.logger.Error("storage subscriber dropped ticks — raw capture has a gap",
					"dropped_delta", d-lastDropped, "dropped_total", d)
				lastDropped = d
			}
			if err := w.handle(t); err != nil {
				failures++
				w.logger.Error("writer handle tick", "err", err, "consecutive_failures", failures)
				if cerr := w.closeStint("error"); cerr != nil {
					w.logger.Error("close stint after handle error", "err", cerr)
				}
				if failures >= maxConsecutiveFailures {
					return err
				}
				continue
			}
			failures = 0
		}
	}
}

func (w *Writer) handle(t *tick.Tick) error {
	if w.sessionID == "" {
		if err := w.openSession(t); err != nil {
			return err
		}
	} else if reason := w.sessionSplitReason(t); reason != "" {
		if err := w.closeStint(reason); err != nil {
			return err
		}
		w.closeSession(reason)
		if err := w.openSession(t); err != nil {
			return err
		}
	}
	w.sessionLastNS = t.ServerRecvNS
	w.sessionLastTS = t.GameTSMillis

	if w.cur == nil {
		return w.openStint(t)
	}
	if reason := w.splitReason(t); reason != "" {
		if err := w.closeStint(reason); err != nil {
			return err
		}
		return w.openStint(t)
	}
	return w.appendTick(t)
}

// tsRegressToleranceMS absorbs out-of-order UDP delivery: only a GameTSMillis
// jump backwards by more than this counts as a game reboot. (A uint32 wrap at
// ~49.7 days of continuous game uptime would also read as a regression —
// implausible enough to ignore; noted in docs/data-needed.md.)
const tsRegressToleranceMS = 60_000

// sessionSplitReason returns a non-empty reason when `t` belongs to a new
// Session (ADR 0012): a silence gap of >= sessionGap, or the game's own clock
// jumping backwards (a relaunch — GameTSMillis restarts near zero).
func (w *Writer) sessionSplitReason(t *tick.Tick) string {
	if t.ServerRecvNS-w.sessionLastNS >= w.sessionGap.Nanoseconds() {
		return "session-gap"
	}
	if int64(t.GameTSMillis) < int64(w.sessionLastTS)-tsRegressToleranceMS {
		return "game-restart"
	}
	return ""
}

func (w *Writer) openSession(t *tick.Tick) error {
	id := sessionIDFromTime(time.Unix(0, t.ServerRecvNS))
	if _, err := w.store.db.Exec(
		`INSERT INTO sessions (id, started_at_ns) VALUES (?, ?)`,
		id, t.ServerRecvNS,
	); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	w.sessionID = id
	w.logger.Info("session started", "session", id)
	return nil
}

// closeSession stamps the session's end as its LAST TICK's arrival time (not
// wall-clock now — after an hour-long gap the session ended an hour ago). A
// session whose stints were all discarded is deleted outright, so long-running
// servers don't accumulate empty rows between drives.
func (w *Writer) closeSession(reason string) {
	if w.sessionID == "" {
		return
	}
	id := w.sessionID
	w.sessionID = ""

	var stints int
	if err := w.store.db.QueryRow(
		`SELECT COUNT(*) FROM stints WHERE session_id = ?`, id,
	).Scan(&stints); err != nil {
		w.logger.Error("count session stints", "session", id, "err", err)
		stints = 1 // fail safe: keep the row rather than risk deleting data
	}
	if stints == 0 {
		if _, err := w.store.db.Exec(`DELETE FROM sessions WHERE id = ?`, id); err != nil {
			w.logger.Error("delete empty session", "session", id, "err", err)
		} else {
			w.logger.Info("session discarded (no surviving stints)", "session", id, "reason", reason)
		}
		return
	}
	if _, err := w.store.db.Exec(
		`UPDATE sessions SET ended_at_ns = ? WHERE id = ?`,
		w.sessionLastNS, id,
	); err != nil {
		w.logger.Error("mark session ended", "session", id, "err", err)
		return
	}
	w.logger.Info("session closed", "session", id, "reason", reason, "stints", stints)
}

// splitReason returns a non-empty reason if a new Stint should start at `t`,
// otherwise "". Order: gap > race-state > car (ADR 0013).
func (w *Writer) splitReason(t *tick.Tick) string {
	if t.ServerRecvNS-w.cur.lastTickNS >= w.gapThreshold.Nanoseconds() {
		return "gap"
	}
	if t.IsRaceOn != w.cur.raceOn {
		return "race-state"
	}
	// Treat CarOrdinal==0 as "unknown / not yet populated" and don't split on
	// transitions to/from unknown. A genuine car swap shows both ordinals
	// non-zero and different.
	if w.cur.carOrdinal != 0 && t.CarOrdinal != 0 && w.cur.carOrdinal != t.CarOrdinal {
		return "car"
	}
	return ""
}

func (w *Writer) openStint(t *tick.Tick) error {
	ordinal, err := w.nextOrdinal()
	if err != nil {
		return err
	}
	id := fmt.Sprintf("%04d", ordinal)
	dir := w.store.HotDir(w.sessionID, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir stint dir: %w", err)
	}
	w.cur = &stintState{
		id:            id,
		ordinal:       ordinal,
		dir:           dir,
		path:          filepath.Join(dir, "*.parquet"),
		startedAtNS:   t.ServerRecvNS,
		lastTickNS:    t.ServerRecvNS,
		firstGameTSMS: t.GameTSMillis,
		lastGameTSMS:  t.GameTSMillis,
		carOrdinal:    t.CarOrdinal,
		carClass:      t.CarClass,
		carPI:         t.CarPerformanceIndex,
		raceOn:        t.IsRaceOn,
		lapMin:        t.LapNumber,
		lapMax:        t.LapNumber,
	}
	if err := w.openSegment(t.ServerRecvNS); err != nil {
		return err
	}
	if _, err := w.store.db.Exec(
		`INSERT INTO stints (id, session_id, ordinal, started_at_ns, parquet_path)
		 VALUES (?, ?, ?, ?, ?)`,
		w.stintRowID(), w.sessionID, ordinal, t.ServerRecvNS, w.cur.path,
	); err != nil {
		return fmt.Errorf("insert stint: %w", err)
	}
	return w.appendTick(t)
}

// openSegment starts the stint's next Parquet segment file.
func (w *Writer) openSegment(nowNS int64) error {
	cur := w.cur
	cur.seg++
	f, err := os.Create(filepath.Join(cur.dir, fmt.Sprintf("%04d.parquet", cur.seg)))
	if err != nil {
		return fmt.Errorf("create parquet segment: %w", err)
	}
	cur.file = f
	cur.pq = parquet.NewGenericWriter[parquetRow](f)
	cur.segStartNS = nowNS
	return nil
}

// closeSegment finalizes the current segment: footer, fsync, close. Once this
// returns nil the segment is durable — a later crash cannot lose it.
func closeSegment(cur *stintState) error {
	closeErr := cur.pq.Close()
	if syncErr := cur.file.Sync(); closeErr == nil {
		closeErr = syncErr
	}
	if cerr := cur.file.Close(); closeErr == nil {
		closeErr = cerr
	}
	return closeErr
}

// rotateSegment makes everything written so far crash-durable and continues
// the stint in a fresh segment (ADR 0011).
func (w *Writer) rotateSegment(nowNS int64) error {
	if err := closeSegment(w.cur); err != nil {
		return fmt.Errorf("close segment on rotate: %w", err)
	}
	if err := w.openSegment(nowNS); err != nil {
		return err
	}
	w.logger.Debug("rotated parquet segment", "stint", w.cur.id, "segment", w.cur.seg)
	return nil
}

func (w *Writer) appendTick(t *tick.Tick) error {
	row := toParquetRow(t)
	if _, err := w.cur.pq.Write([]parquetRow{row}); err != nil {
		return fmt.Errorf("parquet write: %w", err)
	}
	w.cur.tickCount++
	w.cur.lastTickNS = t.ServerRecvNS
	w.cur.lastGameTSMS = t.GameTSMillis
	if t.CurrentRaceTime > 0 {
		w.cur.sawRace = true
	}
	if t.LapNumber < w.cur.lapMin {
		w.cur.lapMin = t.LapNumber
	}
	if t.LapNumber > w.cur.lapMax {
		w.cur.lapMax = t.LapNumber
	}
	// Backfill car identity once a non-zero CarOrdinal arrives — splitReason
	// already ignores zero→nonzero transitions, so opening on an unknown car
	// must adopt the first real one without producing a fresh Stint.
	if w.cur.carOrdinal == 0 && t.CarOrdinal != 0 {
		w.cur.carOrdinal = t.CarOrdinal
		w.cur.carClass = t.CarClass
		w.cur.carPI = t.CarPerformanceIndex
	}
	if w.rotateEvery > 0 && t.ServerRecvNS-w.cur.segStartNS >= w.rotateEvery.Nanoseconds() {
		if err := w.rotateSegment(t.ServerRecvNS); err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) closeStint(reason string) error {
	if w.cur == nil {
		return nil
	}
	cur := w.cur
	w.cur = nil

	closeErr := closeSegment(cur)

	duration := time.Duration(cur.lastTickNS - cur.startedAtNS)
	stintType := resolveStintType(cur.raceOn, cur.sawRace, cur.lapMax-cur.lapMin)

	// Discard noise before persisting. Idle stints (menus / loading / pause)
	// and stints that never saw a real Car (CarOrdinal stays 0) carry no
	// analysable telemetry and only pollute the history. Each routes to the
	// same row+parquet removal as the sub-min case — and crucially runs before
	// aggregateStint, so no child rows (summaries) exist yet to orphan.
	if cause := discardCause(duration, w.minDuration, cur.tickCount, w.minTicks, cur.raceOn, cur.carOrdinal); cause != "" {
		if err := w.discardStint(cur, duration, cause); err != nil && closeErr == nil {
			closeErr = err
		}
		return closeErr
	}
	if _, err := w.store.db.Exec(
		`UPDATE stints
		 SET ended_at_ns = ?, tick_count = ?,
		     first_game_ts_ms = ?, last_game_ts_ms = ?,
		     car_ordinal = ?, car_class = ?, car_pi = ?,
		     stint_type = ?
		 WHERE id = ?`,
		cur.lastTickNS, cur.tickCount,
		cur.firstGameTSMS, cur.lastGameTSMS,
		cur.carOrdinal, cur.carClass, cur.carPI,
		stintType,
		stintRowID(w.sessionID, cur.ordinal),
	); err != nil {
		w.logger.Error("update stint row", "stint", cur.id, "err", err)
		if closeErr == nil {
			closeErr = err
		}
	}

	stintID := stintRowID(w.sessionID, cur.ordinal)

	// Aggregation scans the stint's whole parquet (seconds on long stints) and
	// used to run inline here — the one stall long enough to overflow the tick
	// subscription buffer and punch a gap in the raw capture. The row is
	// already finalized above, so summaries can land moments later without the
	// consume loop waiting. An aggregation failure no longer feeds the
	// writer's failure counter either: the raw ticks are safely on disk, only
	// the derived summary is missing.
	//
	// ponytail: a delete of this stint racing the seconds-long aggregation can
	// make the FK reject one side (logged, harmless). Serialize via a worker
	// queue if that ever bites.
	w.aggWG.Add(1)
	go func() {
		defer w.aggWG.Done()
		if err := aggregateStintFn(w.store.db, stintAggregateInput{
			stintID:     stintID,
			parquetPath: cur.path,
		}); err != nil {
			w.logger.Error("aggregate stint", "stint", cur.id, "err", err)
		}
	}()

	w.logger.Info("stint closed",
		"stint", cur.id,
		"reason", reason,
		"type", stintType,
		"ticks", cur.tickCount,
		"duration_ms", duration.Milliseconds(),
	)
	return closeErr
}

func (w *Writer) discardStint(cur *stintState, duration time.Duration, cause string) error {
	if _, err := w.store.db.Exec(
		`DELETE FROM stints WHERE id = ?`,
		stintRowID(w.sessionID, cur.ordinal),
	); err != nil {
		return fmt.Errorf("delete short stint row: %w", err)
	}
	removeParquet(w.logger, cur.id, cur.path)
	w.logger.Info("stint discarded",
		"stint", cur.id,
		"cause", cause,
		"ticks", cur.tickCount,
		"duration_ms", duration.Milliseconds(),
	)
	return nil
}

// discardCause returns a non-empty reason a freshly-closed stint should be
// dropped rather than persisted, or "" to keep it. Order is cheapest-first;
// the returned string is purely for the discard log line.
func discardCause(duration, minDuration time.Duration, tickCount, minTicks int64, raceOn bool, carOrdinal int32) string {
	if duration < minDuration {
		return "sub-min duration"
	}
	if tickCount < minTicks {
		return "too few ticks"
	}
	if !raceOn {
		return "idle"
	}
	if carOrdinal == 0 {
		return "no car"
	}
	return ""
}

// aggregateStintFn is a seam for tests to observe/block async aggregation;
// production always points at aggregateStint.
var aggregateStintFn = aggregateStint

// shutdown is the deferred cleanup path; logs but does not propagate errors,
// since Run() may be returning for an unrelated reason.
func (w *Writer) shutdown() {
	if err := w.closeStint("shutdown"); err != nil {
		w.logger.Error("close stint on shutdown", "err", err)
	}
	w.closeSession("shutdown")
	// Run() must not return until every async aggregation has landed — main's
	// shutdown drain closes the store right after, and a summary written to a
	// closed DB would be lost for good (nothing recomputes it later).
	w.aggWG.Wait()
}

// nextOrdinal returns MAX(ordinal)+1 so discarded stints don't cause ordinal
// reuse (which would overwrite the previous Parquet path).
func (w *Writer) nextOrdinal() (int, error) {
	var n int
	err := w.store.db.QueryRow(
		`SELECT COALESCE(MAX(ordinal), 0) FROM stints WHERE session_id = ?`, w.sessionID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("max ordinal: %w", err)
	}
	return n + 1, nil
}

func (w *Writer) stintRowID() string {
	return stintRowID(w.sessionID, w.cur.ordinal)
}

func stintRowID(sessionID string, ordinal int) string {
	return fmt.Sprintf("%s_%04d", sessionID, ordinal)
}

