package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
