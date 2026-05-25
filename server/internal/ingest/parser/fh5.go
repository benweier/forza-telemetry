package parser

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/benweier/forza-telemetry/server/internal/tick"
)

// Wire-format sizes for Forza Data Out packets.
//
//	Sled                = 232 bytes  (FH4, FH5, FM — shared physics base)
//	HorizonDash         = 311 bytes  (FH4, FH5 — Sled + Horizon extensions)
//	MotorsportDash      = 331 bytes  (FM7 / FM 2023 — Sled + Motorsport extensions, with TireWear and TrackOrdinal)
//
// Sizes are the discriminator the Registry uses to pick a parser. FH6's exact
// size is TBD until verified against a real game capture.
const (
	SledSize           = 232
	HorizonDashSize    = 311
	MotorsportDashSize = 331
)

type fh5Base struct{}

func (fh5Base) GameVersion() tick.GameVersion { return tick.GameFH5 }

// FH5Sled parses the 232-byte Sled packet.
type FH5Sled struct{ fh5Base }

func NewFH5Sled() *FH5Sled                  { return &FH5Sled{} }
func (FH5Sled) Variant() tick.PacketVariant { return tick.VariantSled }

func (FH5Sled) Decode(buf []byte, out *tick.Tick) error {
	if len(buf) != SledSize {
		return fmt.Errorf("sled: expected %d bytes, got %d", SledSize, len(buf))
	}
	d := decoder{buf: buf}
	decodeSled(&d, out)
	return nil
}

// FH5Dash parses the 311-byte Horizon Dash packet (Sled prefix + Horizon tail).
type FH5Dash struct{ fh5Base }

func NewFH5Dash() *FH5Dash                  { return &FH5Dash{} }
func (FH5Dash) Variant() tick.PacketVariant { return tick.VariantHorizonDash }

func (FH5Dash) Decode(buf []byte, out *tick.Tick) error {
	if len(buf) != HorizonDashSize {
		return fmt.Errorf("dash: expected %d bytes, got %d", HorizonDashSize, len(buf))
	}
	d := decoder{buf: buf}
	decodeSled(&d, out)
	decodeHorizonTail(&d, out)
	return nil
}

func decodeSled(d *decoder, out *tick.Tick) {
	out.IsRaceOn = d.i32() != 0
	out.GameTSMillis = d.u32()
	out.EngineMaxRPM = d.f32()
	out.EngineIdleRPM = d.f32()
	out.EngineRPM = d.f32()
	out.AccelerationX = d.f32()
	out.AccelerationY = d.f32()
	out.AccelerationZ = d.f32()
	out.VelocityX = d.f32()
	out.VelocityY = d.f32()
	out.VelocityZ = d.f32()
	out.AngularVelocityX = d.f32()
	out.AngularVelocityY = d.f32()
	out.AngularVelocityZ = d.f32()
	out.Yaw = d.f32()
	out.Pitch = d.f32()
	out.Roll = d.f32()
	d.f32Array(out.NormSuspensionTravel[:])
	d.f32Array(out.TireSlipRatio[:])
	d.f32Array(out.WheelRotationSpeed[:])
	d.f32Array(out.WheelOnRumbleStrip[:])
	d.f32Array(out.WheelInPuddleDepth[:])
	d.f32Array(out.SurfaceRumble[:])
	d.f32Array(out.TireSlipAngle[:])
	d.f32Array(out.TireCombinedSlip[:])
	d.f32Array(out.SuspensionTravelM[:])
	out.CarOrdinal = d.i32()
	out.CarClass = d.i32()
	out.CarPerformanceIndex = d.i32()
	out.DrivetrainType = d.i32()
	out.NumCylinders = d.i32()
}

func decodeHorizonTail(d *decoder, out *tick.Tick) {
	out.PositionX = d.f32()
	out.PositionY = d.f32()
	out.PositionZ = d.f32()
	out.Speed = d.f32()
	out.Power = d.f32()
	out.Torque = d.f32()
	d.f32Array(out.TireTemp[:])
	out.Boost = d.f32()
	out.Fuel = d.f32()
	out.DistanceTraveled = d.f32()
	out.BestLapTime = d.f32()
	out.LastLapTime = d.f32()
	out.CurrentLapTime = d.f32()
	out.CurrentRaceTime = d.f32()
	out.LapNumber = d.u16()
	out.RacePosition = d.u8()
	out.Accel = d.u8()
	out.Brake = d.u8()
	out.Clutch = d.u8()
	out.HandBrake = d.u8()
	out.Gear = d.u8()
	out.Steer = int8(d.u8())
	_ = d.u8() // NormalizedDrivingLine — not currently surfaced
	_ = d.u8() // NormalizedAIBrakeDifference — not currently surfaced
}

type decoder struct {
	buf []byte
	off int
}

func (d *decoder) u8() uint8 {
	v := d.buf[d.off]
	d.off++
	return v
}

func (d *decoder) u16() uint16 {
	v := binary.LittleEndian.Uint16(d.buf[d.off:])
	d.off += 2
	return v
}

func (d *decoder) u32() uint32 {
	v := binary.LittleEndian.Uint32(d.buf[d.off:])
	d.off += 4
	return v
}

func (d *decoder) i32() int32 {
	return int32(d.u32())
}

func (d *decoder) f32() float32 {
	return math.Float32frombits(d.u32())
}

func (d *decoder) f32Array(out []float32) {
	for i := range out {
		out[i] = d.f32()
	}
}
