package storage

import (
	"context"
	"io"
	"log/slog"
	"math"
	"os"
	"testing"
	"time"

	"github.com/parquet-go/parquet-go"

	"github.com/benweier/forza-telemetry/server/internal/stream"
	"github.com/benweier/forza-telemetry/server/internal/tick"
)

type stintRow struct {
	id        string
	ordinal   int
	tickCount int64
	stintType *string
	path      string
}

func TestWriterGapSplit(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())

	writer, sub, done, cancel := startWriter(t, store)
	defer cancel()

	base := int64(0)
	// Stint 1: 5 ticks across 2.5s.
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{}))
	}
	// 15s gap (> 10s threshold) → split.
	base += int64(15 * time.Second)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{}))
	}

	waitForStints(t, store, writer.sessionID, 2)
	cancel()
	sub.Close()
	_ = <-done

	stints := readStints(t, store, writer.sessionID)
	if len(stints) != 2 {
		t.Fatalf("want 2 stints, got %d", len(stints))
	}
	for i, s := range stints {
		if s.tickCount != 5 {
			t.Errorf("stint %d: tick_count want 5 got %d", i, s.tickCount)
		}
		if got := parquetRowCount(t, s.path); got != 5 {
			t.Errorf("stint %d: parquet rows want 5 got %d", i, got)
		}
	}
}

func TestWriterTypeSplit(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())

	writer, sub, done, cancel := startWriter(t, store)
	defer cancel()

	// 5 idle ticks (IsRaceOn=false) over 2.5s, then 5 freeroam ticks over 2.5s,
	// adjacent (no gap). Type change must split into 2 stints.
	base := int64(0)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{isRaceOn: false}))
	}
	base += 5 * int64(500*time.Millisecond)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{isRaceOn: true}))
	}

	waitForStints(t, store, writer.sessionID, 2)
	cancel()
	sub.Close()
	_ = <-done

	stints := readStints(t, store, writer.sessionID)
	if len(stints) != 2 {
		t.Fatalf("want 2 stints, got %d", len(stints))
	}
	wantTypes := []string{"idle", "freeroam"}
	for i, s := range stints {
		if s.stintType == nil || *s.stintType != wantTypes[i] {
			t.Errorf("stint %d: type want %q got %v", i, wantTypes[i], s.stintType)
		}
	}
}

func TestWriterCarSplit(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())

	writer, sub, done, cancel := startWriter(t, store)
	defer cancel()

	base := int64(0)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{carOrdinal: 100}))
	}
	base += 5 * int64(500*time.Millisecond)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{carOrdinal: 200}))
	}

	waitForStints(t, store, writer.sessionID, 2)
	cancel()
	sub.Close()
	_ = <-done

	stints := readStints(t, store, writer.sessionID)
	if len(stints) != 2 {
		t.Fatalf("want 2 stints, got %d", len(stints))
	}
}

func TestWriterShortStintDiscarded(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())

	writer, sub, done, cancel := startWriter(t, store)
	defer cancel()

	// 1 tick — way under 2s threshold.
	writer.broker.Publish(makeTick(0, tickOpts{}))

	// Allow goroutine to process before shutdown closes the stint.
	time.Sleep(50 * time.Millisecond)

	cancel()
	sub.Close()
	_ = <-done

	var n int
	if err := store.db.QueryRow(
		`SELECT COUNT(*) FROM stints WHERE session_id = ?`, writer.sessionID,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("short stint must be discarded, got %d rows", n)
	}
	// Parquet file should be removed too.
	path := store.HotPath(writer.sessionID, "0001")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("short-stint parquet should be removed, stat err: %v", err)
	}
}

