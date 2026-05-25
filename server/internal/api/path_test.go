package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRESTListPathDefaults(t *testing.T) {
	s, stintID := newRealTickServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stints/"+stintID+"/path", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body)
	}
	var body struct {
		Columns  []string  `json:"columns"`
		Rows     [][]any   `json:"rows"`
		Step     int       `json:"step"`
		SampleHz float64   `json:"sample_hz"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	wantCols := []string{"server_recv_ns", "pos_x", "pos_y", "pos_z", "speed_ms"}
	if fmt.Sprint(body.Columns) != fmt.Sprint(wantCols) {
		t.Errorf("columns: want %v got %v", wantCols, body.Columns)
	}
	if body.Step != defaultPathStep {
		t.Errorf("step: want %d got %d", defaultPathStep, body.Step)
	}
	// 80 synthetic ticks @ step=6 → indices 0,6,12,...,78 = 14 rows.
	if len(body.Rows) < 10 || len(body.Rows) > 16 {
		t.Errorf("rows: want ~14 (80/step=6) got %d", len(body.Rows))
	}
}

func TestRESTListPathCustomStep(t *testing.T) {
	s, stintID := newRealTickServer(t)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/stints/"+stintID+"/path?step=1", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body)
	}
	var body struct {
		Rows [][]any `json:"rows"`
		Step int     `json:"step"`
	}
	json.NewDecoder(rec.Body).Decode(&body)
	if body.Step != 1 {
		t.Errorf("step: want 1 got %d", body.Step)
	}
	// step=1 → every row, all 80 ticks.
	if len(body.Rows) != 80 {
		t.Errorf("rows: want 80 (step=1) got %d", len(body.Rows))
	}
}

func TestRESTListPathRejectsBadStep(t *testing.T) {
	s, stintID := newRealTickServer(t)
	cases := []string{"0", "-1", "999", "abc"}
	for _, raw := range cases {
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/stints/"+stintID+"/path?step="+raw, nil)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("step=%q: want 400 got %d body=%s", raw, rec.Code, rec.Body)
		}
	}
}

func TestRESTListPath404(t *testing.T) {
	s, _ := newRealTickServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stints/does-not-exist/path", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Errorf("status: want 404 got %d", rec.Code)
	}
}
