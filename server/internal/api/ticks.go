package api

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// tickChannels maps a public channel name (JSON / query-string token) to the
// parquet column name. Anything outside this whitelist is rejected — the
// query goes straight into a SQL projection, and unconstrained columns would
// let callers exfiltrate or rename columns at will.
var tickChannels = map[string]string{
	"speed_ms":         "speed_ms",
	"engine_rpm":       "engine_rpm",
	"lateral_g":        "lateral_g",
	"longitudinal_g":   "longitudinal_g",
	"throttle_pct":     "throttle_pct",
	"brake_pct":        "brake_pct",
	"rpm_pct":          "rpm_pct",
	"gear":             "gear",
	"gear_shift":       "gear_shift",
	"steer":            "steer",
	"pos_x":            "pos_x",
	"pos_y":            "pos_y",
	"pos_z":            "pos_z",
	"distance_m":       "distance_m",
	"lap_distance_m":   "lap_distance_m",
	"lap_number":       "lap_number",
	"current_lap_s":    "current_lap_s",
	"current_race_s":   "current_race_s",
	"is_race_on":       "is_race_on",
	"boost":            "boost",
	"fuel":             "fuel",
	"smashable_mass":   "smashable_mass",
}

var defaultTickChannels = []string{
	"speed_ms", "engine_rpm", "throttle_pct", "brake_pct",
	"lateral_g", "longitudinal_g", "gear", "lap_number",
}

// maxTickWindow caps the requested time window. At 60Hz that is ~3600
// samples × N channels — small enough to ship over a single JSON response
// without paging.
const maxTickWindow = 60 * time.Second

func (s *Server) handleListTicks(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Need parquet_path for read_parquet(); stints metadata also confirms
	// the stint exists (→ 404 if not).
	var parquetPath string
	var stintStart, stintEnd nullableInt64
	err := s.store.DB().QueryRow(
		`SELECT parquet_path, started_at_ns, ended_at_ns FROM stints WHERE id = ?`, id,
	).Scan(&parquetPath, &stintStart.Value, &stintEnd)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "stint not found")
		return
	}
	if err != nil {
		s.internalError(w, "list_ticks lookup", err)
		return
	}
	stintStart.Valid = true

	from, to, err := parseWindow(r, stintStart.Value, stintEnd)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	channels, err := parseChannels(r.URL.Query().Get("channels"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Build the SELECT — server_recv_ns always first so callers can correlate.
	cols := append([]string{"server_recv_ns"}, channels...)
	selectList := strings.Join(cols, ", ")
	q := fmt.Sprintf(
		`SELECT %s FROM read_parquet('%s')
		 WHERE server_recv_ns BETWEEN ? AND ?
		 ORDER BY server_recv_ns`,
		selectList, escapeSQLLiteral(parquetPath),
	)
	rows, err := s.store.DB().Query(q, from, to)
	if err != nil {
		s.internalError(w, "list_ticks query", err)
		return
	}
	defer rows.Close()

	// Column-oriented response — far more compact than row-of-objects for
	// chart payloads, and what uPlot consumes natively.
	rowsOut := [][]any{}
	for rows.Next() {
		dest := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range dest {
			ptrs[i] = &dest[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			s.internalError(w, "list_ticks scan", err)
			return
		}
		rowsOut = append(rowsOut, dest)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"columns": cols,
		"rows":    rowsOut,
		"from_ns": from,
		"to_ns":   to,
	})
}

// parseWindow resolves the from/to query params against the stint's range.
// Missing params default to the stint start / start+maxTickWindow.
func parseWindow(r *http.Request, stintStartNS int64, stintEndNS nullableInt64) (int64, int64, error) {
	q := r.URL.Query()
	from := stintStartNS
	to := stintStartNS + maxTickWindow.Nanoseconds()
	if stintEndNS.Valid && to > stintEndNS.Value {
		to = stintEndNS.Value
	}
	if v := q.Get("from"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("bad from: %v", err)
		}
		from = n
	}
	if v := q.Get("to"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("bad to: %v", err)
		}
		to = n
	}
	if to <= from {
		return 0, 0, fmt.Errorf("to must be > from")
	}
	if to-from > maxTickWindow.Nanoseconds() {
		return 0, 0, fmt.Errorf("window exceeds max %v", maxTickWindow)
	}
	return from, to, nil
}

// escapeSQLLiteral escapes single quotes for safe SQL literal embedding.
// Mirrors storage.escapeSQLLiteral; duplicated to avoid exporting an
// internal helper from the storage package.
func escapeSQLLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func parseChannels(raw string) ([]string, error) {
	if raw == "" {
		return defaultTickChannels, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		name := strings.TrimSpace(p)
		if name == "" {
			continue
		}
		if _, ok := tickChannels[name]; !ok {
			return nil, fmt.Errorf("unknown channel %q", name)
		}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no channels selected")
	}
	return out, nil
}
