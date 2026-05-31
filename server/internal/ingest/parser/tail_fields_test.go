package parser

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/benweier/forza-telemetry/server/internal/tick"
)

// buildHorizonPacket builds a 311-byte FH5 Horizon Dash packet with the two
// trailing input bytes — DrivingLine and AIBrakeDifference, the last two bytes
// of the packet — set to known values, pinning the decode of those
// previously-discarded bytes.
func buildHorizonPacket(drivingLine, aiBrakeDiff int8) []byte {
	b := make([]byte, HorizonDashSize)
	binary.LittleEndian.PutUint32(b[0:], 1)                      // IsRaceOn = true
	binary.LittleEndian.PutUint32(b[8:], math.Float32bits(8000)) // EngineMaxRPM
	b[HorizonDashSize-2] = byte(drivingLine)                     // DrivingLine
	b[HorizonDashSize-1] = byte(aiBrakeDiff)                     // AIBrakeDifference
	return b
}

func TestHorizonTailDecodesDrivingLineAndAIBrake(t *testing.T) {
	cases := []struct {
		name        string
		drivingLine int8
		aiBrakeDiff int8
	}{
		{"zeros", 0, 0},
		{"upper observed range", 65, 122},
		{"lower observed range", -1, -127},
		{"mixed", 30, -56},
	}
	p := NewFH5Dash()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got tick.Tick
			if err := p.Decode(buildHorizonPacket(c.drivingLine, c.aiBrakeDiff), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.DrivingLine != c.drivingLine {
				t.Errorf("DrivingLine: want %d got %d", c.drivingLine, got.DrivingLine)
			}
			if got.AIBrakeDifference != c.aiBrakeDiff {
				t.Errorf("AIBrakeDifference: want %d got %d", c.aiBrakeDiff, got.AIBrakeDifference)
			}
		})
	}
}
