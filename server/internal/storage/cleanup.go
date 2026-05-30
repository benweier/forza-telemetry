package storage

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
)

// pollutedStintCond selects stints with no analysable telemetry: idle ones
// (menus / loading / pause) and ones that never saw a real Car (car_ordinal
// stays 0, or NULL for a stint that crashed before its close UPDATE). These
// only pollute the history — the Writer now discards them at close, and this
// sweep removes any already persisted by an older build.
const pollutedStintCond = `stint_type = 'idle' OR car_ordinal = 0 OR car_ordinal IS NULL`

// childStintTables hold rows keyed by stint_id (FK → stints.id). They must be
// deleted before the parent stints rows to satisfy the foreign keys.
var childStintTables = []string{
	"turns",
	"straights",
	"stint_summary",
	"lap_summary",
	"preview_samples",
}

// sweepPollutedStints removes polluted stints (see pollutedStintCond), their
// child rows, and their Parquet files. It is idempotent — a no-op once the DB
// is clean — and is called once at startup, before any Writer opens, under the
// DuckDB single-writer lock, so no concurrent stint mutation can race it.
func sweepPollutedStints(db *sql.DB, logger *slog.Logger) error {
	rows, err := db.Query(`SELECT id, parquet_path FROM stints WHERE ` + pollutedStintCond)
	if err != nil {
		return fmt.Errorf("select polluted stints: %w", err)
	}
	type victim struct {
		id          string
		parquetPath string
	}
	var victims []victim
	for rows.Next() {
		var v victim
		if err := rows.Scan(&v.id, &v.parquetPath); err != nil {
			rows.Close()
			return fmt.Errorf("scan polluted stint: %w", err)
		}
		victims = append(victims, v)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate polluted stints: %w", err)
	}
	rows.Close()

	if len(victims) == 0 {
		return nil
	}

	// Children first (FK order), then the parent rows. Both filter by the same
	// condition so the set is consistent under the startup lock.
	for _, table := range childStintTables {
		if _, err := db.Exec(
			`DELETE FROM ` + table + ` WHERE stint_id IN (SELECT id FROM stints WHERE ` + pollutedStintCond + `)`,
		); err != nil {
			return fmt.Errorf("delete %s for polluted stints: %w", table, err)
		}
	}
	if _, err := db.Exec(`DELETE FROM stints WHERE ` + pollutedStintCond); err != nil {
		return fmt.Errorf("delete polluted stints: %w", err)
	}

	// Parquet removal is best-effort: a missing file (already-cleaned, or a
	// stint that crashed before writing) must not fail the sweep.
	for _, v := range victims {
		if v.parquetPath == "" {
			continue
		}
		if err := os.Remove(v.parquetPath); err != nil && !os.IsNotExist(err) {
			logger.Warn("remove polluted stint parquet", "stint", v.id, "path", v.parquetPath, "err", err)
		}
	}

	logger.Info("swept polluted stints", "count", len(victims))
	return nil
}

// sweepEmptySessions deletes session rows that have no stints. Run at startup
// after sweepPollutedStints and before NewWriter creates the active session,
// so any zero-stint session here is genuinely abandoned — it only ever held
// idle/no-car stints (just swept) or never produced one. Idempotent.
func sweepEmptySessions(db *sql.DB, logger *slog.Logger) error {
	res, err := db.Exec(
		`DELETE FROM sessions
		 WHERE NOT EXISTS (SELECT 1 FROM stints WHERE stints.session_id = sessions.id)`,
	)
	if err != nil {
		return fmt.Errorf("delete empty sessions: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		logger.Info("swept empty sessions", "count", n)
	}
	return nil
}
