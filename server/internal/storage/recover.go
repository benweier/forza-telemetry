package storage

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// recoverCrashedStints finalizes stints left open by a crash (ended_at_ns IS
// NULL at startup). Segment rotation (ADR 0011) means every closed segment is
// a complete Parquet file; only the segment that was open at the crash lacks
// its footer. Recovery deletes the unreadable segment(s), then closes the
// stint row exactly as Writer.closeStint would have — same fields, same
// discard rules, same aggregation — so at most rotateEvery of driving is lost
// instead of the whole stint.
//
// Runs at startup under the DuckDB single-writer lock, before the sweeps and
// before any Writer opens. Legacy single-file stints (pre-rotation builds) are
// unreadable by construction after a crash and are left for the polluted-stint
// sweep, which removes them as before.
func recoverCrashedStints(db *sql.DB, logger *slog.Logger) error {
	rows, err := db.Query(`SELECT id, parquet_path FROM stints WHERE ended_at_ns IS NULL`)
	if err != nil {
		return fmt.Errorf("select crashed stints: %w", err)
	}
	type crashed struct{ id, path string }
	var victims []crashed
	for rows.Next() {
		var c crashed
		if err := rows.Scan(&c.id, &c.path); err != nil {
			rows.Close()
			return fmt.Errorf("scan crashed stint: %w", err)
		}
		victims = append(victims, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate crashed stints: %w", err)
	}
	rows.Close()

	for _, c := range victims {
		if !strings.Contains(c.path, "*") {
			continue // legacy single-file stint: footerless; the sweep handles it
		}
		if err := recoverOneStint(db, logger, c.id, c.path); err != nil {
			// Recovery is best-effort per stint — log and let the sweep take it.
			logger.Error("recover crashed stint", "stint", c.id, "err", err)
		}
	}
	return nil
}

func recoverOneStint(db *sql.DB, logger *slog.Logger, stintID, glob string) error {
	segs, err := filepath.Glob(glob)
	if err != nil {
		return fmt.Errorf("glob segments: %w", err)
	}
	sort.Strings(segs)

	// Drop segments DuckDB can't read (no footer — mid-write at the crash) and
	// empty-but-valid ones (a rotation that never received a tick).
	readable := 0
	for _, seg := range segs {
		var n int64
		err := db.QueryRow(
			fmt.Sprintf(`SELECT COUNT(*) FROM read_parquet('%s')`, escapeSQLLiteral(seg)),
		).Scan(&n)
		if err != nil || n == 0 {
			if rmErr := os.Remove(seg); rmErr != nil && !os.IsNotExist(rmErr) {
				logger.Warn("remove unreadable segment", "stint", stintID, "path", seg, "err", rmErr)
			}
			continue
		}
		readable++
	}
	if readable == 0 {
		// Nothing salvageable; the sweep deletes the row (car_ordinal is NULL).
		_ = os.Remove(filepath.Dir(glob))
		logger.Info("crashed stint had no durable segments", "stint", stintID)
		return nil
	}

	// Reconstruct Writer.closeStint's fields from the durable segments. A stint
	// has one category and one car for its whole span (ADR 0006 invariants), so
	// aggregates over the ticks recover them exactly; MAX() on the car fields
	// mirrors the writer's backfill-first-nonzero semantics.
	var (
		tickCount            int64
		firstNS, lastNS      int64
		firstGameTS          int64
		lastGameTS           int64
		carOrdinal           int32
		carClass, carPI      int32
		lapMin, lapMax       int64
		anyRaceOn, anyRaceTm bool
	)
	if err := db.QueryRow(fmt.Sprintf(
		`SELECT COUNT(*), MIN(server_recv_ns), MAX(server_recv_ns),
		        MIN(game_ts_ms), MAX(game_ts_ms),
		        MAX(car_ordinal), MAX(car_class), MAX(car_pi),
		        MIN(lap_number), MAX(lap_number),
		        BOOL_OR(is_race_on), BOOL_OR(current_race_s > 0)
		 FROM read_parquet('%s')`, escapeSQLLiteral(glob)),
	).Scan(&tickCount, &firstNS, &lastNS, &firstGameTS, &lastGameTS,
		&carOrdinal, &carClass, &carPI, &lapMin, &lapMax, &anyRaceOn, &anyRaceTm); err != nil {
		return fmt.Errorf("scan recovered stats: %w", err)
	}

	category := categoryIdle
	if anyRaceOn {
		category = categoryFreeroam
		if anyRaceTm {
			category = categoryRace
		}
	}
	duration := time.Duration(lastNS - firstNS)
	if cause := discardCause(duration, minStintDuration, tickCount, minStintTicks, category, carOrdinal); cause != "" {
		if _, err := db.Exec(`DELETE FROM stints WHERE id = ?`, stintID); err != nil {
			return fmt.Errorf("delete unrecoverable stint row: %w", err)
		}
		removeParquet(logger, stintID, glob)
		logger.Info("crashed stint recovered then discarded", "stint", stintID, "cause", cause)
		return nil
	}

	stintType := resolveStintType(category, uint16(lapMax-lapMin))
	if _, err := db.Exec(
		`UPDATE stints
		 SET ended_at_ns = ?, tick_count = ?,
		     first_game_ts_ms = ?, last_game_ts_ms = ?,
		     car_ordinal = ?, car_class = ?, car_pi = ?,
		     stint_type = ?
		 WHERE id = ?`,
		lastNS, tickCount, firstGameTS, lastGameTS,
		carOrdinal, carClass, carPI, stintType, stintID,
	); err != nil {
		return fmt.Errorf("finalize recovered stint: %w", err)
	}
	if err := aggregateStint(db, stintAggregateInput{stintID: stintID, parquetPath: glob}); err != nil {
		return fmt.Errorf("aggregate recovered stint: %w", err)
	}
	logger.Info("recovered crashed stint",
		"stint", stintID,
		"type", stintType,
		"ticks", tickCount,
		"duration_ms", duration.Milliseconds(),
	)
	return nil
}
