package storage

import (
	"testing"

	"github.com/benweier/forza-telemetry/server/internal/tick"
)

func TestCategorize(t *testing.T) {
	cases := []struct {
		name string
		in   tick.Tick
		want stintCategory
	}{
		{"menu", tick.Tick{IsRaceOn: false}, categoryIdle},
		{"freeroam", tick.Tick{IsRaceOn: true, CurrentRaceTime: 0}, categoryFreeroam},
		{"race in progress", tick.Tick{IsRaceOn: true, CurrentRaceTime: 42.5}, categoryRace},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := categorize(&c.in); got != c.want {
				t.Errorf("categorize: got %d want %d", got, c.want)
			}
		})
	}
}

func TestResolveStintType(t *testing.T) {
	cases := []struct {
		name     string
		cat      stintCategory
		lapDelta uint16
		want     string
	}{
		{"idle", categoryIdle, 0, "idle"},
		{"freeroam", categoryFreeroam, 0, "freeroam"},
		{"race no laps -> sprint", categoryRace, 0, "sprint"},
		{"race with laps -> circuit", categoryRace, 3, "circuit"},
		{"race single lap inc -> circuit", categoryRace, 1, "circuit"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveStintType(c.cat, c.lapDelta); got != c.want {
				t.Errorf("resolveStintType: got %q want %q", got, c.want)
			}
		})
	}
}
