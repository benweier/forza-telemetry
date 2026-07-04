package storage

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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
	// Stint 1: 5 freeroam ticks across 2.5s. Freeroam (not idle) + a real car
	// so the stint is persisted under the idle/no-car discard policy.
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{isRaceOn: true}))
	}
	// 15s gap (> 10s threshold) → split.
	base += int64(15 * time.Minute)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{isRaceOn: true}))
	}

	waitForStints(t, store, 2)
	cancel()
	sub.Close()
	_ = <-done

	stints := readStints(t, store)
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

// ADR 0013: CurrentRaceTime no longer splits — a race entered without an
// IsRaceOn flip stays in the same stint, which classifies by "did it ever
// race" (sawRace) at close.
func TestWriterRaceTransitionDoesNotSplit(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())

	writer, sub, done, cancel := startWriter(t, store)
	defer cancel()

	// 5 freeroam ticks then 5 race ticks, adjacent, IsRaceOn true throughout:
	// one stint, classified sprint because it saw race time.
	base := int64(0)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{isRaceOn: true}))
	}
	base += 5 * int64(500*time.Millisecond)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{isRaceOn: true, currentRaceTime: 10}))
	}

	waitForStints(t, store, 1)
	cancel()
	sub.Close()
	_ = <-done

	stints := readStints(t, store)
	if len(stints) != 1 {
		t.Fatalf("race transition must not split: want 1 stint, got %d", len(stints))
	}
	if stints[0].tickCount != 10 {
		t.Errorf("tick_count want 10 got %d", stints[0].tickCount)
	}
	if stints[0].stintType == nil || *stints[0].stintType != "sprint" {
		t.Errorf("type want sprint got %v", stints[0].stintType)
	}
}

// ADR 0013: an IsRaceOn flip IS a split — driving → menus → driving yields two
// persisted driving stints (the idle middle is discarded).
func TestWriterRaceStateSplit(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())

	writer, sub, done, cancel := startWriter(t, store)
	defer cancel()

	base := int64(0)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{isRaceOn: true}))
	}
	base += 5 * int64(500*time.Millisecond)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{isRaceOn: false}))
	}
	base += 5 * int64(500*time.Millisecond)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{isRaceOn: true}))
	}

	waitForStints(t, store, 2)
	cancel()
	sub.Close()
	_ = <-done

	stints := readStints(t, store)
	if len(stints) != 2 {
		t.Fatalf("want 2 driving stints (idle middle discarded), got %d", len(stints))
	}
	for i, s := range stints {
		if s.stintType == nil || *s.stintType != "freeroam" {
			t.Errorf("stint %d: type want freeroam got %v", i, s.stintType)
		}
	}
}

func TestWriterDiscardsIdleAndNoCar(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())

	writer, sub, done, cancel := startWriter(t, store)
	defer cancel()

	// Block A: idle (IsRaceOn=false) with a real car — discarded for being idle.
	base := int64(0)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{isRaceOn: false, carOrdinal: 100}))
	}
	// Block B: 15s gap then freeroam with no car (CarOrdinal 0) — discarded for
	// having no car. makeTick defaults a zero ordinal to 123, so pass it through
	// a tickOpts sentinel of -1 mapped to 0 below.
	base += int64(15 * time.Minute)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{isRaceOn: true, carOrdinal: noCarSentinel}))
	}

	time.Sleep(100 * time.Millisecond)
	cancel()
	sub.Close()
	_ = <-done

	var n int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM stints`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("idle and no-car stints must not persist, got %d rows", n)
	}
}

func TestWriterDiscardsThinStint(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())

	writer, sub, done, cancel := startWriter(t, store)
	defer cancel()
	// Restore the production density floor (startWriter zeroes it). The channel
	// send below happens-before the writer goroutine reads minTicks at close.
	writer.minTicks = 180

	// 10 race ticks spread across 3s — over the 2s duration floor, but far
	// under 180 ticks. Must be discarded for thin data.
	base := int64(0)
	for i := 0; i < 10; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(300*time.Millisecond), tickOpts{
			isRaceOn:        true,
			currentRaceTime: 10,
			carOrdinal:      100,
		}))
	}

	time.Sleep(100 * time.Millisecond)
	cancel()
	sub.Close()
	_ = <-done

	var n int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM stints`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("thin stint (<180 ticks) must be discarded, got %d rows", n)
	}
}

