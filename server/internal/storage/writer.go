package storage

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
			tickNS: t.ServerRecvNS,
			x:      t.PositionX,
			z:      t.PositionZ,
			longG:  t.LongitudinalG,
			latG:   t.LateralG,
			lap:    t.LapNumber,
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

	hotSpots := 0
	for _, err := range w.flushHotSpots(cur) {
		w.logger.Error("insert hot_spot", "stint", cur.id, "err", err)
		if closeErr == nil {
			closeErr = err
		}
	}
	for _, d := range cur.detectors {
		hotSpots += len(d.found)
	}

	corners := 0
	if stintType == stintTypeCircuit {
		var err error
		corners, err = w.flushCorners(cur)
		if err != nil {
			w.logger.Error("insert corners", "stint", cur.id, "err", err)
			if closeErr == nil {
				closeErr = err
			}
		}
	}

	if err := aggregateStint(w.store.db, stintRowID(w.sessionID, cur.ordinal), cur.path); err != nil {
		w.logger.Error("aggregate stint", "stint", cur.id, "err", err)
		if closeErr == nil {
			closeErr = err
		}
	}

	w.logger.Info("stint closed",
		"stint", cur.id,
		"reason", reason,
		"type", stintType,
		"ticks", cur.tickCount,
		"duration_ms", duration.Milliseconds(),
		"hot_spots", hotSpots,
		"corners", corners,
	)
	return closeErr
}

// flushCorners groups the collected pathSamples by lap, runs detection per
// lap, and inserts rows into the corners table. Returns total persisted
// count + first error encountered.
func (w *Writer) flushCorners(cur *stintState) (int, error) {
	if len(cur.pathSamples) == 0 {
		return 0, nil
	}
	stintID := stintRowID(w.sessionID, cur.ordinal)

	// Group consecutive samples by lap number; samples are already sorted
	// by tickNS so a simple split is enough.
	lapGroups := make(map[uint16][]pathSample)
	for _, s := range cur.pathSamples {
		lapGroups[s.lap] = append(lapGroups[s.lap], s)
	}

	var firstErr error
	total := 0
	for lap, samples := range lapGroups {
		corners := detectCorners(samples)
		for i, c := range corners {
			id := fmt.Sprintf("%s_lap%d_corner%d", stintID, lap, i+1)
			if _, err := w.store.db.Exec(
				`INSERT INTO corners
				 (id, stint_id, lap_number, corner_index,
				  started_at_ns, apex_tick_ns, ended_at_ns,
				  peak_curvature, peak_lateral_g, direction)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, stintID, int(lap), i+1,
				c.StartTickNS, c.ApexTickNS, c.EndTickNS,
				c.PeakCurvature, c.PeakLateralG, c.Direction,
			); err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("lap %d corner %d: %w", lap, i+1, err)
				}
				continue
			}
			total++
		}
	}
	return total, firstErr
}

// flushHotSpots drains each detector and inserts the resulting hot_spots rows.
// Returns any errors encountered; insertion continues past failures so we
// persist as many as possible.
func (w *Writer) flushHotSpots(cur *stintState) []error {
	var errs []error
	seq := 0
	stintID := stintRowID(w.sessionID, cur.ordinal)
	for _, d := range cur.detectors {
		candidates := d.flush(cur.lastTickNS)
		// Re-attach to d.found so the caller can count them after the loop;
		// flush() emptied it.
		d.found = candidates
		for _, c := range candidates {
			seq++
			id := fmt.Sprintf("%s_hs_%04d", stintID, seq)
			if _, err := w.store.db.Exec(
				`INSERT INTO hot_spots
				 (id, stint_id, type, started_at_ns, ended_at_ns, peak_tick_ns, peak_value, label)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				id, stintID, string(c.Type),
				c.StartNS, c.EndNS, c.PeakNS, float64(c.PeakValue), c.Label,
			); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", c.Type, err))
			}
		}
	}
	return errs
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
