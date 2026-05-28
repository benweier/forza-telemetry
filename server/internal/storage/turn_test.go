package storage

import (
	"math"
	"testing"
)

// makeArcSamples generates path samples tracing a circular arc. Centre is at
// (cx, cz); radius r; sweeping startAngle → endAngle (radians); n samples
// evenly spaced; lateral G fixed, longitudinal G zero. tickNS starts at 0,
// spaced ~16ms (~60Hz).
func makeArcSamples(cx, cz, r, startAngle, endAngle float64, n int, latG float32) []pathSample {
	out := make([]pathSample, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(n-1)
		a := startAngle + (endAngle-startAngle)*t
		out[i] = pathSample{
			tickNS: int64(i) * 16_000_000,
			x:      float32(cx + r*math.Sin(a)),
			z:      float32(cz + r*math.Cos(a)),
			latG:   latG,
		}
	}
	return out
}

func makeStraightSamples(z0, dz float64, n int) []pathSample {
	out := make([]pathSample, n)
	for i := 0; i < n; i++ {
		out[i] = pathSample{
			tickNS: int64(i) * 16_000_000,
			x:      0,
			z:      float32(z0 + dz*float64(i)),
		}
	}
	return out
}

// integratePath builds a path by stepping a unit-length each tick along an
// integrated heading. headingDelta[i] is the per-step δθ in radians.
func integratePath(headingDeltas []float64, latGs []float32) []pathSample {
	samples := make([]pathSample, len(headingDeltas))
	x, z, heading := 0.0, 0.0, 0.0
	for i := range headingDeltas {
		heading += headingDeltas[i]
		x += math.Sin(heading)
		z += math.Cos(heading)
		samples[i] = pathSample{
			tickNS: int64(i) * 16_000_000,
			x:      float32(x),
			z:      float32(z),
			latG:   latGs[i],
		}
	}
	return samples
}

func TestDetectTurnsStraightLineNone(t *testing.T) {
	samples := makeStraightSamples(0, 1.0, 200) // 200m straight
	got := detectTurns(samples)
	if len(got) != 0 {
		t.Errorf("straight line should produce 0 turns, got %d", len(got))
	}
}

func TestDetectTurnsSingleRightHand(t *testing.T) {
	// 50m radius right-hand 90° turn, ~78m of arc length. Surround with
	// straight entry + exit so resampling has clean start/end.
	entry := makeStraightSamples(0, 1.0, 100)
	arc := makeArcSamples(50, 100, 50, -math.Pi/2, 0, 80, 0.8)
	for i := range arc {
		arc[i].tickNS += int64(100) * 16_000_000
	}
	exitStart := arc[len(arc)-1]
	exit := make([]pathSample, 100)
	for i := 0; i < 100; i++ {
		exit[i] = pathSample{
			tickNS: exitStart.tickNS + int64(i+1)*16_000_000,
			x:      exitStart.x + float32(i+1),
			z:      exitStart.z,
		}
	}
	samples := append(append(entry, arc...), exit...)

	got := detectTurns(samples)
	if len(got) != 1 {
		t.Fatalf("want 1 turn, got %d: %+v", len(got), got)
	}
	c := got[0]
	// Radius 50m, 90° sweep → |Δθ| ≈ π/2 ≈ 1.57.
	if math.Abs(c.PeakDeltaTheta) < 1.2 || math.Abs(c.PeakDeltaTheta) > 1.8 {
		t.Errorf("peak Δθ: want ~π/2 got %v", c.PeakDeltaTheta)
	}
	// Radius 50m → κ ≈ 1/50 = 0.02.
	if math.Abs(c.PeakCurvature) < 0.015 || math.Abs(c.PeakCurvature) > 0.030 {
		t.Errorf("peak curvature: want ~0.02 got %v", c.PeakCurvature)
	}
	if c.Direction != "right" {
		t.Errorf("direction: want right got %s", c.Direction)
	}
}

func TestDetectTurnsSCurveTwoTurns(t *testing.T) {
	// Build a path: 30 straight, 80 right (δθ > 0), 30 straight, 80 left, 30 straight.
	const turnRate = math.Pi / 2 / 80
	var deltas []float64
	var lats []float32
	for i := 0; i < 30; i++ {
		deltas = append(deltas, 0)
		lats = append(lats, 0)
	}
	for i := 0; i < 80; i++ {
		deltas = append(deltas, +turnRate)
		lats = append(lats, 0.8)
	}
	for i := 0; i < 30; i++ {
		deltas = append(deltas, 0)
		lats = append(lats, 0)
	}
	for i := 0; i < 80; i++ {
		deltas = append(deltas, -turnRate)
		lats = append(lats, -0.8)
	}
	for i := 0; i < 30; i++ {
		deltas = append(deltas, 0)
		lats = append(lats, 0)
	}
	samples := integratePath(deltas, lats)

	got := detectTurns(samples)
	if len(got) != 2 {
		t.Fatalf("want 2 turns, got %d: %+v", len(got), got)
	}
	if got[0].Direction == got[1].Direction {
		t.Errorf("S-curve turns must have opposite directions, got %s + %s",
			got[0].Direction, got[1].Direction)
	}
	if got[0].PeakCurvature*got[1].PeakCurvature >= 0 {
		t.Errorf("S-curve curvatures must have opposite signs, got %v + %v",
			got[0].PeakCurvature, got[1].PeakCurvature)
	}
	// Each leg sweeps 90° → |Δθ| ≈ π/2. Opposite signs.
	if got[0].PeakDeltaTheta*got[1].PeakDeltaTheta >= 0 {
		t.Errorf("S-curve Δθ must have opposite signs, got %v + %v",
			got[0].PeakDeltaTheta, got[1].PeakDeltaTheta)
	}
}

