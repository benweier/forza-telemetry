// Package storage owns persistence: DuckDB for queryable metadata + aggregates,
// Parquet files for raw Tick streams in hot/ and cold/ tiers (ADR 0001).
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
)

const duckdbFilename = "forza.duckdb"

type Store struct {
	dataDir string
	db      *sql.DB
	logger  *slog.Logger
}

// New ensures the storage directory layout exists under dataDir, opens the
// DuckDB metadata database, and applies schema migrations.
//
//	<dataDir>/
//	  forza.duckdb           // metadata + aggregates
//	  parquet/
//	    hot/<session>/<stint>.parquet
//	    cold/<session>/<stint>.parquet
func New(dataDir string, logger *slog.Logger) (*Store, error) {
	for _, sub := range []string{"parquet/hot", "parquet/cold"} {
		if err := os.MkdirAll(filepath.Join(dataDir, sub), 0o755); err != nil {
			return nil, fmt.Errorf("ensure %s: %w", sub, err)
		}
	}

	db, err := sql.Open("duckdb", filepath.Join(dataDir, duckdbFilename))
	if err != nil {
		return nil, fmt.Errorf("open duckdb: %w", err)
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{dataDir: dataDir, db: db, logger: logger.With("component", "storage")}, nil
}

// HotPath returns the absolute Parquet path for a hot Stint.
func (s *Store) HotPath(sessionID, stintID string) string {
	return filepath.Join(s.dataDir, "parquet", "hot", sessionID, stintID+".parquet")
}

// ColdPath returns the absolute Parquet path for a downsampled (cold) Stint.
func (s *Store) ColdPath(sessionID, stintID string) string {
	return filepath.Join(s.dataDir, "parquet", "cold", sessionID, stintID+".parquet")
}

// DB exposes the metadata database handle for read-only callers (REST endpoints).
func (s *Store) DB() *sql.DB { return s.db }

// NewWriter inserts a sessions row stamped at `now` and returns a Writer that
// owns the Stint lifecycle for that Session.
func (s *Store) NewWriter(now time.Time) (*Writer, error) {
	sessionID := sessionIDFromTime(now)
	if _, err := s.db.Exec(
		`INSERT INTO sessions (id, started_at_ns) VALUES (?, ?)`,
		sessionID, now.UnixNano(),
	); err != nil {
		return nil, fmt.Errorf("insert session: %w", err)
	}
	return &Writer{
		store:        s,
		sessionID:    sessionID,
		logger:       s.logger.With("session", sessionID),
		gapThreshold: 10 * time.Second,
		minDuration:  2 * time.Second,
	}, nil
}

// Close releases the DuckDB handle. Pending Writers must be closed first.
func (s *Store) Close(_ context.Context) error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func sessionIDFromTime(t time.Time) string {
	return t.UTC().Format("20060102T150405Z")
}
