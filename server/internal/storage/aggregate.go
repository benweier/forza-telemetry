package storage

import (
	"database/sql"
	"fmt"
	"strings"
)

// stintAggregateInput bundles everything a freshly-closed stint hands the
// aggregator. parquetPath drives the SQL passes (stint_summary, lap_summary,
// preview_samples). pathSamples drive the in-memory Turn + Straight passes.
type stintAggregateInput struct {
	stintID      string
	parquetPath  string
	stintStartNS int64
	stintEndNS   int64
	pathSamples  []pathSample // empty for non-race stints — Turn/Straight passes skipped
}

// aggregateStint computes per-stint, per-lap, and 1Hz preview rows, plus
// Turn and Straight rows for race-category stints. Reads back from the
// Parquet file just written by the Writer — DuckDB's read_parquet() is fast
// enough that a round-trip to disk is cheaper than maintaining a parallel
// streaming aggregator in Go.
//
// Called from Writer.closeStint after the stints row has been updated.
// Per-lap rows are emitted for every distinct lap_number present, including
// lap 0 (pre-race / out-lap segments captured before the first lap increment).
//
// Turn + Straight insertion happens *before* hot_spots insertion (handled
// by the Writer) so the XOR CHECK on hot_spots can resolve to a valid
// segment at INSERT time.
func aggregateStint(db *sql.DB, in stintAggregateInput) error {
	pq := escapeSQLLiteral(in.parquetPath)
	if err := insertStintSummary(db, in.stintID, pq); err != nil {
		return fmt.Errorf("stint_summary: %w", err)
	}
	if err := insertLapSummaries(db, in.stintID, pq); err != nil {
		return fmt.Errorf("lap_summary: %w", err)
	}
	if err := insertPreviewSamples(db, in.stintID, pq); err != nil {
		return fmt.Errorf("preview_samples: %w", err)
	}
	// Turn + Straight detection runs only when path samples were collected
	// (Circuit + Sprint stints per ADR 0008). Freeroam / Idle stints skip the
	// pass entirely — they have no path geometry to segment.
	if len(in.pathSamples) > 0 {
		turns := detectTurns(in.pathSamples)
		if err := insertTurns(db, in.stintID, turns); err != nil {
			return fmt.Errorf("turns: %w", err)
		}
		straights := deriveStraights(turns, in.pathSamples, in.stintStartNS, in.stintEndNS)
		if err := insertStraights(db, in.stintID, straights); err != nil {
			return fmt.Errorf("straights: %w", err)
		}
	}
	return nil
}

func insertStintSummary(db *sql.DB, stintID, escapedPath string) error {
	q := fmt.Sprintf(`
INSERT INTO stint_summary
  (stint_id, top_speed_ms, distance_m, avg_speed_ms, max_rpm,
   peak_lateral_g, peak_long_g, peak_brake_pct, gear_shift_count)
SELECT
  ?,
  MAX(speed_ms),
  COALESCE(MAX(distance_m) - MIN(distance_m), 0),
  AVG(speed_ms),
  MAX(engine_rpm),
  MAX(ABS(lateral_g)),
  MAX(ABS(longitudinal_g)),
  MAX(brake_pct),
  CAST(SUM(CASE WHEN gear_shift THEN 1 ELSE 0 END) AS INTEGER)
FROM read_parquet('%s')
`, escapedPath)
	_, err := db.Exec(q, stintID)
	return err
}

func insertLapSummaries(db *sql.DB, stintID, escapedPath string) error {
	q := fmt.Sprintf(`
INSERT INTO lap_summary
  (id, stint_id, lap_number, lap_time_s, top_speed_ms,
   distance_m, peak_lateral_g, peak_brake_pct)
SELECT
  ? || '_lap_' || CAST(lap_number AS VARCHAR),
  ?,
  lap_number,
  MAX(current_lap_s),
  MAX(speed_ms),
  MAX(lap_distance_m),
  MAX(ABS(lateral_g)),
  MAX(brake_pct)
FROM read_parquet('%s')
GROUP BY lap_number
ORDER BY lap_number
`, escapedPath)
	_, err := db.Exec(q, stintID, stintID)
	return err
}

func insertPreviewSamples(db *sql.DB, stintID, escapedPath string) error {
	// One sample per second (server-side bucket). Bucket index is
	// monotonic-from-zero so the scrub bar maps 0..N to N seconds.
	q := fmt.Sprintf(`
INSERT INTO preview_samples
  (stint_id, second_index, tick_ns, speed_ms, lateral_g, longitudinal_g,
   throttle_pct, brake_pct, rpm, pos_x, pos_y, pos_z, lap_number)
-- second_index must match the bucket the row was partitioned into.
-- Deriving it from (ns - MIN(ns)) // 1e9 produces duplicates when the
-- earliest row lands mid-second: two surviving rows can both round to 0.
-- The bucket value itself (ns // 1e9) is already the canonical key, so
-- offset it by MIN(bucket) to get a 0-based monotonic series.
SELECT
  ?,
  CAST(bucket - MIN(bucket) OVER () AS INTEGER),
  server_recv_ns,
  speed_ms,
  lateral_g,
  longitudinal_g,
  throttle_pct,
  brake_pct,
  engine_rpm,
  pos_x,
  pos_y,
  pos_z,
  lap_number
FROM (
  SELECT *,
    server_recv_ns // 1000000000 AS bucket,
    ROW_NUMBER() OVER (
      PARTITION BY server_recv_ns // 1000000000
      ORDER BY server_recv_ns
    ) AS rn
  FROM read_parquet('%s')
)
WHERE rn = 1
ORDER BY server_recv_ns
`, escapedPath)
	_, err := db.Exec(q, stintID)
	return err
}

func insertTurns(db *sql.DB, stintID string, turns []turnCandidate) error {
	for i, t := range turns {
		idx := i + 1
		id := fmt.Sprintf("%s_turn_%d", stintID, idx)
		if _, err := db.Exec(
			`INSERT INTO turns
			 (id, stint_id, turn_index, started_at_ns, apex_tick_ns, ended_at_ns,
			  peak_curvature, peak_delta_theta, direction, shape)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
			id, stintID, idx,
			t.StartTickNS, t.ApexTickNS, t.EndTickNS,
			t.PeakCurvature, t.PeakDeltaTheta, t.Direction,
		); err != nil {
			return fmt.Errorf("turn %d: %w", idx, err)
		}
	}
	return nil
}

func insertStraights(db *sql.DB, stintID string, straights []straightCandidate) error {
	for i, s := range straights {
		idx := i + 1
		id := fmt.Sprintf("%s_straight_%d", stintID, idx)
		// peak_speed_ms is nullable when no samples fall in the range —
		// only zero-length straights produce that.
		var peakSpeed any
		if s.PeakSpeedMS > 0 {
			peakSpeed = s.PeakSpeedMS
		}
		if _, err := db.Exec(
			`INSERT INTO straights
			 (id, stint_id, straight_index, started_at_ns, ended_at_ns,
			  distance_m, peak_speed_ms)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id, stintID, idx,
			s.StartTickNS, s.EndTickNS,
			s.DistanceM, peakSpeed,
		); err != nil {
			return fmt.Errorf("straight %d: %w", idx, err)
		}
	}
	return nil
}

// escapeSQLLiteral escapes single quotes for safe embedding in a SQL string
// literal. Parquet paths are server-controlled (built from XDG + session ID +
// stint ID), so this is defensive rather than load-bearing.
func escapeSQLLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
