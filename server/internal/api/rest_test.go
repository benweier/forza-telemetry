package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/benweier/forza-telemetry/server/internal/config"
	"github.com/benweier/forza-telemetry/server/internal/storage"
	"github.com/benweier/forza-telemetry/server/internal/stream"
)

const (
	fixSessionID = "20260524T170000Z"
	fixStintID   = "20260524T170000Z_0001"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := storage.New(dir, logger)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close(context.Background()) })

	db := store.DB()
	mustExec(t, db, `INSERT INTO sessions (id, started_at_ns, ended_at_ns) VALUES (?, ?, ?)`,
		fixSessionID, int64(1_000_000_000), int64(2_000_000_000))
	mustExec(t, db, `INSERT INTO stints (id, session_id, ordinal, started_at_ns, ended_at_ns,
	                                     first_game_ts_ms, last_game_ts_ms, tick_count, stint_type,
	                                     car_ordinal, car_class, car_pi, parquet_path)
	                  VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		fixStintID, fixSessionID, 1, int64(1_100_000_000), int64(1_900_000_000),
		uint32(0), uint32(800), int64(48), "circuit",
		int32(100), int32(4), int32(600), "/tmp/fake.parquet")
	mustExec(t, db, `INSERT INTO stint_summary
	                 (stint_id, top_speed_ms, distance_m, avg_speed_ms, max_rpm,
	                  peak_lateral_g, peak_long_g, peak_brake_pct, gear_shift_count)
	                 VALUES (?, 50.0, 1500.0, 25.0, 6500.0, 1.2, 0.6, 0.85, 3)`,
		fixStintID)
	mustExec(t, db, `INSERT INTO lap_summary (id, stint_id, lap_number, lap_time_s, top_speed_ms,
	                                          distance_m, peak_lateral_g, peak_brake_pct)
	                 VALUES (?, ?, 0, 78.5, 50.0, 1500.0, 1.2, 0.85)`,
		fixStintID+"_lap_0", fixStintID)
	mustExec(t, db, `INSERT INTO preview_samples
	                 (stint_id, second_index, tick_ns, speed_ms, lateral_g, longitudinal_g,
	                  throttle_pct, brake_pct, rpm, pos_x, pos_y, pos_z, lap_number)
	                 VALUES (?, 0, 1100000000, 25.0, 0.5, 0.2, 0.6, 0.0, 5500.0, 100.0, 12.5, 200.0, 0)`,
		fixStintID)

	broker := stream.NewBroker(8)
	return New(config.APIConfig{Addr: ":0"}, broker, store, logger)
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func doRequest(t *testing.T, s *Server, path string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response from %s: %v", path, err)
	}
	return rec.Code, body
}

func TestRESTListSessions(t *testing.T) {
	s := newTestServer(t)
	code, body := doRequest(t, s, "/api/v1/sessions")
	if code != 200 {
		t.Fatalf("status %d: %v", code, body)
	}
	if body["total"].(float64) != 1 {
		t.Errorf("total: want 1 got %v", body["total"])
	}
	sessions := body["sessions"].([]any)
	if len(sessions) != 1 {
		t.Fatalf("sessions len: want 1 got %d", len(sessions))
	}
	first := sessions[0].(map[string]any)
	if first["id"] != fixSessionID {
		t.Errorf("session id: want %q got %v", fixSessionID, first["id"])
	}
	if first["stint_count"].(float64) != 1 {
		t.Errorf("stint_count: want 1 got %v", first["stint_count"])
	}
}

func TestRESTGetSession(t *testing.T) {
	s := newTestServer(t)
	code, body := doRequest(t, s, "/api/v1/sessions/"+fixSessionID)
	if code != 200 {
		t.Fatalf("status %d: %v", code, body)
	}
	if body["id"] != fixSessionID {
		t.Errorf("id: want %q got %v", fixSessionID, body["id"])
	}
	stints := body["stints"].([]any)
	if len(stints) != 1 {
		t.Fatalf("stints len: want 1 got %d", len(stints))
	}
}

func TestRESTGetSession404(t *testing.T) {
	s := newTestServer(t)
	code, body := doRequest(t, s, "/api/v1/sessions/does-not-exist")
	if code != 404 {
		t.Fatalf("status: want 404 got %d body=%v", code, body)
	}
}

func TestRESTGetStint(t *testing.T) {
	s := newTestServer(t)
	code, body := doRequest(t, s, "/api/v1/stints/"+fixStintID)
	if code != 200 {
		t.Fatalf("status %d: %v", code, body)
	}
	if body["stint_type"] != "circuit" {
		t.Errorf("stint_type: want circuit got %v", body["stint_type"])
	}
	car := body["car"].(map[string]any)
	if car["ordinal"].(float64) != 100 {
		t.Errorf("car.ordinal: want 100 got %v", car["ordinal"])
	}
	sum := body["summary"].(map[string]any)
	if sum["top_speed_ms"].(float64) != 50 {
		t.Errorf("summary.top_speed_ms: want 50 got %v", sum["top_speed_ms"])
	}
	if _, present := body["parquet_path"]; present {
		t.Errorf("parquet_path must not be serialised to clients")
	}
}

func TestRESTPatchSessionPin(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/sessions/"+fixSessionID,
		strings.NewReader(`{"pinned": true}`))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body)
	}
	var body map[string]any
	json.NewDecoder(rec.Body).Decode(&body)
	if body["pinned"] != true {
		t.Errorf("pinned: want true got %v", body["pinned"])
	}
}

func TestRESTPatchSessionEmptyBody(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/sessions/"+fixSessionID,
		strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: want 400 got %d body=%s", rec.Code, rec.Body)
	}
}

func TestRESTPatchSession404(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/sessions/nope",
		strings.NewReader(`{"pinned": true}`))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status: want 404 got %d", rec.Code)
	}
}

func TestRESTDownsampleSession501(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+fixSessionID+"/downsample", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status: want 501 got %d body=%s", rec.Code, rec.Body)
	}
}

func TestRESTDownsampleSession404(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/nope/downsample", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status: want 404 got %d", rec.Code)
	}
}

func TestRESTListLaps(t *testing.T) {
	s := newTestServer(t)
	code, body := doRequest(t, s, "/api/v1/stints/"+fixStintID+"/laps")
	if code != 200 {
		t.Fatal(code)
	}
	laps := body["laps"].([]any)
	if len(laps) != 1 {
		t.Fatalf("laps len: want 1 got %d", len(laps))
	}
}

func TestRESTListPreview(t *testing.T) {
	s := newTestServer(t)
	code, body := doRequest(t, s, "/api/v1/stints/"+fixStintID+"/preview")
	if code != 200 {
		t.Fatal(code)
	}
	samples := body["samples"].([]any)
	if len(samples) != 1 {
		t.Fatalf("samples len: want 1 got %d", len(samples))
	}
}

func doMethod(t *testing.T, s *Server, method, path string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec.Code
}

func countRows(t *testing.T, s *Server, query string, args ...any) int {
	t.Helper()
	var n int
	if err := s.store.DB().QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

func TestRESTDeleteStint(t *testing.T) {
	s := newTestServer(t)
	if code := doMethod(t, s, http.MethodDelete, "/api/v1/stints/"+fixStintID); code != 200 {
		t.Fatalf("delete stint: want 200 got %d", code)
	}
	if code := doMethod(t, s, http.MethodDelete, "/api/v1/stints/"+fixStintID); code != 404 {
		t.Errorf("re-delete: want 404 got %d", code)
	}
	if code, _ := doRequest(t, s, "/api/v1/stints/"+fixStintID); code != 404 {
		t.Errorf("get deleted stint: want 404 got %d", code)
	}
	// Child rows cascade away; the parent session survives.
	if n := countRows(t, s, `SELECT COUNT(*) FROM stints WHERE id = ?`, fixStintID); n != 0 {
		t.Errorf("stint rows: want 0 got %d", n)
	}
	for _, tbl := range []string{"stint_summary", "lap_summary", "preview_samples"} {
		if n := countRows(t, s, `SELECT COUNT(*) FROM `+tbl+` WHERE stint_id = ?`, fixStintID); n != 0 {
			t.Errorf("%s rows: want 0 got %d", tbl, n)
		}
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM sessions WHERE id = ?`, fixSessionID); n != 1 {
		t.Errorf("session should survive stint delete: got %d", n)
	}
}

