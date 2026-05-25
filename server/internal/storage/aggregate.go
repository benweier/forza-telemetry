package storage

import (
	"database/sql"
	"fmt"
	"strings"
)

// aggregateStint computes the per-stint, per-lap, and 1Hz preview rows for a
// freshly-closed stint and inserts them. Reads back from the Parquet file
// just written by the Writer — DuckDB's read_parquet() is fast enough that a
// round-trip to disk is cheaper than maintaining a parallel streaming
// aggregator in Go.
//
// Called from Writer.closeStint after the stints row has been updated.
// Per-lap rows are emitted for every distinct lap_number present, including
// lap 0 (pre-race / out-lap segments captured before the first lap increment).
func aggregateStint(db *sql.DB, stintID, parquetPath string) error {
	pq := escapeSQLLiteral(parquetPath)
	if err := insertStintSummary(db, stintID, pq); err != nil {
		return fmt.Errorf("stint_summary: %w", err)
	}
	if err := insertLapSummaries(db, stintID, pq); err != nil {
		return fmt.Errorf("lap_summary: %w", err)
	}
	if err := insertPreviewSamples(db, stintID, pq); err != nil {
		return fmt.Errorf("preview_samples: %w", err)
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
   throttle_pct, brake_pct, rpm, pos_x, pos_z, lap_number)
SELECT
  ?,
  CAST((server_recv_ns - MIN(server_recv_ns) OVER ()) // 1000000000 AS INTEGER),
  server_recv_ns,
  speed_ms,
  lateral_g,
  longitudinal_g,
  throttle_pct,
  brake_pct,
  engine_rpm,
  pos_x,
  pos_z,
  lap_number
FROM (
  SELECT *, ROW_NUMBER() OVER (
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

// escapeSQLLiteral escapes single quotes for safe embedding in a SQL string
// literal. Parquet paths are server-controlled (built from XDG + session ID +
// stint ID), so this is defensive rather than load-bearing.
func escapeSQLLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