func TestWriterCarSplit(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())

	writer, sub, done, cancel := startWriter(t, store)
	defer cancel()

	// Race ticks (persisted) so the car-change split is observable as 2 rows.
	base := int64(0)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{isRaceOn: true, currentRaceTime: 10, carOrdinal: 100}))
	}
	base += 5 * int64(500*time.Millisecond)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{isRaceOn: true, currentRaceTime: 10, carOrdinal: 200}))
	}

	waitForStints(t, store, 2)
	cancel()
	sub.Close()
	_ = <-done

	stints := readStints(t, store)
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
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM stints`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("short stint must be discarded, got %d rows", n)
	}
	// Parquet segments should be removed too — no stint dirs anywhere under hot/.
	entries, err := os.ReadDir(filepath.Join(store.dataDir, "parquet", "hot"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, e := range entries {
		t.Errorf("hot dir should be empty after discard, found %s", e.Name())
	}
	// And the session that held only the discarded stint is deleted (ADR 0012).
	var sessions int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 {
		t.Errorf("session with no surviving stints must be discarded, got %d rows", sessions)
	}
}

// Rotation (ADR 0011): the writer must close a durable segment every
// rotateEvery of tick time and continue in a fresh one, with the stored glob
// covering all of them.
func TestWriterRotatesSegments(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())

	writer, sub, done, cancel := startWriter(t, store)
	defer cancel()
	writer.rotateEvery = time.Second

	// 10 race ticks over 4.5s at 500ms spacing → rotations at the 1s marks.
	base := int64(0)
	for i := 0; i < 10; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{
			isRaceOn:        true,
			currentRaceTime: 10,
			carOrdinal:      100,
		}))
	}
	// A gap tick forces stint 1 to CLOSE before we assert — waiting on the row
	// alone raced tick consumption (the row exists from the first tick, so a
	// slow consumer could be cancelled mid-stream; -race caught this at 7/10).
	// Stint 2 is a 1-tick throwaway that shutdown discards as sub-minimum.
	writer.broker.Publish(makeTick(base+int64(15*time.Minute), tickOpts{
		isRaceOn:        true,
		currentRaceTime: 10,
		carOrdinal:      100,
	}))

	waitForStints(t, store, 2)
	cancel()
	sub.Close()
	_ = <-done

	stints := readStints(t, store)
	if len(stints) != 1 {
		t.Fatalf("want 1 surviving stint (throwaway discarded), got %d", len(stints))
	}
	if stints[0].tickCount != 10 {
		t.Errorf("tick_count want 10 got %d", stints[0].tickCount)
	}
	segs, err := filepath.Glob(stints[0].path)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) < 3 {
		t.Errorf("want >=3 rotated segments, got %d (%v)", len(segs), segs)
	}
	// Every segment must be individually complete (readable footer), and the
	// glob must cover every tick exactly once.
	var total int64
	for _, seg := range segs {
		total += singleParquetRowCount(t, seg)
	}
	if total != 10 {
		t.Errorf("rows across segments want 10 got %d", total)
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
	base += int64(15 * time.Minute)
	// Race stint B: LapNumber 0→1→2 → circuit.
	for i := 0; i < 6; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{
			isRaceOn:        true,
			currentRaceTime: 10,
			lapNumber:       uint16(i / 2),
			carOrdinal:      100,
		}))
	}

	waitForStints(t, store, 2)
	cancel()
	sub.Close()
	_ = <-done

	stints := readStints(t, store)
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
			Speed:               10 + float32(i)/2,                              // grows 10 -> 50
			DistanceTraveled:    float32(i) * 1.0,                               // grows 0 -> 80
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

	// A throwaway gap tick forces the stint to CLOSE deterministically before
	// asserting (waiting on the row alone races tick consumption); shutdown
	// discards the 1-tick follow-up as sub-minimum.
	writer.broker.Publish(makeTick(int64(total)*step+int64(15*time.Minute), tickOpts{isRaceOn: true, carOrdinal: 100}))

	waitForStints(t, store, 2)
	cancel()
	sub.Close()
	_ = <-done

	stints := readStints(t, store)
	if len(stints) != 1 {
		t.Fatalf("want 1 surviving stint, got %d", len(stints))
	}
	stintID := stints[0].id

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
	w := store.NewWriter()
	// These tests exercise splitting / typing / aggregation with small synthetic
	// stints (a handful of ticks). Disable the production 180-tick density floor
	// so those stints persist; TestWriterDiscardsThinStint covers the floor.
	w.minTicks = 0
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
	// gameTS overrides the derived GameTSMillis (session game-restart tests
	// need the game clock decoupled from arrival time). 0 → derived.
	gameTS uint32
}

// noCarSentinel lets a test request a genuine CarOrdinal of 0 (makeTick
// otherwise defaults an unset ordinal to a real car for convenience).
const noCarSentinel int32 = -1

func makeTick(serverRecvNS int64, o tickOpts) *tick.Tick {
	car := o.carOrdinal
	switch car {
	case 0:
		car = 123
	case noCarSentinel:
		car = 0
	}
	gameTS := o.gameTS
	if gameTS == 0 {
		gameTS = uint32(serverRecvNS / int64(time.Millisecond))
	}
	return &tick.Tick{
		ServerRecvNS:        serverRecvNS,
		GameTSMillis:        gameTS,
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

func waitForStints(t *testing.T, store *Store, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		if err := store.db.QueryRow(
			`SELECT COUNT(*) FROM stints`,
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

func readStints(t *testing.T, store *Store) []stintRow {
	t.Helper()
	rows, err := store.db.Query(
		`SELECT id, ordinal, tick_count, stint_type, parquet_path
		 FROM stints ORDER BY ordinal`,
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

// parquetRowCount sums rows across a stint's segment glob (or a single file
// for legacy paths).
func parquetRowCount(t *testing.T, path string) int64 {
	t.Helper()
	files := []string{path}
	if strings.Contains(path, "*") {
		matches, err := filepath.Glob(path)
		if err != nil {
			t.Fatalf("glob %s: %v", path, err)
		}
		if len(matches) == 0 {
			t.Fatalf("glob %s matched no segments", path)
		}
		files = matches
	}
	var total int64
	for _, p := range files {
		total += singleParquetRowCount(t, p)
	}
	return total
}

func singleParquetRowCount(t *testing.T, path string) int64 {
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

// Aggregation must be OFF the consume loop: while a closed stint's aggregation
// is blocked, the writer must keep consuming ticks and open the next stint.
// And shutdown must wait for pending aggregations, so summaries always land
// before the store closes.
func TestWriterAggregationOffHotPath(t *testing.T) {
	block := make(chan struct{})
	defer func() {
		select {
		case <-block: // already released
		default:
			close(block) // failing path: unblock so shutdown's Wait can't hang
		}
	}()
	orig := aggregateStintFn
	aggregateStintFn = func(db *sql.DB, in stintAggregateInput) error {
		<-block
		return orig(db, in)
	}
	defer func() { aggregateStintFn = orig }()

	store := newTestStore(t)
	defer store.Close(context.Background())
	writer, sub, done, cancel := startWriter(t, store)
	defer cancel()

	// Stint A, then a 15s gap forcing A to close — its aggregation blocks.
	base := int64(0)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{isRaceOn: true}))
	}
	base += int64(15 * time.Minute)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{isRaceOn: true}))
	}

	// Stint B's row appearing while A's aggregation is still blocked is the
	// proof the hot path no longer waits on the heavy scans.
	waitForStints(t, store, 2)

	var summaries int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM stint_summary`).Scan(&summaries); err != nil {
		t.Fatal(err)
	}
	if summaries != 0 {
		t.Fatalf("aggregation ran on the hot path: %d summaries while blocked", summaries)
	}

	close(block)
	cancel()
	sub.Close()
	_ = <-done // Run returns only after shutdown's aggWG.Wait()

	if err := store.db.QueryRow(`SELECT COUNT(*) FROM stint_summary`).Scan(&summaries); err != nil {
		t.Fatal(err)
	}
	if summaries != 2 {
		t.Errorf("want 2 summaries after shutdown drain, got %d", summaries)
	}
}

