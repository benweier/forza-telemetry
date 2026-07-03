package storage

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/parquet-go/parquet-go"

	"github.com/benweier/forza-telemetry/server/internal/tick"
)

// writeSegment writes a complete (footered) Parquet segment file.
func writeSegment(t *testing.T, path string, ticks []*tick.Tick) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := parquet.NewGenericWriter[parquetRow](f)
	for _, tk := range ticks {
		if _, err := w.Write([]parquetRow{toParquetRow(tk)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func raceTicks(n int, spacing time.Duration) []*tick.Tick {
	out := make([]*tick.Tick, n)
	for i := range out {
		out[i] = makeTick(int64(i)*int64(spacing), tickOpts{
			isRaceOn:        true,
			currentRaceTime: 10,
			carOrdinal:      100,
		})
	}
	return out
}

// Simulated crash: a stint row with NULL ended_at_ns, one durable segment and
// one footerless segment. Reopening the store (the real startup path) must
// delete the bad segment, finalize the stint from the good one, aggregate it,
// and backfill the crashed session — instead of sweeping hours of driving.
func TestRecoverCrashedStintOnStartup(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := New(dir, logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// 200 race ticks at 16ms ≈ 3.2s — clears the 2s / 180-tick discard floors.
	ticks := raceTicks(200, 16*time.Millisecond)
	lastNS := ticks[len(ticks)-1].ServerRecvNS

	stintDir := store.HotDir("sess", "0001")
	if err := os.MkdirAll(stintDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSegment(t, filepath.Join(stintDir, "0001.parquet"), ticks)
	// The segment that was open at the crash: no footer.
	if err := os.WriteFile(filepath.Join(stintDir, "0002.parquet"), []byte("no footer here"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustExecT(t, store, `INSERT INTO sessions (id, started_at_ns) VALUES ('sess', 0)`)
	mustExecT(t, store,
		`INSERT INTO stints (id, session_id, ordinal, started_at_ns, parquet_path) VALUES (?, 'sess', 1, 0, ?)`,
		"sess_0001", filepath.Join(stintDir, "*.parquet"))

	if err := store.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	store, err = New(dir, logger) // startup path: recover → sweeps → backfill
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer store.Close(context.Background())

	var (
		ended     *int64
		tickCount int64
		stintType *string
		carOrd    *int32
	)
	if err := store.db.QueryRow(
		`SELECT ended_at_ns, tick_count, stint_type, car_ordinal FROM stints WHERE id = 'sess_0001'`,
	).Scan(&ended, &tickCount, &stintType, &carOrd); err != nil {
		t.Fatalf("recovered stint row: %v", err)
	}
	if ended == nil || *ended != lastNS {
		t.Errorf("ended_at_ns: want %d got %v", lastNS, ended)
	}
	if tickCount != 200 {
		t.Errorf("tick_count: want 200 got %d", tickCount)
	}
	if stintType == nil || *stintType != "sprint" {
		t.Errorf("stint_type: want sprint got %v", stintType)
	}
	if carOrd == nil || *carOrd != 100 {
		t.Errorf("car_ordinal: want 100 got %v", carOrd)
	}
	if _, err := os.Stat(filepath.Join(stintDir, "0002.parquet")); !os.IsNotExist(err) {
		t.Errorf("footerless segment should be deleted, stat err: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stintDir, "0001.parquet")); err != nil {
		t.Errorf("durable segment must survive recovery: %v", err)
	}
	var summaries int
	if err := store.db.QueryRow(
		`SELECT COUNT(*) FROM stint_summary WHERE stint_id = 'sess_0001'`,
	).Scan(&summaries); err != nil {
		t.Fatal(err)
	}
	if summaries != 1 {
		t.Errorf("recovered stint must be aggregated: want 1 summary got %d", summaries)
	}
	var sessEnded *int64
	if err := store.db.QueryRow(`SELECT ended_at_ns FROM sessions WHERE id = 'sess'`).Scan(&sessEnded); err != nil {
		t.Fatal(err)
	}
	if sessEnded == nil || *sessEnded != lastNS {
		t.Errorf("crashed session should be backfilled to %d, got %v", lastNS, sessEnded)
	}
}

// Crashed stints with nothing durable (all-garbage segments) or too little
// data (under the discard floors) must be cleaned up, not resurrected.
func TestRecoverDiscardsUnsalvageableStints(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := New(dir, logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Stint 1: only a footerless segment — nothing durable.
	d1 := store.HotDir("sess", "0001")
	if err := os.MkdirAll(d1, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d1, "0001.parquet"), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Stint 2: durable but thin — 10 ticks, far under the 180-tick floor.
	d2 := store.HotDir("sess", "0002")
	if err := os.MkdirAll(d2, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSegment(t, filepath.Join(d2, "0001.parquet"), raceTicks(10, 300*time.Millisecond))

	mustExecT(t, store, `INSERT INTO sessions (id, started_at_ns) VALUES ('sess', 0)`)
	mustExecT(t, store,
		`INSERT INTO stints (id, session_id, ordinal, started_at_ns, parquet_path) VALUES (?, 'sess', 1, 0, ?)`,
		"sess_0001", filepath.Join(d1, "*.parquet"))
	mustExecT(t, store,
		`INSERT INTO stints (id, session_id, ordinal, started_at_ns, parquet_path) VALUES (?, 'sess', 2, 0, ?)`,
		"sess_0002", filepath.Join(d2, "*.parquet"))

	if err := store.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	store, err = New(dir, logger)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer store.Close(context.Background())

	var stints, sessions int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM stints`).Scan(&stints); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if stints != 0 || sessions != 0 {
		t.Errorf("unsalvageable stints/session should be gone: got %d stints, %d sessions", stints, sessions)
	}
	for _, d := range []string{d2} {
		if matches, _ := filepath.Glob(filepath.Join(d, "*.parquet")); len(matches) != 0 {
			t.Errorf("parquet segments should be removed from %s: %v", d, matches)
		}
	}
}

func mustExecT(t *testing.T, s *Store, q string, args ...any) {
	t.Helper()
	if _, err := s.db.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}
