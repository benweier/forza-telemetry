package storage

import "testing"

func TestResolveStintType(t *testing.T) {
	cases := []struct {
		name     string
		raceOn   bool
		sawRace  bool
		lapDelta uint16
		want     string
	}{
		{"idle", false, false, 0, "idle"},
		{"idle outranks sawRace", false, true, 0, "idle"},
		{"freeroam", true, false, 0, "freeroam"},
		{"race no laps -> sprint", true, true, 0, "sprint"},
		{"race with laps -> circuit", true, true, 3, "circuit"},
		{"race single lap inc -> circuit", true, true, 1, "circuit"},
		{"laps without race time stay freeroam", true, false, 2, "freeroam"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveStintType(c.raceOn, c.sawRace, c.lapDelta); got != c.want {
				t.Errorf("resolveStintType: got %q want %q", got, c.want)
			}
		})
	}
}