func TestWriterResolvesCircuitVsSprint(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())

	writer, sub, done, cancel := startWriter(t, store)
	defer cancel()

	// Race stint A: LapNumber stays at 0 → sprint.
	base := int64(0)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{
			isRaceOn:        true,
			currentRaceTime: 10,
			lapNumber:       0,
			carOrdinal:      100,
		}))
	}
	// 15s gap → split.
	base += int64(15 * time.Second)
	// Race stint B: LapNumber 0→1→2 → circuit.
	for i := 0; i < 6; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{
			isRaceOn:        true,
			currentRaceTime: 10,
			lapNumber:       uint16(i / 2),
			carOrdinal:      100,
		}))
	}

	waitForStints(t, store, writer.sessionID, 2)
	cancel()
	sub.Close()
	_ = <-done

	stints := readStints(t, store, writer.sessionID)
	if len(stints) != 2 {
		t.Fatalf("want 2 stints, got %d", len(stints))
	}
	wantTypes := []string{"sprint", "circuit"}
	for i, s := range stints {
		if s.stintType == nil || *s.stintType != wantTypes[i] {
			t.Errorf("stint %d: type want %q got %v", i, wantTypes[i], s.stintType)
		}
	}
}

func TestWriterEmitsHotSpots(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())

	writer, sub, done, cancel := startWriter(t, store)
	defer cancel()

	// Build a freeroam stint with three identifiable peaks:
	//   - lateral G spike 1.2 (peak_lateral_g)
	//   - brake spike 0.8 pct (peak_brake)
	//   - top speed run 42 m/s (top_speed)
	// All inside a single >2s stint (sub-2s would be discarded).
	step := int64(100 * time.Millisecond)
	type sample struct{ lat, brake, speed float32 }
	stream := []sample{
		{0.1, 0.0, 10},
		{0.1, 0.0, 10},
		{0.1, 0.0, 10},
		// Lateral G peak (rises above 0.7, peaks at 1.2, falls below 0.5)
		{0.8, 0.0, 10},
		{1.2, 0.0, 10},
		{0.6, 0.0, 10},
		{0.2, 0.0, 10},
		// Brake event (BrakePct rises above 0.5, peaks 0.8, falls below 0.3)
		{0.0, 0.6, 10},
		{0.0, 0.8, 10},
		{0.0, 0.4, 10},
		{0.0, 0.1, 10},
		// Top-speed run (Speed rises above 30, peaks 42, falls below 25)
		{0.0, 0.0, 32},
		{0.0, 0.0, 38},
		{0.0, 0.0, 42},
		{0.0, 0.0, 35},
		{0.0, 0.0, 28},
		{0.0, 0.0, 20},
		// Cool-down
		{0.0, 0.0, 10},
		{0.0, 0.0, 10},
		{0.0, 0.0, 10},
		{0.0, 0.0, 10},
		{0.0, 0.0, 10},
	}
	base := int64(0)
	for i, s := range stream {
		writer.broker.Publish(makeTick(base+int64(i)*step, tickOpts{
			isRaceOn:   true,
			lateralG:   s.lat,
			brakePct:   s.brake,
			speed:      s.speed,
			carOrdinal: 100,
		}))
	}

	waitForStints(t, store, writer.sessionID, 1)
	cancel()
	sub.Close()
	_ = <-done

	rows, err := store.db.Query(
		`SELECT type, peak_value, label FROM hot_spots
		 WHERE stint_id LIKE ? ORDER BY peak_tick_ns`,
		writer.sessionID+"%",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type seen struct {
		typ   string
		peak  float64
		label string
	}
	var got []seen
	for rows.Next() {
		var s seen
		if err := rows.Scan(&s.typ, &s.peak, &s.label); err != nil {
			t.Fatal(err)
		}
		got = append(got, s)
	}

	wantTypes := []string{"peak_lateral_g", "peak_brake", "top_speed"}
	if len(got) != 3 {
		t.Fatalf("want 3 hot-spots, got %d: %+v", len(got), got)
	}
	for i, w := range wantTypes {
		if got[i].typ != w {
			t.Errorf("hot-spot %d: type want %q got %q", i, w, got[i].typ)
		}
	}
	if got[0].peak < 1.19 || got[0].peak > 1.21 {
		t.Errorf("lat_g peak: want ~1.2 got %v", got[0].peak)
	}
	if got[1].peak < 0.79 || got[1].peak > 0.81 {
		t.Errorf("brake peak: want ~0.8 got %v", got[1].peak)
	}
	if got[2].peak < 41.9 || got[2].peak > 42.1 {
		t.Errorf("speed peak: want ~42 got %v", got[2].peak)
	}
}

