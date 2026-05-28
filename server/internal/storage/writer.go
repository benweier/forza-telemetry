package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/parquet-go/parquet-go"

	"github.com/benweier/forza-telemetry/server/internal/stream"
	"github.com/benweier/forza-telemetry/server/internal/tick"
)

// Writer owns the per-Session Stint lifecycle: it reads ticks from a broker
// subscription and splits them into Stints on the rules from ADR 0006:
//
//   - a packet-arrival gap of >= gapThreshold
//   - a change of stint category (idle / freeroam / race)
//   - a change of Car (CarOrdinal)
//
// Each Stint is written to its own Parquet file under parquet/hot/<session>/,
// with metadata rows maintained in DuckDB. Stints shorter than minDuration are
// discarded at close (file removed, row deleted).
type Writer struct {
	store        *Store
	sessionID    string
	logger       *slog.Logger
	gapThreshold time.Duration
	minDuration  time.Duration

	cur *stintState
}

type stintState struct {
	id            string
	ordinal       int
	path          string
	file          *os.File
	pq            *parquet.GenericWriter[parquetRow]
	tickCount     int64
	startedAtNS   int64
	lastTickNS    int64
	firstGameTSMS uint32
	lastGameTSMS  uint32
	carOrdinal    int32
	carClass      int32
	carPI         int32
	category      stintCategory
	lapMin        uint16
	lapMax        uint16
	detectors     []*peakDetector
	collectPath   bool
	pathSamples   []pathSample
}

// Run consumes ticks from sub until ctx is cancelled or the channel closes.
// The active Stint (if any) is flushed before return.
func (w *Writer) Run(ctx context.Context, sub *stream.Subscription) error {
	defer w.shutdown()

	for {
		select {
		case <-ctx.Done():
			return nil
		case t, ok := <-sub.C():
			if !ok {
				return nil
			}
			if err := w.handle(t); err != nil {
				w.logger.Error("writer handle tick", "err", err)
				return err
			}
		}
	}
}