func TestRESTDeleteSession(t *testing.T) {
	s := newTestServer(t)
	if code := doMethod(t, s, http.MethodDelete, "/api/v1/sessions/"+fixSessionID); code != 200 {
		t.Fatalf("delete session: want 200 got %d", code)
	}
	if code, _ := doRequest(t, s, "/api/v1/sessions/"+fixSessionID); code != 404 {
		t.Errorf("get deleted session: want 404 got %d", code)
	}
	// Everything beneath the session is gone.
	if n := countRows(t, s, `SELECT COUNT(*) FROM stints WHERE session_id = ?`, fixSessionID); n != 0 {
		t.Errorf("stints: want 0 got %d", n)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM preview_samples WHERE stint_id = ?`, fixStintID); n != 0 {
		t.Errorf("preview_samples: want 0 got %d", n)
	}
}

func TestRESTDeleteActiveRejected(t *testing.T) {
	s := newTestServer(t)
	db := s.store.DB()
	// A session + stint still recording (ended_at_ns IS NULL).
	mustExec(t, db, `INSERT INTO sessions (id, started_at_ns) VALUES (?, ?)`,
		"ACTIVE_SESSION", int64(3_000_000_000))
	mustExec(t, db, `INSERT INTO stints (id, session_id, ordinal, started_at_ns, parquet_path)
	                 VALUES (?, ?, 1, ?, ?)`,
		"ACTIVE_SESSION_0001", "ACTIVE_SESSION", int64(3_000_000_000), "/tmp/active.parquet")

	if code := doMethod(t, s, http.MethodDelete, "/api/v1/sessions/ACTIVE_SESSION"); code != 409 {
		t.Errorf("delete active session: want 409 got %d", code)
	}
	if code := doMethod(t, s, http.MethodDelete, "/api/v1/stints/ACTIVE_SESSION_0001"); code != 409 {
		t.Errorf("delete active stint: want 409 got %d", code)
	}
	// Both still present.
	if n := countRows(t, s, `SELECT COUNT(*) FROM sessions WHERE id = ?`, "ACTIVE_SESSION"); n != 1 {
		t.Errorf("active session should survive: got %d", n)
	}
}

func TestRESTDeleteMissing(t *testing.T) {
	s := newTestServer(t)
	if code := doMethod(t, s, http.MethodDelete, "/api/v1/sessions/nope"); code != 404 {
		t.Errorf("delete missing session: want 404 got %d", code)
	}
	if code := doMethod(t, s, http.MethodDelete, "/api/v1/stints/nope"); code != 404 {
		t.Errorf("delete missing stint: want 404 got %d", code)
	}
}

// The parquet footer only exists after stint close — tick/path reads on the
// actively-recording stint used to 500; they must 409 like the deletes.
func TestRESTTicksAndPathOnRecordingStint(t *testing.T) {
	s := newTestServer(t)
	db := s.store.DB()
	mustExec(t, db, `INSERT INTO sessions (id, started_at_ns) VALUES (?, ?)`,
		"REC_SESSION", int64(1_000_000_000))
	mustExec(t, db, `INSERT INTO stints (id, session_id, ordinal, started_at_ns, parquet_path)
	                 VALUES (?, ?, 1, ?, ?)`,
		"REC_SESSION_0001", "REC_SESSION", int64(1_000_000_000), "/tmp/recording.parquet")

	if code, _ := doRequest(t, s, "/api/v1/stints/REC_SESSION_0001/ticks"); code != 409 {
		t.Errorf("ticks on recording stint: want 409 got %d", code)
	}
	if code, _ := doRequest(t, s, "/api/v1/stints/REC_SESSION_0001/path"); code != 409 {
		t.Errorf("path on recording stint: want 409 got %d", code)
	}
}

// A `from` with no `to` must default to from+60s (docs/api.md), not
// stint_start+60s — the old anchoring rejected any from > start+60s.
func TestParseWindowDefaultToFollowsFrom(t *testing.T) {
	const sec = int64(1_000_000_000)
	end := nullableInt64{Value: 300 * sec, Valid: true}
	req := httptest.NewRequest(http.MethodGet, "/?from="+strconv.FormatInt(70*sec, 10), nil)

	from, to, err := parseWindow(req, 0, end)
	if err != nil {
		t.Fatalf("parseWindow: %v", err)
	}
	if from != 70*sec || to != 130*sec {
		t.Errorf("want [70s,130s], got [%d,%d]", from, to)
	}

	// Default `to` still clamps to the stint end.
	req = httptest.NewRequest(http.MethodGet, "/?from="+strconv.FormatInt(290*sec, 10), nil)
	if _, to, err = parseWindow(req, 0, end); err != nil || to != 300*sec {
		t.Errorf("want clamp to 300s, got to=%d err=%v", to, err)
	}
}