func TestWriterEmitsTurnsForRaceStint(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())

	writer, sub, done, cancel := startWriter(t, store)
	defer cancel()

	// Drive a synthetic single-lap circuit stint: straight + right turn +
	// straight + left turn + straight, with LapNumber incrementing partway
	// through so the stint resolves to circuit (not sprint).
	const step = int64(50 * time.Millisecond)
	const turnRate = math.Pi / 2 / 60 // 90° over 60 ticks
	var (
		samples  []*tick.Tick
		x, z     float64
		heading  float64
		baseTime int64
	)
	emit := func(latG, longG float32, lap uint16) {
		samples = append(samples, &tick.Tick{
			ServerRecvNS:        baseTime,
			GameTSMillis:        uint32(baseTime / int64(time.Millisecond)),
			IsRaceOn:            true,
			CurrentRaceTime:     10,
			LapNumber:           lap,
			CarOrdinal:          100,
			CarClass:            4,
			CarPerformanceIndex: 600,
			PositionX:           float32(x),
			PositionZ:           float32(z),
			LateralG:            latG,
			LongitudinalG:       longG,
		})
		baseTime += step
	}
	advance := func() {
		x += math.Sin(heading)
		z += math.Cos(heading)
	}

	// 40 ticks straight (2s) — lap 0
	for i := 0; i < 40; i++ {
		emit(0, 0, 0)
		advance()
	}
	// 60 ticks right turn (3s), lat G 0.8, no longG
	for i := 0; i < 60; i++ {
		emit(0.8, 0, 0)
		heading += turnRate
		advance()
	}
	// Lap completes here → lap 1
	// 40 ticks straight (2s)
	for i := 0; i < 40; i++ {
		emit(0, 0, 1)
		advance()
	}
	// 60 ticks left turn (3s)
	for i := 0; i < 60; i++ {
		emit(-0.8, 0, 1)
		heading -= turnRate
		advance()
	}
	// 40 ticks cool-down (2s)
	for i := 0; i < 40; i++ {
		emit(0, 0, 1)
		advance()
	}

	for _, t := range samples {
		writer.broker.Publish(t)
	}

	waitForStints(t, store, writer.sessionID, 1)
	cancel()
	sub.Close()
	_ = <-done

	// Stint type should be circuit (lap incremented).
	var stintType string
	if err := store.db.QueryRow(
		`SELECT stint_type FROM stints WHERE session_id = ?`, writer.sessionID,
	).Scan(&stintType); err != nil {
		t.Fatal(err)
	}
	if stintType != "circuit" {
		t.Fatalf("stint_type: want circuit got %q", stintType)
	}

	rows, err := store.db.Query(
		`SELECT turn_index, direction, peak_delta_theta
		 FROM turns WHERE stint_id LIKE ?
		 ORDER BY turn_index`,
		writer.sessionID+"%",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type seen struct {
		idx        int
		dir        string
		deltaTheta float64
	}
	var got []seen
	for rows.Next() {
		var s seen
		if err := rows.Scan(&s.idx, &s.dir, &s.deltaTheta); err != nil {
			t.Fatal(err)
		}
		got = append(got, s)
	}

	// Two distinct turns along the stint: turn 1 (right), turn 2 (left).
	// Per ADR 0008 numbering is chronological along the Stint (not per-Lap).
	if len(got) != 2 {
		t.Fatalf("want 2 turns (one per direction-change), got %d: %+v", len(got), got)
	}
	if got[0].idx != 1 || got[1].idx != 2 {
		t.Errorf("turn indices: want 1,2 got %d,%d", got[0].idx, got[1].idx)
	}
	if got[0].dir == got[1].dir {
		t.Errorf("right then left turns must have opposite direction, got %s + %s",
			got[0].dir, got[1].dir)
	}
	// Δθ should have opposite signs.
	if got[0].deltaTheta*got[1].deltaTheta >= 0 {
		t.Errorf("turn Δθ must have opposite signs, got %v + %v",
			got[0].deltaTheta, got[1].deltaTheta)
	}

	// And the K+1 invariant: 2 turns → 3 straights, one row each.
	var straightCount int
	if err := store.db.QueryRow(
		`SELECT COUNT(*) FROM straights WHERE stint_id LIKE ?`,
		writer.sessionID+"%",
	).Scan(&straightCount); err != nil {
		t.Fatal(err)
	}
	if straightCount != 3 {
		t.Errorf("straights: want 3 (K+1 for K=2), got %d", straightCount)
	}
}

