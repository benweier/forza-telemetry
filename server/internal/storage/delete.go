package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotFound / ErrActive are sentinel errors the API layer maps to 404 / 409.
// Active means the resource is still recording (ended_at_ns IS NULL): its
// Parquet file is held open by the live Writer, so deleting it would pull the
// floor out from under ingest.
var (
	ErrNotFound = errors.New("not found")
	ErrActive   = errors.New("resource is actively recording")
)

// DeleteStint removes a closed stint: its child rows (FK order), the stint row,
// and its Parquet file. Refuses the actively-recording stint.
func (s *Store) DeleteStint(stintID string) error {
	var (
		parquetPath string
		ended       sql.NullInt64
	)
	err := s.db.QueryRow(
		`SELECT parquet_path, ended_at_ns FROM stints WHERE id = ?`, stintID,
	).Scan(&parquetPath, &ended)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lookup stint: %w", err)
	}
	if !ended.Valid {
		return ErrActive
	}

	// Children first, then the parent — each as its own autocommit statement.
	// DuckDB checks FK constraints against the transaction's start state, so a
	// single transaction deleting child + parent together fails the FK check
	// (see cleanup.go, which deletes the same way for the startup sweep).
	for _, table := range childStintTables {
		if _, err := s.db.Exec(`DELETE FROM `+table+` WHERE stint_id = ?`, stintID); err != nil {
			return fmt.Errorf("delete %s: %w", table, err)
		}
	}
	if _, err := s.db.Exec(`DELETE FROM stints WHERE id = ?`, stintID); err != nil {
		return fmt.Errorf("delete stint: %w", err)
	}

	removeParquet(s.logger, stintID, parquetPath)
	return nil
}

// DeleteSession removes a session and everything beneath it: all stint child
// rows, the stint rows, the session row, and every stint's Parquet file.
// Refuses the actively-recording session so the live Writer is never deleted
// out from under itself.
func (s *Store) DeleteSession(sessionID string) error {
	var ended sql.NullInt64
	err := s.db.QueryRow(
		`SELECT ended_at_ns FROM sessions WHERE id = ?`, sessionID,
	).Scan(&ended)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lookup session: %w", err)
	}
	if !ended.Valid {
		return ErrActive
	}

	rows, err := s.db.Query(`SELECT id, parquet_path FROM stints WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("list stints: %w", err)
	}
	type victim struct{ id, path string }
	var victims []victim
	for rows.Next() {
		var v victim
		if err := rows.Scan(&v.id, &v.path); err != nil {
			rows.Close()
			return fmt.Errorf("scan stint: %w", err)
		}
		victims = append(victims, v)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate stints: %w", err)
	}
	rows.Close()

	// Children → stints → session, each autocommit (DuckDB FK limitation; see
	// DeleteStint and cleanup.go).
	for _, table := range childStintTables {
		if _, err := s.db.Exec(
			`DELETE FROM `+table+` WHERE stint_id IN (SELECT id FROM stints WHERE session_id = ?)`,
			sessionID,
		); err != nil {
			return fmt.Errorf("delete %s: %w", table, err)
		}
	}
	if _, err := s.db.Exec(`DELETE FROM stints WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("delete stints: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, sessionID); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	for _, v := range victims {
		removeParquet(s.logger, v.id, v.path)
	}
	// Best-effort: drop the now-empty per-session Parquet directory.
	if len(victims) > 0 && victims[0].path != "" {
		_ = os.Remove(stintParentDir(victims[0].path))
	}
	return nil
}

// removeParquet deletes a stint's Parquet data best-effort — a missing file
// (already cleaned, or a stint that crashed before writing) is not an error.
// Handles both layouts: a segment glob (ADR 0011 — every match plus the
// segment directory) and a legacy single-file path.
func removeParquet(logger *slog.Logger, stintID, path string) {
	if path == "" {
		return
	}
	if !strings.Contains(path, "*") {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			logger.Warn("remove stint parquet", "stint", stintID, "path", path, "err", err)
		}
		return
	}
	matches, err := filepath.Glob(path)
	if err != nil {
		logger.Warn("glob stint parquet", "stint", stintID, "path", path, "err", err)
		return
	}
	for _, m := range matches {
		if err := os.Remove(m); err != nil && !os.IsNotExist(err) {
			logger.Warn("remove stint parquet segment", "stint", stintID, "path", m, "err", err)
		}
	}
	// The now-empty segment directory, then its per-session parent if this was
	// the session's last stint. Both best-effort — non-empty/missing is fine.
	_ = os.Remove(filepath.Dir(path))
	_ = os.Remove(filepath.Dir(filepath.Dir(path)))
}

// stintParentDir returns the per-session directory that holds a stint's
// parquet data: the parent of the segment dir for glob paths, the file's dir
// for legacy single-file paths.
func stintParentDir(path string) string {
	dir := filepath.Dir(path)
	if strings.Contains(path, "*") {
		dir = filepath.Dir(dir)
	}
	return dir
}
