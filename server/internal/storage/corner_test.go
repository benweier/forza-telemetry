package storage

import (
	"math"
	"testing"
)

// makeArcSamples generates path samples tracing a circular arc.
// Centre is at (cx, cz); radius r; sweeping from startAngle to endAngle
// (radians); n samples evenly spaced along the sweep; lateral G fixed,
// longitudinal G zero. tickNS starts at 0, spaced 16ms apart (~60Hz).
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

func TestDetectCornersStraightLineNone(t *testing.T) {
	samples := makeStraightSamples(0, 1.0, 200) // 200m straight
	got := detectCorners(samples)
	if len(got) != 0 {
		t.Errorf("straight line should produce 0 corners, got %d", len(got))
	}
}

func TestDetectCornersSingleRightHand(t *testing.T) {
	// 50m radius right-hand 90° corner, ~78m of arc length.
	// Surround with 100m straight entry + 100m straight exit so resampling
	// has clean start/end.
	entry := makeStraightSamples(0, 1.0, 100)
	// Arc start at (0, 100) heading +z, end at (50, 150) heading +x.
	// Right-hand turn: centre at (50, 100), radius 50, angles -π/2 → 0 (CCW
	// in atan2 terms, but in Forza coords this is a clockwise/right turn).
	arc := makeArcSamples(50, 100, 50, -math.Pi/2, 0, 80, 0.8)
	// Time-shift arc to follow entry
	for i := range arc {
		arc[i].tickNS += int64(100) * 16_000_000
	}
	// Exit straight starts at arc endpoint heading +x
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

	got := detectCorners(samples)
	if len(got) != 1 {
		t.Fatalf("want 1 corner, got %d: %+v", len(got), got)
	}
	c := got[0]
	if c.PeakLateralG < 0.79 || c.PeakLateralG > 0.81 {
		t.Errorf("peak lateral G: want ~0.8 got %v", c.PeakLateralG)
	}
	// Radius 50m → κ ≈ 1/50 = 0.02. Sign: right turn (positive in Forza atan2(dx,dz) convention).
	if math.Abs(c.PeakCurvature) < 0.015 || math.Abs(c.PeakCurvature) > 0.030 {
		t.Errorf("peak curvature: want ~0.02 got %v", c.PeakCurvature)
	}
	if c.Direction != "right" {
		t.Errorf("direction: want right got %s", c.Direction)
	}
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

func TestDetectCornersSCurveTwoCorners(t *testing.T) {
	// Build a path with: 30-tick straight, 80-tick right turn (δθ > 0),
	// 30-tick straight, 80-tick left turn (δθ < 0), 30-tick straight.
	const turnRate = math.Pi / 2 / 80 // 90° over 80 ticks
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

	got := detectCorners(samples)
	if len(got) != 2 {
		t.Fatalf("want 2 corners, got %d: %+v", len(got), got)
	}
	if got[0].Direction == got[1].Direction {
		t.Errorf("S-curve corners must have opposite directions, got %s + %s",
			got[0].Direction, got[1].Direction)
	}
	if got[0].PeakCurvature*got[1].PeakCurvature >= 0 {
		t.Errorf("S-curve curvatures must have opposite signs, got %v + %v",
			got[0].PeakCurvature, got[1].PeakCurvature)
	}
}

func TestDetectCornersBoundaryExtension(t *testing.T) {
	// Synthetic: 50 ticks of braking (longG = -0.4), then 50 ticks of arc,
	// then 50 ticks of exit acceleration (longG = +0.4). Boundary extension
	// should sweep back into the brake zone and forward into the accel zone.
	const step = int64(16_000_000)
	tickNS := int64(0)
	var samples []pathSample
	// Brake leading-up: straight along +z
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

	got := detectCorners(samples)
	if len(got) != 1 {
		t.Fatalf("want 1 corner, got %d", len(got))
	}
	c := got[0]
	// Start must be ≤ start of arc (i.e. extended back into brake zone)
	arcStartNS := arc[0].tickNS
	if c.StartTickNS >= arcStartNS {
		t.Errorf("StartTickNS=%d should be extended back before arc start %d", c.StartTickNS, arcStartNS)
	}
	// End must be ≥ end of arc (extended forward into accel zone)
	arcEndNS := arc[len(arc)-1].tickNS
	if c.EndTickNS <= arcEndNS {
		t.Errorf("EndTickNS=%d should be extended forward past arc end %d", c.EndTickNS, arcEndNS)
	}
}

func TestDetectCornersNoLateralG(t *testing.T) {
	// Curving geometry but lateral G stays near zero — confirmation fails.
	arc := makeArcSamples(50, 100, 50, -math.Pi/2, 0, 80, 0.1)
	got := detectCorners(arc)
	if len(got) != 0 {
		t.Errorf("low lateral G should produce 0 corners, got %d", len(got))
	}
}