func TestWriterEmitsAggregates(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())

	writer, sub, done, cancel := startWriter(t, store)
	defer cancel()

	// Freeroam stint spanning ~4 seconds with varied metrics:
	// - peak speed 50 m/s, peak lateral G 1.1, peak brake 0.9
	// - one gear-shift event (Gear 3 -> 4 between consecutive ticks)
	const step = int64(50 * time.Millisecond) // 20 Hz for test brevity
	const total = 80                          // 80 * 50ms = 4s
	for i := 0; i < total; i++ {
		gear := uint8(3)
		if i >= total/2 {
			gear = 4
		}
		writer.broker.Publish(&tick.Tick{
			ServerRecvNS:        int64(i) * step,
			GameTSMillis:        uint32(int64(i) * step / int64(time.Millisecond)),
			IsRaceOn:            true,
			CurrentRaceTime:     0, // freeroam
			CarOrdinal:          100,
			CarClass:            4,
			CarPerformanceIndex: 600,
			Gear:                gear,
			Speed:               10 + float32(i)/2,                 // grows 10 -> 50
			DistanceTraveled:    float32(i) * 1.0,                  // grows 0 -> 80
			LateralG:            float32(0.4 + float64(i)/float64(total-1)*0.7), // 0.4 -> 1.1
			LongitudinalG:       0.2,
			BrakePct:            float32(i) / float32(total) * 0.9, // 0 -> ~0.9
			EngineRPM:           5000 + float32(i)*20,              // 5000 -> 6580
		})
	}
	// Need to manually compute GearShift since Enrich isn't called when we
	// publish raw Ticks in the test (it runs in the listener, not the broker).
	// Set GearShift true on the transition tick.
	// (Skipped — the integration test only asserts on fields not requiring Enrich.)

	waitForStints(t, store, writer.sessionID, 1)
	cancel()
	sub.Close()
	_ = <-done

	stintID := writer.sessionID + "_0001"

	// stint_summary
	var topSpeed, peakLatG, peakBrake float64
	if err := store.db.QueryRow(
		`SELECT top_speed_ms, peak_lateral_g, peak_brake_pct
		 FROM stint_summary WHERE stint_id = ?`, stintID,
	).Scan(&topSpeed, &peakLatG, &peakBrake); err != nil {
		t.Fatalf("stint_summary: %v", err)
	}
	if topSpeed < 49.4 || topSpeed > 50.1 {
		t.Errorf("top_speed_ms: want ~49.5 got %v", topSpeed)
	}
	if peakLatG < 1.09 || peakLatG > 1.11 {
		t.Errorf("peak_lateral_g: want ~1.1 got %v", peakLatG)
	}
	if peakBrake < 0.88 || peakBrake > 0.90 {
		t.Errorf("peak_brake_pct: want ~0.89 got %v", peakBrake)
	}

	// preview_samples: 4s of data → 4 or 5 buckets (depending on ns rounding)
	var previewCount int
	if err := store.db.QueryRow(
		`SELECT COUNT(*) FROM preview_samples WHERE stint_id = ?`, stintID,
	).Scan(&previewCount); err != nil {
		t.Fatal(err)
	}
	if previewCount < 4 || previewCount > 5 {
		t.Errorf("preview_samples count: want 4-5 got %d", previewCount)
	}

	// Second indices must be 0..N-1 monotonically.
	rows, err := store.db.Query(
		`SELECT second_index FROM preview_samples WHERE stint_id = ? ORDER BY second_index`, stintID,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	prev := -1
	for rows.Next() {
		var idx int
		if err := rows.Scan(&idx); err != nil {
			t.Fatal(err)
		}
		if idx != prev+1 {
			t.Errorf("second_index not contiguous: prev=%d got=%d", prev, idx)
		}
		prev = idx
	}

	// lap_summary — freeroam stint has only lap 0
	var lapCount int
	if err := store.db.QueryRow(
		`SELECT COUNT(*) FROM lap_summary WHERE stint_id = ?`, stintID,
	).Scan(&lapCount); err != nil {
		t.Fatal(err)
	}
	if lapCount != 1 {
		t.Errorf("lap_summary count for freeroam: want 1 got %d", lapCount)
	}
}

