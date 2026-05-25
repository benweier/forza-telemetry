package storage

import (
	"database/sql"
	"fmt"
)

// schemaStatements is the canonical DDL applied at startup. Per ADR 0003 the
// schema evolves additively only — new columns get appended here and applied
// with `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` in a future migration.
var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS sessions (
		id            TEXT PRIMARY KEY,
		started_at_ns BIGINT NOT NULL,
		ended_at_ns   BIGINT,
		pinned        BOOLEAN NOT NULL DEFAULT FALSE,
		downsampled   BOOLEAN NOT NULL DEFAULT FALSE
	)`,
	`CREATE TABLE IF NOT EXISTS stints (
		id               TEXT PRIMARY KEY,
		session_id       TEXT NOT NULL REFERENCES sessions(id),
		ordinal          INTEGER NOT NULL,
		started_at_ns    BIGINT NOT NULL,
		ended_at_ns      BIGINT,
		first_game_ts_ms INTEGER,
		last_game_ts_ms  INTEGER,
		tick_count       BIGINT NOT NULL DEFAULT 0,
		stint_type       TEXT,
		car_ordinal      INTEGER,
		car_class        INTEGER,
		car_pi           INTEGER,
		parquet_path     TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_stints_session ON stints(session_id)`,
	`CREATE TABLE IF NOT EXISTS hot_spots (
		id            TEXT PRIMARY KEY,
		stint_id      TEXT NOT NULL REFERENCES stints(id),
		type          TEXT NOT NULL,
		started_at_ns BIGINT NOT NULL,
		ended_at_ns   BIGINT NOT NULL,
		peak_tick_ns  BIGINT NOT NULL,
		peak_value    DOUBLE NOT NULL,
		label         TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_hotspots_stint ON hot_spots(stint_id)`,
	`CREATE INDEX IF NOT EXISTS idx_hotspots_type ON hot_spots(stint_id, type)`,
	`CREATE TABLE IF NOT EXISTS corners (
		id              TEXT PRIMARY KEY,
		stint_id        TEXT NOT NULL REFERENCES stints(id),
		lap_number      INTEGER NOT NULL,
		corner_index    INTEGER NOT NULL,
		started_at_ns   BIGINT NOT NULL,
		apex_tick_ns    BIGINT NOT NULL,
		ended_at_ns     BIGINT NOT NULL,
		peak_curvature  DOUBLE NOT NULL,
		peak_lateral_g  DOUBLE NOT NULL,
		direction       TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_corners_stint ON corners(stint_id, lap_number, corner_index)`,
	`CREATE TABLE IF NOT EXISTS stint_summary (
		stint_id          TEXT PRIMARY KEY REFERENCES stints(id),
		top_speed_ms      DOUBLE,
		distance_m        DOUBLE,
		avg_speed_ms      DOUBLE,
		max_rpm           DOUBLE,
		peak_lateral_g    DOUBLE,
		peak_long_g       DOUBLE,
		peak_brake_pct    DOUBLE,
		gear_shift_count  INTEGER
	)`,
	`CREATE TABLE IF NOT EXISTS lap_summary (
		id              TEXT PRIMARY KEY,
		stint_id        TEXT NOT NULL REFERENCES stints(id),
		lap_number      INTEGER NOT NULL,
		lap_time_s      DOUBLE,
		top_speed_ms    DOUBLE,
		distance_m      DOUBLE,
		peak_lateral_g  DOUBLE,
		peak_brake_pct  DOUBLE
	)`,
	`CREATE INDEX IF NOT EXISTS idx_lap_summary_stint ON lap_summary(stint_id, lap_number)`,
	`CREATE TABLE IF NOT EXISTS preview_samples (
		stint_id        TEXT NOT NULL REFERENCES stints(id),
		second_index    INTEGER NOT NULL,
		tick_ns         BIGINT NOT NULL,
		speed_ms        DOUBLE,
		lateral_g       DOUBLE,
		longitudinal_g  DOUBLE,
		throttle_pct    DOUBLE,
		brake_pct       DOUBLE,
		rpm             DOUBLE,
		pos_x           DOUBLE,
		pos_y           DOUBLE,
		pos_z           DOUBLE,
		lap_number      INTEGER,
		PRIMARY KEY (stint_id, second_index)
	)`,
	`ALTER TABLE preview_samples ADD COLUMN IF NOT EXISTS pos_y DOUBLE`,
}

func migrate(db *sql.DB) error {
	for i, stmt := range schemaStatements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("schema statement %d: %w", i, err)
		}
	}
	if err := backfillPreviewPosY(db); err != nil {
		return fmt.Errorf("backfill pos_y: %w", err)
	}
	return nil
}

// backfillPreviewPosY repopulates preview_samples.pos_y for stints aggregated
// before the column existed. The column was added via ALTER TABLE ... ADD COLUMN,
// which leaves existing rows NULL. The parquet files still hold pos_y at full
// resolution, so we join by tick_ns and patch row-by-stint. Idempotent: the
// WHERE pos_y IS NULL clause makes a second run a no-op.
func backfillPreviewPosY(db *sql.DB) error {
	var needBackfill int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM preview_samples WHERE pos_y IS NULL`,
	).Scan(&needBackfill); err != nil {
		return fmt.Errorf("count: %w", err)
	}
	if needBackfill == 0 {
		return nil
	}
	rows, err := db.Query(`
		SELECT DISTINCT s.id, s.parquet_path
		FROM stints s
		JOIN preview_samples ps ON ps.stint_id = s.id
		WHERE ps.pos_y IS NULL
	`)
	if err != nil {
		return fmt.Errorf("list stints: %w", err)
	}
	type stintRef struct {
		id, path string
	}
	var refs []stintRef
	for rows.Next() {
		var r stintRef
		if err := rows.Scan(&r.id, &r.path); err != nil {
			rows.Close()
			return fmt.Errorf("scan: %w", err)
		}
		refs = append(refs, r)
	}
	rows.Close()
	for _, r := range refs {
		q := fmt.Sprintf(`
			UPDATE preview_samples
			SET pos_y = pq.pos_y
			FROM (SELECT server_recv_ns, pos_y FROM read_parquet('%s')) pq
			WHERE preview_samples.stint_id = ?
			  AND preview_samples.tick_ns = pq.server_recv_ns
			  AND preview_samples.pos_y IS NULL
		`, escapeSQLLiteral(r.path))
		if _, err := db.Exec(q, r.id); err != nil {
			return fmt.Errorf("update %s: %w", r.id, err)
		}
	}
	return nil
}
