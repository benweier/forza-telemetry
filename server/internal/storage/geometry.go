package storage

import "math"

// pathSample is the minimum per-tick data needed for turn + straight
// detection. Populated by the Writer during the stint and discarded after
// detection.
type pathSample struct {
	tickNS  int64
	x, z    float32
	speedMS float32
	longG   float32
	latG    float32
	lap     uint16
}

type resampledPoint struct {
	tickNS int64
	x, z   float32
}

// resamplePath emits points spaced at >= step metres of straight-line
// distance from the previous emit. Approximation of uniform-arc resampling;
// good enough for curvature on ~60Hz Forza paths.
func resamplePath(samples []pathSample, step float64) []resampledPoint {
	out := []resampledPoint{{tickNS: samples[0].tickNS, x: samples[0].x, z: samples[0].z}}
	lastX, lastZ := samples[0].x, samples[0].z
	for _, s := range samples[1:] {
		dx := float64(s.x - lastX)
		dz := float64(s.z - lastZ)
		if math.Hypot(dx, dz) < step {
			continue
		}
		out = append(out, resampledPoint{tickNS: s.tickNS, x: s.x, z: s.z})
		lastX, lastZ = s.x, s.z
	}
	return out
}

// computeCurvature returns signed κ at each interior point of rs. The output
// slice has length len(rs)-2 — one curvature per centre-vertex of a 3-point
// triple.
func computeCurvature(rs []resampledPoint) []float64 {
	out := make([]float64, len(rs)-2)
	for i := 0; i < len(out); i++ {
		a, b, c := rs[i], rs[i+1], rs[i+2]
		thetaIn := math.Atan2(float64(b.x-a.x), float64(b.z-a.z))
		thetaOut := math.Atan2(float64(c.x-b.x), float64(c.z-b.z))
		dtheta := wrapPi(thetaOut - thetaIn)
		dsIn := math.Hypot(float64(b.x-a.x), float64(b.z-a.z))
		dsOut := math.Hypot(float64(c.x-b.x), float64(c.z-b.z))
		ds := (dsIn + dsOut) / 2
		if ds == 0 {
			out[i] = 0
			continue
		}
		out[i] = dtheta / ds
	}
	return out
}

type curvatureRegion struct {
	startIdx int
	endIdx   int
}

// findCurvatureRegions returns index ranges (into the curvatures slice) where
// |κ| stays above threshold for at least minPoints consecutive entries.
// Regions broken by a single sub-threshold blip are not merged here — that
// would over-claim; rely on the Turn detector's boundary extension to widen
// real turns.
func findCurvatureRegions(curvatures []float64, threshold float64, minPoints int) []curvatureRegion {
	var out []curvatureRegion
	i := 0
	for i < len(curvatures) {
		if math.Abs(curvatures[i]) < threshold {
			i++
			continue
		}
		start := i
		for i < len(curvatures) && math.Abs(curvatures[i]) >= threshold {
			i++
		}
		if i-start >= minPoints {
			out = append(out, curvatureRegion{startIdx: start, endIdx: i - 1})
		}
	}
	return out
}

// boundarySearchTicks is the lookback / lookahead window for Turn boundary
// extension. Neutral ticks (between curving and braking/accel zones) within
// this window are tolerated; once the accel/brake zone is found, the boundary
// is extended through its consecutive run.
const boundarySearchTicks = 60

// extendBackward walks backward from startNS, searching up to
// boundarySearchTicks for a braking onset (longG ≤ brakeThreshold), then
// extending to the earliest consecutive braking tick.
func extendBackward(samples []pathSample, startNS int64, brakeThreshold float64) int64 {
	before := -1
	for i := len(samples) - 1; i >= 0; i-- {
		if samples[i].tickNS < startNS {
			before = i
			break
		}
	}
	if before < 0 {
		return startNS
	}
	onset := -1
	for j := before; j >= 0 && before-j < boundarySearchTicks; j-- {
		if float64(samples[j].longG) <= brakeThreshold {
			onset = j
			break
		}
	}
	if onset < 0 {
		return startNS
	}
	for j := onset - 1; j >= 0; j-- {
		if float64(samples[j].longG) > brakeThreshold {
			break
		}
		onset = j
	}
	return samples[onset].tickNS
}

// extendForward is the mirror of extendBackward.
func extendForward(samples []pathSample, endNS int64, accelThreshold float64) int64 {
	after := -1
	for i, s := range samples {
		if s.tickNS > endNS {
			after = i
			break
		}
	}
	if after < 0 {
		return endNS
	}
	onset := -1
	for j := after; j < len(samples) && j-after < boundarySearchTicks; j++ {
		if float64(samples[j].longG) >= accelThreshold {
			onset = j
			break
		}
	}
	if onset < 0 {
		return endNS
	}
	last := onset
	for j := onset + 1; j < len(samples); j++ {
		if float64(samples[j].longG) < accelThreshold {
			break
		}
		last = j
	}
	return samples[last].tickNS
}

// wrapPi normalises an angle to (-π, π].
func wrapPi(theta float64) float64 {
	for theta > math.Pi {
		theta -= 2 * math.Pi
	}
	for theta <= -math.Pi {
		theta += 2 * math.Pi
	}
	return theta
}
