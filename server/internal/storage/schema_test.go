package storage

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestMigrate(t *testing.T) {
	db, err := sql.Open("duckdb", filepath.Join(t.TempDir(), "test.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Idempotency — second run must not error.
	if err := migrate(db); err != nil {
		t.Fatalf("migrate twice: %v", err)
	}

	for _, table := range []string{
		"sessions", "stints",
		"stint_summary", "lap_summary", "preview_samples",
	} {
		var n int
		err := db.QueryRow(
			"SELECT count(*) FROM information_schema.tables WHERE table_name = ?",
			table,
		).Scan(&n)
		if err != nil {
			t.Fatalf("query %s: %v", table, err)
		}
		if n != 1 {
			t.Errorf("table %s: want 1 row in information_schema, got %d", table, n)
		}
	}
}
