package storage

import (
	"context"
	"errors"
	"testing"
)

// seedClosedStint inserts a session + one closed (deletable) stint.
func seedClosedStint(t *testing.T, store *Store) (sessionID, stintID string) {
	t.Helper()
	sessionID, stintID = "20260101T000000Z", "20260101T000000Z_0001"
	db := store.DB()
	if _, err := db.Exec(
		`INSERT INTO sessions (id, started_at_ns, ended_at_ns) VALUES (?, 1, 2)`, sessionID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO stints (id, session_id, ordinal, started_at_ns, ended_at_ns, tick_count, parquet_path)
		 VALUES (?, ?, 1, 1, 2, 5, '')`, stintID, sessionID,
	); err != nil {
		t.Fatal(err)
	}
	return sessionID, stintID
}

// TestDropLegacyTablesUnblocksDelete reproduces the production failure: a DB
// carried over from an older build still has a `turns` table whose foreign key
// onto stints blocks deletion. dropLegacyTables must clear it so deletes work.
func TestDropLegacyTablesUnblocksDelete(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())
	_, stintID := seedClosedStint(t, store)
	db := store.DB()

	// Re-create the legacy table + a referencing row, mimicking an old DB.
	if _, err := db.Exec(`CREATE TABLE turns (
		id       TEXT PRIMARY KEY,
		stint_id TEXT NOT NULL REFERENCES stints(id)
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO turns (id, stint_id) VALUES (?, ?)`, stintID+"_t1", stintID); err != nil {
		t.Fatal(err)
	}

	// The lingering FK blocks the stint delete.
	if err := store.DeleteStint(stintID); err == nil {
		t.Fatal("expected FK constraint to block delete before legacy cleanup")
	}

	if err := dropLegacyTables(db, store.logger); err != nil {
		t.Fatalf("dropLegacyTables: %v", err)
	}

	// Now it succeeds.
	if err := store.DeleteStint(stintID); err != nil {
		t.Fatalf("delete after legacy cleanup: %v", err)
	}
}

// TestDeleteSessionCascades verifies a session delete removes its stints + child
// rows, and that re-deleting a missing session reports ErrNotFound.
func TestDeleteSessionCascades(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())
	sessionID, stintID := seedClosedStint(t, store)
	db := store.DB()
	if _, err := db.Exec(
		`INSERT INTO preview_samples (stint_id, second_index, tick_ns) VALUES (?, 0, 1)`, stintID,
	); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteSession(sessionID); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM stints WHERE session_id = ?`, sessionID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("stints remain: %d", n)
	}
	if err := store.DeleteSession(sessionID); !errors.Is(err, ErrNotFound) {
		t.Errorf("re-delete: want ErrNotFound got %v", err)
	}
}
