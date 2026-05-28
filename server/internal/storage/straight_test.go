package storage

import "testing"

func TestDeriveStraightsNoTurnsOneFullStraight(t *testing.T) {
	samples := []pathSample{
		{tickNS: 0, x: 0, z: 0, speedMS: 10},
		{tickNS: 100, x: 0, z: 100, speedMS: 20},
		{tickNS: 200, x: 0, z: 200, speedMS: 15},
	}
	got := deriveStraights(nil, samples, 0, 200)
	if len(got) != 1 {
		t.Fatalf("no turns → expect 1 straight, got %d", len(got))
	}
	if got[0].StartTickNS != 0 || got[0].EndTickNS != 200 {
		t.Errorf("range: want [0,200] got [%d,%d]", got[0].StartTickNS, got[0].EndTickNS)
	}
	if got[0].DistanceM < 199 || got[0].DistanceM > 201 {
		t.Errorf("distance: want ~200 got %v", got[0].DistanceM)
	}
	if got[0].PeakSpeedMS != 20 {
		t.Errorf("peak speed: want 20 got %v", got[0].PeakSpeedMS)
	}
}

func TestDeriveStraightsOneTurnInMiddleProducesTwo(t *testing.T) {
	samples := []pathSample{
		{tickNS: 0, x: 0, z: 0, speedMS: 30},
		{tickNS: 50, x: 0, z: 50, speedMS: 30},
		{tickNS: 100, x: 0, z: 100, speedMS: 15}, // turn region
		{tickNS: 150, x: 50, z: 100, speedMS: 25},
		{tickNS: 200, x: 100, z: 100, speedMS: 35},
	}
	turns := []turnCandidate{{StartTickNS: 51, EndTickNS: 149}}
	got := deriveStraights(turns, samples, 0, 200)
	if len(got) != 2 {
		t.Fatalf("1 turn → expect 2 straights, got %d", len(got))
	}
	if got[0].StartTickNS != 0 || got[0].EndTickNS != 50 {
		t.Errorf("straight 1: want [0,50] got [%d,%d]", got[0].StartTickNS, got[0].EndTickNS)
	}
	if got[1].StartTickNS != 150 || got[1].EndTickNS != 200 {
		t.Errorf("straight 2: want [150,200] got [%d,%d]", got[1].StartTickNS, got[1].EndTickNS)
	}
}

func TestDeriveStraightsTurnFlushAtStartEmitsZeroLengthLeading(t *testing.T) {
	samples := []pathSample{
		{tickNS: 0, x: 0, z: 0, speedMS: 20},
		{tickNS: 100, x: 50, z: 50, speedMS: 25},
		{tickNS: 200, x: 100, z: 100, speedMS: 30},
	}
	turns := []turnCandidate{{StartTickNS: 0, EndTickNS: 100}}
	got := deriveStraights(turns, samples, 0, 200)
	if len(got) != 2 {
		t.Fatalf("expect 2 straights (1 zero-length leading), got %d", len(got))
	}
	if got[0].StartTickNS != got[0].EndTickNS {
		t.Errorf("leading straight must be zero-length, got [%d,%d]",
			got[0].StartTickNS, got[0].EndTickNS)
	}
	if got[0].DistanceM != 0 {
		t.Errorf("leading zero-length distance: want 0 got %v", got[0].DistanceM)
	}
}

func TestDeriveStraightsTurnFlushAtEndEmitsZeroLengthTrailing(t *testing.T) {
	samples := []pathSample{
		{tickNS: 0, x: 0, z: 0, speedMS: 25},
		{tickNS: 100, x: 0, z: 100, speedMS: 30},
		{tickNS: 200, x: 50, z: 100, speedMS: 15},
	}
	turns := []turnCandidate{{StartTickNS: 100, EndTickNS: 200}}
	got := deriveStraights(turns, samples, 0, 200)
	if len(got) != 2 {
		t.Fatalf("expect 2 straights (1 zero-length trailing), got %d", len(got))
	}
	if got[1].StartTickNS != got[1].EndTickNS {
		t.Errorf("trailing straight must be zero-length, got [%d,%d]",
			got[1].StartTickNS, got[1].EndTickNS)
	}
}

func TestDeriveStraightsTwoTurnsProduceThree(t *testing.T) {
	samples := []pathSample{
		{tickNS: 0, x: 0, z: 0, speedMS: 30},
		{tickNS: 50, x: 0, z: 50, speedMS: 20},
		{tickNS: 100, x: 30, z: 70, speedMS: 25},
		{tickNS: 150, x: 60, z: 100, speedMS: 22},
		{tickNS: 200, x: 90, z: 100, speedMS: 35},
		{tickNS: 250, x: 90, z: 150, speedMS: 30},
	}
	turns := []turnCandidate{
		{StartTickNS: 50, EndTickNS: 100},
		{StartTickNS: 150, EndTickNS: 200},
	}
	got := deriveStraights(turns, samples, 0, 250)
	if len(got) != 3 {
		t.Fatalf("2 turns → expect 3 straights, got %d", len(got))
	}
}
