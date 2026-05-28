package storage

import "math"

// Straight derivation per ADR 0008. Given the sorted Turns of a stint and the
// stint's tick range, emit the K+1 Straights that fill every tick not covered
// by a Turn. The K+1 invariant is held strictly — leading or trailing
// Straights at stint edges may be zero-length if a Turn's extended boundary
// reaches the stint edge, but they are still emitted as rows so the index
// sequence is uninterrupted.

type straightCandidate struct {
	StartTickNS int64
	EndTickNS   int64
	DistanceM   float64
	PeakSpeedMS float64 // 0 if no samples fall in range
}

// deriveStraights emits Straights for a stint. `turns` must be sorted by
// StartTickNS. `samples` must be sorted by tickNS. `stintStart` / `stintEnd`
// bound the full stint. Zero-length Straights are emitted with
// StartTickNS == EndTickNS, distance 0, peak speed 0.
func deriveStraights(turns []turnCandidate, samples []pathSample, stintStart, stintEnd int64) []straightCandidate {
	// Bookend the Turn list with sentinel boundaries so the gap walk yields
	// K+1 straights uniformly.
	boundaries := make([][2]int64, 0, len(turns)+2)
	boundaries = append(boundaries, [2]int64{stintStart - 1, stintStart - 1}) // virtual "before start"
	for _, t := range turns {
		boundaries = append(boundaries, [2]int64{t.StartTickNS, t.EndTickNS})
	}
	boundaries = append(boundaries, [2]int64{stintEnd + 1, stintEnd + 1}) // virtual "after end"

	out := make([]straightCandidate, 0, len(turns)+1)
	for i := 0; i < len(boundaries)-1; i++ {
		start := boundaries[i][1] + 1
		end := boundaries[i+1][0] - 1
		if start > end {
			// Adjacent Turns or Turn flush against stint edge — emit a
			// zero-length placeholder anchored to a valid in-range tick so
			// the K+1 invariant holds.
			anchor := start
			if anchor < stintStart {
				anchor = stintStart
			}
			if anchor > stintEnd {
				anchor = stintEnd
			}
			start = anchor
			end = anchor
		} else {
			if start < stintStart {
				start = stintStart
			}
			if end > stintEnd {
				end = stintEnd
			}
		}
		dist, peakSpeed := summariseRange(samples, start, end)
		out = append(out, straightCandidate{
			StartTickNS: start,
			EndTickNS:   end,
			DistanceM:   dist,
			PeakSpeedMS: peakSpeed,
		})
	}
	return out
}

// summariseRange returns (distance_m, peak_speed_ms) across samples whose
// tickNS lies in [start, end]. Distance is the polyline length between
// consecutive in-range samples; speed is `max(speedMS)`. Both 0 if fewer
// than 2 samples fall in range.
func summariseRange(samples []pathSample, start, end int64) (float64, float64) {
	if start > end {
		return 0, 0
	}
	var dist float64
	var peakSpeed float64
	haveLast := false
	var lastX, lastZ float32
	for _, s := range samples {
		if s.tickNS < start {
			continue
		}
		if s.tickNS > end {
			break
		}
		if haveLast {
			dx := float64(s.x - lastX)
			dz := float64(s.z - lastZ)
			dist += math.Hypot(dx, dz)
		}
		lastX, lastZ = s.x, s.z
		haveLast = true
		if v := float64(s.speedMS); v > peakSpeed {
			peakSpeed = v
		}
	}
	return dist, peakSpeed
}
