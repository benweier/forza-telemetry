package storage

import (
	"database/sql"
	"fmt"
)

// schemaStatements is the canonical DDL applied at startup. The Tick schema is
// additive only per ADR 0003; the domain tables (turns / straights / hot_spots)
// follow whatever shape the current detectors emit. After ADR 0008 superseded
// ADR 0007, the old `corners` table was dropped outright.
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
	`CREATE TABLE IF NOT EXISTS turns (
		id                TEXT PRIMARY KEY,
		stint_id          TEXT NOT NULL REFERENCES stints(id),
		turn_index        INTEGER NOT NULL,
		started_at_ns     BIGINT NOT NULL,
		apex_tick_ns      BIGINT NOT NULL,
		ended_at_ns       BIGINT NOT NULL,
		peak_curvature    DOUBLE NOT NULL,
		peak_delta_theta  DOUBLE NOT NULL,
		direction         TEXT NOT NULL,
		shape             TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS idx_turns_stint ON turns(stint_id, turn_index)`,
	`CREATE TABLE IF NOT EXISTS straights (
		id              TEXT PRIMARY KEY,
		stint_id        TEXT NOT NULL REFERENCES stints(id),
		straight_index  INTEGER NOT NULL,
		started_at_ns   BIGINT NOT NULL,
		ended_at_ns     BIGINT NOT NULL,
		distance_m      DOUBLE NOT NULL,
		peak_speed_ms   DOUBLE
	)`,
	`CREATE INDEX IF NOT EXISTS idx_straights_stint ON straights(stint_id, straight_index)`,
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
}

func migrate(db *sql.DB) error {
	for i, stmt := range schemaStatements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("schema statement %d: %w", i, err)
		}
	}
	return nil
}