// ---------- Session lifecycle (ADR 0012: data-driven boundaries) ----------

type sessionRow struct {
	id      string
	started int64
	ended   *int64
}

func readSessions(t *testing.T, store *Store) []sessionRow {
	t.Helper()
	rows, err := store.db.Query(`SELECT id, started_at_ns, ended_at_ns FROM sessions ORDER BY started_at_ns`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []sessionRow
	for rows.Next() {
		var s sessionRow
		if err := rows.Scan(&s.id, &s.started, &s.ended); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func stintCountForSession(t *testing.T, store *Store, sessionID string) int {
	t.Helper()
	var n int
	if err := store.db.QueryRow(
		`SELECT COUNT(*) FROM stints WHERE session_id = ?`, sessionID,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Sessions are born from data and split on a >=1h silence; the old session's
// end is its last tick's arrival, not wall-clock at split time.
func TestWriterSessionGapSplit(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())

	writer, sub, done, cancel := startWriter(t, store)
	defer cancel()

	base := int64(0)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{isRaceOn: true}))
	}
	lastOfFirst := base + 4*int64(500*time.Millisecond)
	base += int64(2 * time.Hour)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{isRaceOn: true}))
	}

	waitForStints(t, store, 2)
	cancel()
	sub.Close()
	_ = <-done

	sessions := readSessions(t, store)
	if len(sessions) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(sessions))
	}
	if sessions[0].started != 0 {
		t.Errorf("session 1 started: want 0 got %d", sessions[0].started)
	}
	if sessions[0].ended == nil || *sessions[0].ended != lastOfFirst {
		t.Errorf("session 1 ended: want %d got %v", lastOfFirst, sessions[0].ended)
	}
	if sessions[1].started != base {
		t.Errorf("session 2 started: want %d got %d", base, sessions[1].started)
	}
	if sessions[1].ended == nil {
		t.Error("session 2 must be closed at shutdown")
	}
	for _, s := range sessions {
		if n := stintCountForSession(t, store, s.id); n != 1 {
			t.Errorf("session %s: want 1 stint got %d", s.id, n)
		}
	}
}

