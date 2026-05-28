package storage

import "math"

// Turn detection per ADR 0008. Curvature anchors candidate regions; per-region
// accumulated heading change (Δθ) gates them against driver repositioning on
// straights (which produces lat-G but near-zero net heading change). Boundary
// extension into the braking + exit-acceleration zones carries forward from
// the superseded ADR 0007.

// turnCandidate is one detected Turn. Numbered later (within stint).
type turnCandidate struct {
	StartTickNS    int64
	ApexTickNS     int64
	EndTickNS      int64
	PeakCurvature  float64 // signed; sign determines direction
	PeakDeltaTheta float64 // signed; ≈ peak curvature × arc length of region
	Direction      string  // "left" / "right"
}

// Detection thresholds. First-pass; tracked in docs/data-needed.md for tuning.
const (
	turnResampleDistanceM = 1.0  // resample path every metre of arc length
	turnMinCurvature      = 0.01 // rad/m — ~100m corner radius threshold
	turnMinRegionPoints   = 3    // minimum resampled points of curving path
	turnMinDeltaTheta     = 0.26 // ~15° net heading change required for a Turn
	turnBrakingG          = -0.2 // longitudinal G threshold for backward extension
	turnExitAccelG        = 0.2  // longitudinal G threshold for forward extension
)

// detectTurns runs the full pipeline over a full stint's path samples:
// resample → curvature → region threshold → sign-split → Δθ confirmation →
// boundary extension via longitudinal G. Samples must be sorted by tickNS.
// Returns turns in chronological order; caller assigns turn_index.
func detectTurns(samples []pathSample) []turnCandidate {
	if len(samples) < 4 {
		return nil
	}
	rs := resamplePath(samples, turnResampleDistanceM)
	if len(rs) < 3 {
		return nil
	}
	curvatures := computeCurvature(rs)
	regions := findCurvatureRegions(curvatures, turnMinCurvature, turnMinRegionPoints)
	if len(regions) == 0 {
		return nil
	}

	// Split each region wherever the sign of κ flips so a chicane becomes two
	// adjacent Turn candidates (left + right) rather than one merged region
	// whose Δθ might partially cancel out.
	signedRegions := splitRegionsBySignFlip(regions, curvatures, turnMinRegionPoints)

	var out []turnCandidate
	for _, reg := range signedRegions {
		// Peak curvature inside region.
		apexIdx := reg.startIdx
		peakAbs := math.Abs(curvatures[apexIdx])
		for i := reg.startIdx + 1; i <= reg.endIdx; i++ {
			if a := math.Abs(curvatures[i]); a > peakAbs {
				peakAbs = a
				apexIdx = i
			}
		}

		// Δθ = ∫κ ds across the region. With uniform-ish resample step,
		// approximate ds via inter-sample chord length and sum signed
		// κ × ds. Reject if the magnitude is below threshold — the
		// hallmark of an in-lane wobble vs an actual track turn.
		deltaTheta := regionDeltaTheta(rs, curvatures, reg)
		if math.Abs(deltaTheta) < turnMinDeltaTheta {
			continue
		}

		// rs[apexIdx+1] is the curvature-bearing centre point (curvatures
		// align with the centre vertex of each 3-point triple, so the apex
		// tick corresponds to rs index apexIdx+1).
		startNS := rs[reg.startIdx+1].tickNS
		apexNS := rs[apexIdx+1].tickNS
		endNS := rs[reg.endIdx+1].tickNS

		// Extend boundaries into braking + exit acceleration zones.
		startNS = extendBackward(samples, startNS, turnBrakingG)
		endNS = extendForward(samples, endNS, turnExitAccelG)

		signed := curvatures[apexIdx]
		dir := "left"
		if signed > 0 {
			dir = "right"
		}
		out = append(out, turnCandidate{
			StartTickNS:    startNS,
			ApexTickNS:     apexNS,
			EndTickNS:      endNS,
			PeakCurvature:  signed,
			PeakDeltaTheta: deltaTheta,
			Direction:      dir,
		})
	}
	return out
}

// splitRegionsBySignFlip walks each input region and emits sub-regions whose
// κ values are sign-consistent (treating zero as a continuation of whichever
// sign neighbours it — but |κ| inside a region is always ≥ threshold, so
// zeros don't arise in practice).
func splitRegionsBySignFlip(regions []curvatureRegion, curvatures []float64, minPoints int) []curvatureRegion {
	var out []curvatureRegion
	for _, reg := range regions {
		start := reg.startIdx
		curSign := signOf(curvatures[start])
		for i := reg.startIdx + 1; i <= reg.endIdx; i++ {
			s := signOf(curvatures[i])
			if s != curSign {
				if i-start >= minPoints {
					out = append(out, curvatureRegion{startIdx: start, endIdx: i - 1})
				}
				start = i
				curSign = s
			}
		}
		if reg.endIdx+1-start >= minPoints {
			out = append(out, curvatureRegion{startIdx: start, endIdx: reg.endIdx})
		}
	}
	return out
}

func signOf(v float64) int {
	if v > 0 {
		return 1
	}
	if v < 0 {
		return -1
	}
	return 0
}

// regionDeltaTheta integrates signed κ × ds across the region. Approximates
// total heading change in radians; sign matches the direction of turning.
func regionDeltaTheta(rs []resampledPoint, curvatures []float64, reg curvatureRegion) float64 {
	var theta float64
	for i := reg.startIdx; i <= reg.endIdx; i++ {
		// ds = chord length between the two resampled points that bracket
		// curvatures[i] (which is the centre vertex of triple i, i+1, i+2).
		a, b := rs[i], rs[i+2]
		ds := math.Hypot(float64(b.x-a.x), float64(b.z-a.z)) / 2
		theta += curvatures[i] * ds
	}
	return theta
}