func (w *Writer) handle(t *tick.Tick) error {
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

// splitReason returns a non-empty reason if a new Stint should start at `t`,
// otherwise "". Order: gap > category > car.
func (w *Writer) splitReason(t *tick.Tick) string {
	if t.ServerRecvNS-w.cur.lastTickNS >= w.gapThreshold.Nanoseconds() {
		return "gap"
	}
	if categorize(t) != w.cur.category {
		return "type"
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
	path := w.store.HotPath(w.sessionID, id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir stint dir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create parquet file: %w", err)
	}
	pq := parquet.NewGenericWriter[parquetRow](f)
	w.cur = &stintState{
		id:            id,
		ordinal:       ordinal,
		path:          path,
		file:          f,
		pq:            pq,
		startedAtNS:   t.ServerRecvNS,
		lastTickNS:    t.ServerRecvNS,
		firstGameTSMS: t.GameTSMillis,
		lastGameTSMS:  t.GameTSMillis,
		carOrdinal:    t.CarOrdinal,
		carClass:      t.CarClass,
		carPI:         t.CarPerformanceIndex,
		category:      categorize(t),
		lapMin:        t.LapNumber,
		lapMax:        t.LapNumber,
		detectors:     newDefaultDetectors(),
		collectPath:   categorize(t) == categoryRace,
	}
	if _, err := w.store.db.Exec(
		`INSERT INTO stints (id, session_id, ordinal, started_at_ns, parquet_path)
		 VALUES (?, ?, ?, ?, ?)`,
		w.stintRowID(), w.sessionID, ordinal, t.ServerRecvNS, path,
	); err != nil {
		return fmt.Errorf("insert stint: %w", err)
	}
	return w.appendTick(t)
}

func (w *Writer) appendTick(t *tick.Tick) error {
	row := toParquetRow(t)
	if _, err := w.cur.pq.Write([]parquetRow{row}); err != nil {
		return fmt.Errorf("parquet write: %w", err)
	}
	w.cur.tickCount++
	w.cur.lastTickNS = t.ServerRecvNS
	w.cur.lastGameTSMS = t.GameTSMillis
	if t.LapNumber < w.cur.lapMin {
		w.cur.lapMin = t.LapNumber
	}
	if t.LapNumber > w.cur.lapMax {
		w.cur.lapMax = t.LapNumber
	}
	for _, d := range w.cur.detectors {
		d.feed(t)
	}
	if w.cur.collectPath {
		w.cur.pathSamples = append(w.cur.pathSamples, pathSample{
			tickNS:  t.ServerRecvNS,
			x:       t.PositionX,
			z:       t.PositionZ,
			speedMS: t.Speed,
			longG:   t.LongitudinalG,
			latG:    t.LateralG,
			lap:     t.LapNumber,
		})
	}
	// Backfill car identity once a non-zero CarOrdinal arrives — splitReason
	// already ignores zero→nonzero transitions, so opening on an unknown car
	// must adopt the first real one without producing a fresh Stint.
	if w.cur.carOrdinal == 0 && t.CarOrdinal != 0 {
		w.cur.carOrdinal = t.CarOrdinal
		w.cur.carClass = t.CarClass
		w.cur.carPI = t.CarPerformanceIndex
	}
	return nil
}

func (w *Writer) closeStint(reason string) error {
	if w.cur == nil {
		return nil
	}
	cur := w.cur
	w.cur = nil

	closeErr := cur.pq.Close()
	if syncErr := cur.file.Sync(); closeErr == nil {
		closeErr = syncErr
	}
	if cerr := cur.file.Close(); closeErr == nil {
		closeErr = cerr
	}

	duration := time.Duration(cur.lastTickNS - cur.startedAtNS)
	if duration < w.minDuration {
		if err := w.discardStint(cur, duration, reason); err != nil && closeErr == nil {
			closeErr = err
		}
		return closeErr
	}

	stintType := resolveStintType(cur.category, cur.lapMax-cur.lapMin)
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

	// Aggregator runs first so turns + straights exist when hot-spots are
	// inserted (the XOR CHECK requires a segment_id at INSERT time).
	if err := aggregateStint(w.store.db, stintAggregateInput{
		stintID:      stintID,
		parquetPath:  cur.path,
		stintStartNS: cur.startedAtNS,
		stintEndNS:   cur.lastTickNS,
		pathSamples:  cur.pathSamples,
	}); err != nil {
		w.logger.Error("aggregate stint", "stint", cur.id, "err", err)
		if closeErr == nil {
			closeErr = err
		}
	}

	hotSpots := 0
	for _, err := range w.flushHotSpots(cur, stintID) {
		w.logger.Error("insert hot_spot", "stint", cur.id, "err", err)
		if closeErr == nil {
			closeErr = err
		}
	}
	for _, d := range cur.detectors {
		hotSpots += len(d.found)
	}

	var turnCount, straightCount int
	_ = w.store.db.QueryRow(
		`SELECT COUNT(*) FROM turns WHERE stint_id = ?`, stintID,
	).Scan(&turnCount)
	_ = w.store.db.QueryRow(
		`SELECT COUNT(*) FROM straights WHERE stint_id = ?`, stintID,
	).Scan(&straightCount)

	w.logger.Info("stint closed",
		"stint", cur.id,
		"reason", reason,
		"type", stintType,
		"ticks", cur.tickCount,
		"duration_ms", duration.Milliseconds(),
		"hot_spots", hotSpots,
		"turns", turnCount,
		"straights", straightCount,
	)
	return closeErr
}

// flushHotSpots drains each detector and inserts hot_spots rows with each
// peak attributed to the Turn or Straight whose tick range contains it. The
// segment lookup is built from the rows the aggregator just inserted — every
// tick in the stint is covered exactly once thanks to the K+1 Straight
// invariant (per ADR 0008), so attribution should always succeed. If a peak
// falls outside any segment (defensive), the hot-spot is dropped with a
// logged error since the XOR CHECK would otherwise reject the INSERT.
func (w *Writer) flushHotSpots(cur *stintState, stintID string) []error {
	var errs []error

	segs, err := loadSegments(w.store.db, stintID)
	if err != nil {
		return []error{fmt.Errorf("load segments: %w", err)}
	}

	seq := 0
	for _, d := range cur.detectors {
		candidates := d.flush(cur.lastTickNS)
		// Re-attach to d.found so the caller can count them after the loop;
		// flush() emptied it.
		d.found = candidates
		for _, c := range candidates {
			seq++
			turnID, straightID := attributeToSegment(segs, c.PeakNS)
			if turnID == "" && straightID == "" {
				errs = append(errs, fmt.Errorf("%s: peak %d unattributed", c.Type, c.PeakNS))
				continue
			}
			id := fmt.Sprintf("%s_hs_%04d", stintID, seq)
			var tID, sID any
			if turnID != "" {
				tID = turnID
			}
			if straightID != "" {
				sID = straightID
			}
			if _, err := w.store.db.Exec(
				`INSERT INTO hot_spots
				 (id, stint_id, type, started_at_ns, ended_at_ns,
				  peak_tick_ns, peak_value, label, turn_id, straight_id)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, stintID, string(c.Type),
				c.StartNS, c.EndNS, c.PeakNS, float64(c.PeakValue), c.Label,
				tID, sID,
			); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", c.Type, err))
			}
		}
	}
	return errs
}

// segmentRef is one Turn or Straight row, sorted by startNS for attribution
// lookups.
type segmentRef struct {
	id      string
	isTurn  bool
	startNS int64
	endNS   int64
}

// loadSegments fetches turns + straights for the stint, sorted chronologically.
// Returns a single merged slice so attribution is one pass per hot-spot.
func loadSegments(db *sql.DB, stintID string) ([]segmentRef, error) {
	var out []segmentRef
	rows, err := db.Query(
		`SELECT id, started_at_ns, ended_at_ns FROM turns WHERE stint_id = ?`,
		stintID,
	)
	if err != nil {
		return nil, fmt.Errorf("turns: %w", err)
	}
	for rows.Next() {
		var r segmentRef
		r.isTurn = true
		if err := rows.Scan(&r.id, &r.startNS, &r.endNS); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan turn: %w", err)
		}
		out = append(out, r)
	}
	rows.Close()

	rows2, err := db.Query(
		`SELECT id, started_at_ns, ended_at_ns FROM straights WHERE stint_id = ?`,
		stintID,
	)
	if err != nil {
		return nil, fmt.Errorf("straights: %w", err)
	}
	for rows2.Next() {
		var r segmentRef
		if err := rows2.Scan(&r.id, &r.startNS, &r.endNS); err != nil {
			rows2.Close()
			return nil, fmt.Errorf("scan straight: %w", err)
		}
		out = append(out, r)
	}
	rows2.Close()

	sort.Slice(out, func(i, j int) bool { return out[i].startNS < out[j].startNS })
	return out, nil
}

// attributeToSegment returns (turnID, straightID) where exactly one is non-empty
// if peakNS lies within a segment's [startNS, endNS] (inclusive). Returns two
// empty strings if no segment contains the peak.
func attributeToSegment(segs []segmentRef, peakNS int64) (string, string) {
	for _, s := range segs {
		if peakNS >= s.startNS && peakNS <= s.endNS {
			if s.isTurn {
				return s.id, ""
			}
			return "", s.id
		}
	}
	return "", ""
}

func (w *Writer) discardStint(cur *stintState, duration time.Duration, reason string) error {
	if _, err := w.store.db.Exec(
		`DELETE FROM stints WHERE id = ?`,
		stintRowID(w.sessionID, cur.ordinal),
	); err != nil {
		return fmt.Errorf("delete short stint row: %w", err)
	}
	if err := os.Remove(cur.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove short stint parquet: %w", err)
	}
	w.logger.Info("stint discarded (sub-min duration)",
		"stint", cur.id,
		"reason", reason,
		"ticks", cur.tickCount,
		"duration_ms", duration.Milliseconds(),
	)
	return nil
}

// shutdown is the deferred cleanup path; logs but does not propagate errors,
// since Run() may be returning for an unrelated reason.
func (w *Writer) shutdown() {
	if err := w.closeStint("shutdown"); err != nil {
		w.logger.Error("close stint on shutdown", "err", err)
	}
	if _, err := w.store.db.Exec(
		`UPDATE sessions SET ended_at_ns = ? WHERE id = ?`,
		time.Now().UnixNano(), w.sessionID,
	); err != nil {
		w.logger.Error("mark session ended", "err", err)
	}
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

// SessionID exposes the active session ID for tests and logs.
func (w *Writer) SessionID() string { return w.sessionID }
