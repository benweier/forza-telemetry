package api

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

// Path columns are fixed — the endpoint exists to drive 3D path rendering.
// Keeping it narrow avoids cgo-DuckDB-Scan() blow-up for tight loops and
// keeps the JSON payload small.
var pathColumns = []string{"server_recv_ns", "pos_x", "pos_y", "pos_z", "speed_ms"}

// defaultPathStep — Forza ships ~60Hz over Data Out. step=6 leaves ~10Hz,
// roughly 5–8m sample spacing at typical race speeds. Tuned for visual
// smoothness on tight corners without ballooning the payload.
const (
	defaultPathStep = 6
	maxPathStep     = 60
)

func (s *Server) handleListPath(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var parquetPath string
	err := s.store.DB().QueryRow(
		`SELECT parquet_path FROM stints WHERE id = ?`, id,
	).Scan(&parquetPath)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "stint not found")
		return
	}
	if err != nil {
		s.internalError(w, "list_path lookup", err)
		return
	}

	step, err := parsePathStep(r.URL.Query().Get("step"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// ROW_NUMBER() is 1-based; MOD step picks every Nth row. The outer
	// SELECT projects the fixed column list to keep the response shape
	// stable regardless of parquet column order.
	q := fmt.Sprintf(`
		SELECT server_recv_ns, pos_x, pos_y, pos_z, speed_ms
		FROM (
			SELECT *, ROW_NUMBER() OVER (ORDER BY server_recv_ns) AS rn
			FROM read_parquet('%s')
		)
		WHERE (rn - 1) %% ? = 0
		ORDER BY server_recv_ns
	`, escapeSQLLiteral(parquetPath))
	rows, err := s.store.DB().Query(q, step)
	if err != nil {
		s.internalError(w, "list_path query", err)
		return
	}
	defer rows.Close()

	rowsOut := [][]any{}
	for rows.Next() {
		dest := make([]any, len(pathColumns))
		ptrs := make([]any, len(pathColumns))
		for i := range dest {
			ptrs[i] = &dest[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			s.internalError(w, "list_path scan", err)
			return
		}
		rowsOut = append(rowsOut, dest)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"columns":   pathColumns,
		"rows":      rowsOut,
		"step":      step,
		"sample_hz": 60.0 / float64(step),
	})
}

func parsePathStep(raw string) (int, error) {
	if raw == "" {
		return defaultPathStep, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("bad step: %v", err)
	}
	if n < 1 || n > maxPathStep {
		return 0, fmt.Errorf("step must be 1..%d", maxPathStep)
	}
	return n, nil
}
