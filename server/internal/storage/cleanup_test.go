package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// seedStint inserts a stints row (plus a child preview_samples row and a real
// parquet file on disk) so the sweep's row + child + file removal can all be
// asserted. Returns the parquet path.
func seedStint(t *testing.T, s *Store, id, stintType string, carOrdinal *int32) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, id+".parquet")
	if err := os.WriteFile(path, []byte("stub"), 0o644); err != nil {
		t.Fatalf("write parquet: %v", err)
	}
	if _, err := s.db.Exec(
		`INSERT INTO stints (id, session_id, ordinal, started_at_ns, tick_count, stint_type, car_ordinal, parquet_path)
		 VALUES (?, 'sess', 1, 0, 10, ?, ?, ?)`,
		id, stintType, carOrdinal, path,
	); err != nil {
		t.Fatalf("insert stint %s: %v", id, err)
	}
	if _, err := s.db.Exec(
		`INSERT INTO preview_samples (stint_id, second_index, tick_ns) VALUES (?, 0, 0)`, id,
	); err != nil {
		t.Fatalf("insert preview_sample for %s: %v", id, err)
	}
	return path
}

func stintExists(t *testing.T, s *Store, id string) bool {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM stints WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("count stint %s: %v", id, err)
	}
	return n > 0
}

func previewExists(t *testing.T, s *Store, id string) bool {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM preview_samples WHERE stint_id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("count preview %s: %v", id, err)
	}
	return n > 0
}

func TestSweepPollutedStints(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.db.Exec(`INSERT INTO sessions (id, started_at_ns) VALUES ('sess', 0)`); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	car := int32(1651)
	zero := int32(0)
	idlePath := seedStint(t, s, "idle1", "idle", &car)    // idle type → purge
	zeroPath := seedStint(t, s, "zero1", "sprint", &zero) // car 0 → purge
	nullPath := seedStint(t, s, "null1", "sprint", nil)   // car NULL → purge
	keepPath := seedStint(t, s, "keep1", "circuit", &car) // real → keep

	if err := sweepPollutedStints(s.db, s.logger); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	for _, id := range []string{"idle1", "zero1", "null1"} {
		if stintExists(t, s, id) {
			t.Errorf("stint %s should have been purged", id)
		}
		if previewExists(t, s, id) {
			t.Errorf("preview_samples for %s should have been purged", id)
		}
	}
	for _, p := range []string{idlePath, zeroPath, nullPath} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("parquet %s should have been removed (err=%v)", p, err)
		}
	}

	if !stintExists(t, s, "keep1") {
		t.Error("circuit stint with a real car must be kept")
	}
	if !previewExists(t, s, "keep1") {
		t.Error("kept stint's child rows must survive")
	}
	if _, err := os.Stat(keepPath); err != nil {
		t.Errorf("kept stint parquet must survive: %v", err)
	}

	// Idempotent: a second sweep on the now-clean DB is a no-op.
	if err := sweepPollutedStints(s.db, s.logger); err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if !stintExists(t, s, "keep1") {
		t.Error("idempotent sweep must not touch the kept stint")
	}
}

func TestDiscardCause(t *testing.T) {
	const minDur = 2 * 1e9 // 2s in ns, as time.Duration
	cases := []struct {
		name       string
		durNS      int64
		cat        stintCategory
		carOrdinal int32
		want       string
	}{
		{"keeps a real race stint", 30e9, categoryRace, 1651, ""},
		{"keeps a real freeroam stint", 30e9, categoryFreeroam, 1651, ""},
		{"sub-min wins over everything", 1e9, categoryIdle, 0, "sub-min duration"},
		{"idle discarded", 30e9, categoryIdle, 1651, "idle"},
		{"no car discarded", 30e9, categoryRace, 0, "no car"},
		{"idle outranks no-car", 30e9, categoryIdle, 0, "idle"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := discardCause(time.Duration(c.durNS), time.Duration(minDur), c.cat, c.carOrdinal)
			if got != c.want {
				t.Errorf("discardCause = %q, want %q", got, c.want)
			}
		})
	}
}
