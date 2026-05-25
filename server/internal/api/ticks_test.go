package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/benweier/forza-telemetry/server/internal/config"
	"github.com/benweier/forza-telemetry/server/internal/storage"
	"github.com/benweier/forza-telemetry/server/internal/stream"
	"github.com/benweier/forza-telemetry/server/internal/tick"
)

// newRealTickServer spins up a real Writer, drives synthetic ticks into it,
// and returns the server + stint ID with one populated stint backed by a
// real Parquet file.
func newRealTickServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := storage.New(dir, logger)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}

	writer, err := store.NewWriter(time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	broker := stream.NewBroker(64)
	sub := broker.Subscribe(false)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- writer.Run(ctx, sub) }()

	// 4 seconds of synthetic freeroam ticks at 20Hz with monotonic speed.
	const step = int64(50 * time.Millisecond)
	for i := 0; i < 80; i++ {
		broker.Publish(&tick.Tick{
			ServerRecvNS: int64(i) * step,
			GameTSMillis: uint32(int64(i) * step / int64(time.Millisecond)),
			IsRaceOn:     true,
			CarOrdinal:   100,
			CarClass:     4,
			Speed:        10 + float32(i)/2,
			EngineRPM:    5000 + float32(i)*20,
			ThrottlePct:  0.6,
			BrakePct:     0.1,
		})
	}

	// Wait for stint row to materialise.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		_ = store.DB().QueryRow(
			`SELECT COUNT(*) FROM stints WHERE session_id = ?`, writer.SessionID(),
		).Scan(&n)
		if n >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	sub.Close()
	<-done

	t.Cleanup(func() { _ = store.Close(context.Background()) })

	stintID := fmt.Sprintf("%s_0001", writer.SessionID())
	return New(config.APIConfig{Addr: ":0"}, broker, store, logger), stintID
}

func TestRESTListTicksDefaults(t *testing.T) {
	s, stintID := newRealTickServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stints/"+stintID+"/ticks", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body)
	}
	var body struct {
		Columns []string `json:"columns"`
		Rows    [][]any  `json:"rows"`
		FromNS  int64    `json:"from_ns"`
		ToNS    int64    `json:"to_ns"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Columns[0] != "server_recv_ns" {
		t.Errorf("columns[0]: want server_recv_ns got %q", body.Columns[0])
	}
	if len(body.Rows) == 0 {
		t.Errorf("expected rows, got 0")
	}
	// Default channels should include speed_ms — verify it's in the column list.
	hasSpeed := false
	for _, c := range body.Columns {
		if c == "speed_ms" {
			hasSpeed = true
		}
	}
	if !hasSpeed {
		t.Errorf("default channels missing speed_ms: %v", body.Columns)
	}
}

func TestRESTListTicksChannelSelection(t *testing.T) {
	s, stintID := newRealTickServer(t)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/stints/"+stintID+"/ticks?channels=speed_ms,engine_rpm", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body)
	}
	var body struct {
		Columns []string `json:"columns"`
	}
	json.NewDecoder(rec.Body).Decode(&body)
	want := []string{"server_recv_ns", "speed_ms", "engine_rpm"}
	if fmt.Sprint(body.Columns) != fmt.Sprint(want) {
		t.Errorf("columns: want %v got %v", want, body.Columns)
	}
}

func TestRESTListTicksRejectsBadChannel(t *testing.T) {
	s, stintID := newRealTickServer(t)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/stints/"+stintID+"/ticks?channels=speed_ms,nope", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: want 400 got %d body=%s", rec.Code, rec.Body)
	}
}

func TestRESTListTicksRejectsHugeWindow(t *testing.T) {
	s, stintID := newRealTickServer(t)
	huge := fmt.Sprintf("?from=0&to=%d", maxTickWindow.Nanoseconds()+1)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stints/"+stintID+"/ticks"+huge, nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: want 400 got %d body=%s", rec.Code, rec.Body)
	}
}

func TestRESTListTicks404(t *testing.T) {
	s, _ := newRealTickServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stints/does-not-exist/ticks", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Errorf("status: want 404 got %d", rec.Code)
	}
}