// A GameTSMillis regression (game relaunch) splits the session even with no
// arrival gap at all.
func TestWriterSessionSplitsOnGameRestart(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())

	writer, sub, done, cancel := startWriter(t, store)
	defer cancel()

	base := int64(0)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{
			isRaceOn: true,
			gameTS:   10_000_000 + uint32(i)*500,
		}))
	}
	// Only 60s later by arrival, but the game clock restarted near zero.
	base += int64(60 * time.Second)
	for i := 0; i < 5; i++ {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{
			isRaceOn: true,
			gameTS:   1_000 + uint32(i)*500,
		}))
	}

	waitForStints(t, store, 2)
	cancel()
	sub.Close()
	_ = <-done

	if sessions := readSessions(t, store); len(sessions) != 2 {
		t.Fatalf("game restart must split the session: want 2, got %d", len(sessions))
	}
	if stints := readStints(t, store); len(stints) != 2 {
		t.Fatalf("session split must also close the stint: want 2, got %d", len(stints))
	}
}

// Out-of-order UDP delivery jitters GameTSMillis backwards by fractions of a
// second — regressions inside the tolerance must NOT split anything.
func TestWriterSmallTSRegressionDoesNotSplit(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())

	writer, sub, done, cancel := startWriter(t, store)
	defer cancel()

	base := int64(0)
	ts := []uint32{100_000, 100_500, 101_000, 70_000 + 41_000, 101_500, 102_000} // one 30s dip
	for i, g := range ts {
		writer.broker.Publish(makeTick(base+int64(i)*int64(500*time.Millisecond), tickOpts{
			isRaceOn: true,
			gameTS:   g,
		}))
	}
	// Throwaway gap tick closes stint 1 deterministically before asserting.
	writer.broker.Publish(makeTick(base+int64(15*time.Minute), tickOpts{isRaceOn: true}))

	waitForStints(t, store, 2)
	cancel()
	sub.Close()
	_ = <-done

	if sessions := readSessions(t, store); len(sessions) != 1 {
		t.Fatalf("tolerated regression split the session: want 1, got %d", len(sessions))
	}
	stints := readStints(t, store)
	if len(stints) != 1 || stints[0].tickCount != int64(len(ts)) {
		t.Fatalf("want 1 stint with %d ticks, got %+v", len(ts), stints)
	}
}

// A server left running with no game sending must not manufacture sessions.
func TestWriterNoTicksNoSession(t *testing.T) {
	store := newTestStore(t)
	defer store.Close(context.Background())

	_, sub, done, cancel := startWriter(t, store)
	time.Sleep(50 * time.Millisecond)
	cancel()
	sub.Close()
	_ = <-done

	if sessions := readSessions(t, store); len(sessions) != 0 {
		t.Errorf("want 0 sessions with no data, got %d", len(sessions))
	}
}
