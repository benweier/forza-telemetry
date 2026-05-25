package parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/benweier/forza-telemetry/server/internal/tick"
)

func TestFH6DashWrongSize(t *testing.T) {
	p := NewFH6Dash(nil)
	var out tick.Tick
	if err := p.Decode(make([]byte, 311), &out); err == nil {
		t.Errorf("want size mismatch error, got nil")
	}
}

func TestFH6DashRegisterable(t *testing.T) {
	r := NewRegistry()
	r.Register(FH6PacketSize, NewFH6Dash(nil))
	p, err := r.Resolve(make([]byte, FH6PacketSize))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.GameVersion() != tick.GameFH6 {
		t.Errorf("GameVersion: want FH6 got %v", p.GameVersion())
	}
	if p.Variant() != tick.VariantHorizon6Dash {
		t.Errorf("Variant: want Horizon6Dash got %v", p.Variant())
	}
}

func TestFH6DashCaptureLog(t *testing.T) {
	var buf bytes.Buffer
	p := NewFH6Dash(&buf)
	p.clock = func() time.Time { return time.Unix(0, 1_700_000_000_000_000_000).UTC() }

	pkt := make([]byte, FH6PacketSize)
	for i := range pkt {
		pkt[i] = byte(i)
	}
	binary.LittleEndian.PutUint32(pkt[0:], 0) // IsRaceOn=0 to keep decode trivial
	var out tick.Tick
	if err := p.Decode(pkt, &out); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !strings.Contains(buf.String(), " 324 ") {
		t.Errorf("capture line missing size, got %q", buf.String())
	}
}

// fh6CapturedRaceOn is a real race-on packet captured 2026-05-24 from Forza
// Horizon 6 (CarOrdinal 3773, B-class, 4-cyl, mid-collision with a smashable).
// Used as a golden-fixture to guard the decoded layout against regressions.
const fh6CapturedRaceOn = "01000000950e2c00fbcf0446f8ff474426a3cd455b9853414697533f2c76a9bf83b89dc0257c713f92781442fa3d7e3e6804913e341b8cbcf72caf3f9c9850bd6b79bbbcc249c43e194f523e0911bb3e8568fc3d7d0f70bb10ecf0bc6e1221bc0000000070f7e742c9d5e242dd85e74285eee04200000000000000000000000000000000000000000000000000000000000000009999193f9999193f9999193f000000003c47e53eb39cf63e4f01073f000000003249e53e4512f73e5007073f0000000040bc16bbc88785bc00a5253ae83c89bcbd0e000003000000bc0200000200000004000000190000006922863ceb51383ebec31e454682dc43ddb099455dd21542bc900bc7bc4a4fc215111443631a1643707b0643707b0643bb5b30c10000803fe47647c4000000000000000000000000983dd744000000000000000400007a00"

func TestFH6DashDecodeRealPacket(t *testing.T) {
	pkt, err := hex.DecodeString(fh6CapturedRaceOn)
	if err != nil {
		t.Fatalf("decode fixture hex: %v", err)
	}
	if len(pkt) != FH6PacketSize {
		t.Fatalf("fixture size = %d, want %d", len(pkt), FH6PacketSize)
	}

	var out tick.Tick
	if err := NewFH6Dash(nil).Decode(pkt, &out); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	type check struct {
		name string
		got  any
		want any
	}
	checks := []check{
		{"IsRaceOn", out.IsRaceOn, true},
		{"GameTSMillis", out.GameTSMillis, uint32(2887317)},
		{"CarOrdinal", out.CarOrdinal, int32(3773)},
		{"CarClass", out.CarClass, int32(3)},
		{"CarPerformanceIndex", out.CarPerformanceIndex, int32(700)},
		{"NumCylinders", out.NumCylinders, int32(4)},
		{"CarGroup", out.CarGroup, int32(25)},
		{"LapNumber", out.LapNumber, uint16(0)},
		{"RacePosition", out.RacePosition, uint8(0)},
		{"Accel", out.Accel, uint8(0)},
		{"Brake", out.Brake, uint8(0)},
		{"HandBrake", out.HandBrake, uint8(0)},
		{"Gear", out.Gear, uint8(4)},
		{"Steer", out.Steer, int8(0)},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %v want %v", c.name, c.got, c.want)
		}
	}

	approx := []struct {
		name string
		got  float32
		want float32
	}{
		{"EngineRPM", out.EngineRPM, 6580.39},
		{"SmashableVelDiff", out.SmashableVelDiff, 0.01637},
		{"SmashableMass", out.SmashableMass, 0.18},
		{"PositionX", out.PositionX, 2540.23},
		{"PositionY", out.PositionY, 441.018},
		{"PositionZ", out.PositionZ, 4918.108},
		{"Speed", out.Speed, 37.4554},
		{"Fuel", out.Fuel, 1.0},
		{"CurrentRaceTime", out.CurrentRaceTime, 1721.92},
		{"TireTemp[0]", out.TireTemp[0], 148.067},
	}
	for _, a := range approx {
		if math.Abs(float64(a.got-a.want)) > 0.1 {
			t.Errorf("%s: got %v want ~%v", a.name, a.got, a.want)
		}
	}
}
