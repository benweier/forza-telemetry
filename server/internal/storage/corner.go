package storage

import "math"

// pathSample is the minimum per-tick data needed for corner detection.
// Populated by the Writer during the stint and discarded after detection.
type pathSample struct {
	tickNS int64
	x, z   float32
	longG  float32
	latG   float32
	lap    uint16
}

// cornerCandidate is one detected corner. Numbered later (within lap).
type cornerCandidate struct {
	StartTickNS   int64
	ApexTickNS    int64
	EndTickNS     int64
	PeakCurvature float64 // signed; sign determines direction
	PeakLateralG  float64 // absolute peak |LateralG| within region
	Direction     string  // "left" / "right"
}

// Detection parameters. First-pass defaults; tune against real captures.
const (
	resampleDistanceM = 1.0  // resample path every metre of arc length
	minCurvature      = 0.01 // rad/m — ~100m corner radius threshold
	minRegionPoints   = 3    // need at least this many resampled points of curving path
	minLateralG       = 0.4  // confirmation: corner must produce real lat G
	brakingG          = -0.2 // longitudinal G threshold for backward extension
	exitAccelG        = 0.2  // longitudinal G threshold for forward extension
)

type resampledPoint struct {
	tickNS int64
	x, z   float32
}

// detectCorners runs the full pipeline on one lap's worth of samples:
// resample → curvature → region threshold → lateral G confirmation →
// boundary extension via longitudinal G. Samples must be sorted by tickNS.
func detectCorners(samples []pathSample) []cornerCandidate {
	if len(samples) < 4 {
		return nil
	}
	rs := resamplePath(samples, resampleDistanceM)
	if len(rs) < 3 {
		return nil
	}
	curvatures := computeCurvature(rs)
	regions := findCurvatureRegions(curvatures, minCurvature, minRegionPoints)
	if len(regions) == 0 {
		return nil
	}
	var out []cornerCandidate
	for _, reg := range regions {
		// Peak curvature inside region.
		apexIdx := reg.startIdx
		peakAbs := math.Abs(curvatures[apexIdx])
		for i := reg.startIdx + 1; i <= reg.endIdx; i++ {
			if a := math.Abs(curvatures[i]); a > peakAbs {
				peakAbs = a
				apexIdx = i
			}
		}
		// rs[apexIdx+1] is the curvature-bearing centre point (curvatures
		// align with the centre vertex of each 3-point triple, so the apex
		// tick corresponds to rs index apexIdx+1).
		startNS := rs[reg.startIdx+1].tickNS
		apexNS := rs[apexIdx+1].tickNS
		endNS := rs[reg.endIdx+1].tickNS

		// Confirm with sustained lateral G within the region tick range.
		peakLatG := peakAbsLateralG(samples, startNS, endNS)
		if peakLatG < minLateralG {
			continue
		}

		// Extend boundaries with longitudinal G — go back into the braking
		// zone, then forward into the exit acceleration zone.
		startNS = extendBackward(samples, startNS, brakingG)
		endNS = extendForward(samples, endNS, exitAccelG)

		signed := curvatures[apexIdx]
		dir := "left"
		if signed > 0 {
			dir = "right"
		}
		out = append(out, cornerCandidate{
			StartTickNS:   startNS,
			ApexTickNS:    apexNS,
			EndTickNS:     endNS,
			PeakCurvature: signed,
			PeakLateralG:  peakLatG,
			Direction:     dir,
		})
	}
	return out
}

// resamplePath emits points spaced at >= step metres of straight-line
// distance from the previous emit. This is an approximation of uniform-arc
// resampling; good enough for curvature on ~60Hz Forza paths.
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

// computeCurvature returns signed κ at each interior point of rs. κ[0] and
// κ[len-1] are sentinel zeros so the slice length matches len(rs)-2 (one
// curvature per centre-vertex of a 3-point triple).
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

// findCurvatureRegions returns the index ranges (into the curvatures slice)
// where |κ| stays above threshold for at least minPoints consecutive entries.
// Regions broken by a single sub-threshold blip are NOT merged here — that
// would over-claim; rely on the boundary extension to widen real corners.
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

// peakAbsLateralG returns max(|LateralG|) across the tick range [startNS, endNS].
func peakAbsLateralG(samples []pathSample, startNS, endNS int64) float64 {
	peak := 0.0
	for _, s := range samples {
		if s.tickNS < startNS || s.tickNS > endNS {
			continue
		}
		v := math.Abs(float64(s.latG))
		if v > peak {
			peak = v
		}
	}
	return peak
}

// boundarySearchTicks is the lookback / lookahead window for boundary
// extension. Neutral ticks (between curving and braking/accel zones) within
// this window are tolerated; once the accel/brake zone is found, the
// boundary is extended through its consecutive run.
const boundarySearchTicks = 60

// extendBackward walks backward from startNS, searching up to
// boundarySearchTicks for a braking onset (longG ≤ brakeThreshold), then
// extending to the earliest consecutive braking tick.
func extendBackward(samples []pathSample, startNS int64, brakeThreshold float64) int64 {
	// Find the index just before startNS.
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
	// Search backward for brake onset.
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
	// Walk further back through consecutive brake ticks.
	for j := onset - 1; j >= 0; j-- {
		if float64(samples[j].longG) > brakeThreshold {
			break
		}
		onset = j
	}
	return samples[onset].tickNS
}

// extendForward is the mirror of extendBackward — searches up to
// boundarySearchTicks for an accel onset (longG ≥ accelThreshold), then
// extends through consecutive accel ticks.
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