func TestDetectTurnsBoundaryExtension(t *testing.T) {
	// 50 ticks brake (longG = -0.4), 80 ticks arc, 50 ticks accel (longG = +0.4).
	// Boundary extension should sweep back into brake and forward into accel.
	const step = int64(16_000_000)
	tickNS := int64(0)
	var samples []pathSample
	for i := 0; i < 50; i++ {
		samples = append(samples, pathSample{tickNS: tickNS, x: 0, z: float32(i), longG: -0.4})
		tickNS += step
	}
	arc := makeArcSamples(50, 50, 50, -math.Pi/2, 0, 80, 0.8)
	for i := range arc {
		arc[i].tickNS = tickNS
		tickNS += step
	}
	samples = append(samples, arc...)
	last := arc[len(arc)-1]
	for i := 0; i < 50; i++ {
		samples = append(samples, pathSample{
			tickNS: tickNS,
			x:      last.x + float32(i+1),
			z:      last.z,
			longG:  0.4,
		})
		tickNS += step
	}

	got := detectTurns(samples)
	if len(got) != 1 {
		t.Fatalf("want 1 turn, got %d", len(got))
	}
	c := got[0]
	arcStartNS := arc[0].tickNS
	if c.StartTickNS >= arcStartNS {
		t.Errorf("StartTickNS=%d should be extended back before arc start %d",
			c.StartTickNS, arcStartNS)
	}
	arcEndNS := arc[len(arc)-1].tickNS
	if c.EndTickNS <= arcEndNS {
		t.Errorf("EndTickNS=%d should be extended forward past arc end %d",
			c.EndTickNS, arcEndNS)
	}
}

func TestDetectTurnsRejectsSwerveOnStraight(t *testing.T) {
	// A driver-correction "swerve" on a straight: brief left then brief right
	// with sustained lat-G but ~0 net Δθ. Old lat-G-gated detector would
	// false-positive here; Δθ-gated detector must reject.
	//
	// 50 ticks straight; 15 ticks of +δθ (≈ 5° total); 15 ticks of -δθ
	// (re-aligning, ≈ 5° back); 50 ticks straight. Per-half |Δθ| ≈ 5° is
	// well under the 15° gate. Driver-correction swerves on a real straight
	// are this small or smaller; the gate must reject them.
	const wobbleRate = (5.0 * math.Pi / 180.0) / 15 // 5° spread over 15 ticks
	var deltas []float64
	var lats []float32
	for i := 0; i < 50; i++ {
		deltas = append(deltas, 0)
		lats = append(lats, 0)
	}
	for i := 0; i < 15; i++ {
		deltas = append(deltas, +wobbleRate)
		lats = append(lats, 0.4)
	}
	for i := 0; i < 15; i++ {
		deltas = append(deltas, -wobbleRate)
		lats = append(lats, -0.4)
	}
	for i := 0; i < 50; i++ {
		deltas = append(deltas, 0)
		lats = append(lats, 0)
	}
	samples := integratePath(deltas, lats)

	got := detectTurns(samples)
	if len(got) != 0 {
		t.Errorf("swerve on straight must produce 0 turns (Δθ-gated), got %d: %+v",
			len(got), got)
	}
}

func TestDetectTurnsChicaneSplitsIntoTwo(t *testing.T) {
	// Chicane: ~30° left immediately followed by ~30° right. Net Δθ ≈ 0 but
	// each half independently exceeds the 15° threshold, so each is its own
	// Turn row (per ADR 0008's "each direction-change = own Turn").
	const turnRate = math.Pi / 6 / 40 // 30° over 40 ticks
	var deltas []float64
	var lats []float32
	for i := 0; i < 30; i++ {
		deltas = append(deltas, 0)
		lats = append(lats, 0)
	}
	for i := 0; i < 40; i++ {
		deltas = append(deltas, +turnRate)
		lats = append(lats, 0.6)
	}
	for i := 0; i < 40; i++ {
		deltas = append(deltas, -turnRate)
		lats = append(lats, -0.6)
	}
	for i := 0; i < 30; i++ {
		deltas = append(deltas, 0)
		lats = append(lats, 0)
	}
	samples := integratePath(deltas, lats)

	got := detectTurns(samples)
	if len(got) != 2 {
		t.Fatalf("want 2 turns (chicane split), got %d: %+v", len(got), got)
	}
	if got[0].Direction == got[1].Direction {
		t.Errorf("chicane halves must have opposite directions, got %s + %s",
			got[0].Direction, got[1].Direction)
	}
}
