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
	store := &Store{dataDir: dataDir, db: db, logger: logger.With("component", "storage")}
	// Drop tables from superseded schema versions (turns / straights / hot_spots)
	// whose lingering foreign keys onto stints would block stint/session deletes.
	// Runs before the sweeps (which also delete stints) and any Writer.
	if err := dropLegacyTables(db, store.logger); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("drop legacy tables: %w", err)
	}
	// Recover stints left open by a crash BEFORE the polluted-stint sweep —
	// recovery finalizes their rows from the durable Parquet segments; only
	// what recovery can't salvage falls through to the sweep.
	if err := recoverCrashedStints(db, store.logger); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("recover crashed stints: %w", err)
	}
	// Drop any polluted stints (idle / no-car) persisted by an older build.
	// Idempotent and runs before any Writer opens — the DuckDB single-writer
	// lock means nothing else can be mutating stints here.
	if err := sweepPollutedStints(db, store.logger); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sweep polluted stints: %w", err)
	}
	// Then drop sessions left empty (only ever held now-swept idle/no-car
	// stints). Runs before NewWriter, so no live session is at risk.
	if err := sweepEmptySessions(db, store.logger); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sweep empty sessions: %w", err)
	}
	// Finally, close out sessions orphaned by a crash so they stop reading as
	// "recording" and become deletable again.
	if err := backfillCrashedSessions(db, store.logger); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("backfill crashed sessions: %w", err)
	}
	return store, nil
}

// HotDir returns the directory holding a hot Stint's Parquet segment files.
// stints.parquet_path stores the `<dir>/*.parquet` glob over it (ADR 0011);
// databases from pre-rotation builds still hold single-file paths, which every
// consumer (read_parquet, removeParquet) handles transparently.
func (s *Store) HotDir(sessionID, stintID string) string {
	return filepath.Join(s.dataDir, "parquet", "hot", sessionID, stintID)
}

// ColdPath returns the absolute Parquet path for a downsampled (cold) Stint.
func (s *Store) ColdPath(sessionID, stintID string) string {
	return filepath.Join(s.dataDir, "parquet", "cold", sessionID, stintID+".parquet")
}

// DB exposes the metadata database handle for read-only callers (REST endpoints).
func (s *Store) DB() *sql.DB { return s.db }

// NewWriter returns a Writer that owns the Session + Stint lifecycle. Sessions
// are data-driven (ADR 0012): no row is created here — the Writer opens one on
// the first tick and closes it on a session boundary or at shutdown, so a
// server idling with no game running never manufactures empty sessions.
func (s *Store) NewWriter() *Writer {
	return &Writer{
		store:        s,
		logger:       s.logger,
		gapThreshold: stintGap,
		sessionGap:   sessionGap,
		minDuration:  minStintDuration,
		minTicks:     minStintTicks,
		rotateEvery:  segmentRotateEvery,
	}
}

// Boundary + persistence thresholds — shared by the Writer's close-time
// discard and startup crash recovery (recover.go), which must apply identical
// rules.
const (
	// stintGap: packets stopping this long ends the Stint (ADR 0013). In
	// active gameplay Forza streams continuously — a gap means the game was
	// closed/suspended or the network dropped.
	stintGap = 10 * time.Minute
	// sessionGap: packets stopping this long ends the whole Session (ADR
	// 0012). A GameTSMillis regression (game reboot) also ends it, regardless
	// of gap.
	sessionGap = time.Hour
	minStintDuration = 2 * time.Second
	// At ~60Hz, 180 ticks ≈ 3s of actual samples — a floor on data density
	// independent of wall-clock duration (a stint can span 2s+ but arrive
	// thin if packets dropped). Raise later as real captures inform it.
	minStintTicks = 180
	// segmentRotateEvery bounds crash loss to one segment (ADR 0011).
	segmentRotateEvery = 5 * time.Minute
)

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