// --- helpers ---

type writerHandle struct {
	*Writer
	broker *stream.Broker
}

func startWriter(t *testing.T, store *Store) (*writerHandle, *stream.Subscription, chan error, context.CancelFunc) {
	t.Helper()
	w, err := store.NewWriter(time.Date(2026, 5, 24, 17, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	broker := stream.NewBroker(64)
	sub := broker.Subscribe(false)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx, sub) }()
	return &writerHandle{Writer: w, broker: broker}, sub, done, cancel
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := New(dir, logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return store
}

type tickOpts struct {
	isRaceOn        bool
	currentRaceTime float32
	lapNumber       uint16
	carOrdinal      int32
	lateralG        float32
	brakePct        float32
	speed           float32
	positionX       float32
	positionZ       float32
	longitudinalG   float32
}

func makeTick(serverRecvNS int64, o tickOpts) *tick.Tick {
	car := o.carOrdinal
	if car == 0 {
		car = 123
	}
	return &tick.Tick{
		ServerRecvNS:        serverRecvNS,
		GameTSMillis:        uint32(serverRecvNS / int64(time.Millisecond)),
		IsRaceOn:            o.isRaceOn,
		CurrentRaceTime:     o.currentRaceTime,
		LapNumber:           o.lapNumber,
		CarOrdinal:          car,
		CarClass:            4,
		CarPerformanceIndex: 600,
		LateralG:            o.lateralG,
		BrakePct:            o.brakePct,
		Speed:               o.speed,
		PositionX:           o.positionX,
		PositionZ:           o.positionZ,
		LongitudinalG:       o.longitudinalG,
	}
}

func waitForStints(t *testing.T, store *Store, sessionID string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		if err := store.db.QueryRow(
			`SELECT COUNT(*) FROM stints WHERE session_id = ?`, sessionID,
		).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %d stints", want)
}

func readStints(t *testing.T, store *Store, sessionID string) []stintRow {
	t.Helper()
	rows, err := store.db.Query(
		`SELECT id, ordinal, tick_count, stint_type, parquet_path
		 FROM stints WHERE session_id = ? ORDER BY ordinal`, sessionID,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []stintRow
	for rows.Next() {
		var s stintRow
		if err := rows.Scan(&s.id, &s.ordinal, &s.tickCount, &s.stintType, &s.path); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func parquetRowCount(t *testing.T, path string) int64 {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open parquet %s: %v", path, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	pf, err := parquet.OpenFile(f, info.Size())
	if err != nil {
		t.Fatalf("OpenFile parquet: %v", err)
	}
	return pf.NumRows()
}
