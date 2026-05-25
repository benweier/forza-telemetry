// Package tick defines the canonical, enriched Tick — the single per-sample record
// that flows through storage, the live stream, and the API.
//
// Schema is the SUPERSET across all supported Forza game versions (ADR 0003).
// Fields not present in a given game version are zero/nil; gameVersion and
// packetVariant discriminators are always populated. Evolution is additive only —
// never repurpose existing fields.
package tick

// GameVersion identifies which Forza title produced the source packet.
type GameVersion uint8

const (
	GameUnknown GameVersion = iota
	GameFH5
	GameFH6
	GameFM // Forza Motorsport (kept for future expansion)
)

func (g GameVersion) String() string {
	switch g {
	case GameFH5:
		return "fh5"
	case GameFH6:
		return "fh6"
	case GameFM:
		return "fm"
	default:
		return "unknown"
	}
}

// PacketVariant is the wire format of the inbound UDP packet.
type PacketVariant uint8

const (
	VariantUnknown PacketVariant = iota
	VariantSled
	VariantHorizonDash
	VariantMotorsportDash
	VariantHorizon6Dash
)

// Wheel index constants (FL/FR/RL/RR).
const (
	FL = 0
	FR = 1
	RL = 2
	RR = 3
)

// Tick is the canonical enriched record persisted to Parquet and broadcast on the
// live channel. Field order is grouped by category; do not rely on numeric order
// for wire format — that is the parser's responsibility.
//
// Per-wheel arrays are always FL, FR, RL, RR.
type Tick struct {
	// --- Metadata (always populated) ---
	GameVersion   GameVersion   `parquet:"game_version" msgpack:"gv"`
	PacketVariant PacketVariant `parquet:"packet_variant" msgpack:"pv"`
	GameTSMillis  uint32        `parquet:"game_ts_ms" msgpack:"gts"`
	ServerRecvNS  int64         `parquet:"server_recv_ns" msgpack:"sts"`

	// --- Race state ---
	IsRaceOn        bool    `parquet:"is_race_on" msgpack:"race"`
	LapNumber       uint16  `parquet:"lap_number" msgpack:"lap"`
	RacePosition    uint8   `parquet:"race_position" msgpack:"pos"`
	CurrentLapTime  float32 `parquet:"current_lap_s" msgpack:"clt"`
	LastLapTime     float32 `parquet:"last_lap_s" msgpack:"llt"`
	BestLapTime     float32 `parquet:"best_lap_s" msgpack:"blt"`
	CurrentRaceTime float32 `parquet:"current_race_s" msgpack:"crt"`

	// --- Engine ---
	EngineRPM     float32 `parquet:"engine_rpm" msgpack:"rpm"`
	EngineMaxRPM  float32 `parquet:"engine_max_rpm" msgpack:"rmx"`
	EngineIdleRPM float32 `parquet:"engine_idle_rpm" msgpack:"rid"`
	Power         float32 `parquet:"power_w" msgpack:"pw"`
	Torque        float32 `parquet:"torque_nm" msgpack:"tq"`
	Boost         float32 `parquet:"boost" msgpack:"bo"`
	Fuel          float32 `parquet:"fuel" msgpack:"fu"`

	// --- Car identity ---
	CarOrdinal          int32 `parquet:"car_ordinal" msgpack:"co"`
	CarClass            int32 `parquet:"car_class" msgpack:"cc"`
	CarPerformanceIndex int32 `parquet:"car_pi" msgpack:"cpi"`
	DrivetrainType      int32 `parquet:"drivetrain" msgpack:"dt"`
	NumCylinders        int32 `parquet:"num_cylinders" msgpack:"ncy"`
	// CarGroup is an FH6-only enum (values TBD). Always 0 for FH5/FM packets.
	CarGroup int32 `parquet:"car_group" msgpack:"cg"`

	// --- Smashable collision (FH6-only; 0 on FH5/FM) ---
	SmashableVelDiff float32 `parquet:"smashable_vel_diff" msgpack:"svd"`
	SmashableMass    float32 `parquet:"smashable_mass" msgpack:"sm"`

	// --- Motion: position / velocity / acceleration (world frame) ---
	PositionX float32 `parquet:"pos_x" msgpack:"px"`
	PositionY float32 `parquet:"pos_y" msgpack:"py"`
	PositionZ float32 `parquet:"pos_z" msgpack:"pz"`

	VelocityX float32 `parquet:"vel_x" msgpack:"vx"`
	VelocityY float32 `parquet:"vel_y" msgpack:"vy"`
	VelocityZ float32 `parquet:"vel_z" msgpack:"vz"`

	AccelerationX float32 `parquet:"acc_x" msgpack:"ax"`
	AccelerationY float32 `parquet:"acc_y" msgpack:"ay"`
	AccelerationZ float32 `parquet:"acc_z" msgpack:"az"`

	AngularVelocityX float32 `parquet:"avel_x" msgpack:"wx"`
	AngularVelocityY float32 `parquet:"avel_y" msgpack:"wy"`
	AngularVelocityZ float32 `parquet:"avel_z" msgpack:"wz"`

	Yaw   float32 `parquet:"yaw" msgpack:"yaw"`
	Pitch float32 `parquet:"pitch" msgpack:"pit"`
	Roll  float32 `parquet:"roll" msgpack:"rol"`

	Speed            float32 `parquet:"speed_ms" msgpack:"sp"`
	DistanceTraveled float32 `parquet:"distance_m" msgpack:"di"`

	// --- Inputs ---
	Accel     uint8 `parquet:"accel" msgpack:"in_a"`
	Brake     uint8 `parquet:"brake" msgpack:"in_b"`
	Clutch    uint8 `parquet:"clutch" msgpack:"in_c"`
	HandBrake uint8 `parquet:"handbrake" msgpack:"in_h"`
	Gear      uint8 `parquet:"gear" msgpack:"g"`
	Steer     int8  `parquet:"steer" msgpack:"st"`

	// --- Per-wheel arrays (FL, FR, RL, RR) ---
	TireSlipRatio        [4]float32 `parquet:"tire_slip_ratio" msgpack:"tsr"`
	TireSlipAngle        [4]float32 `parquet:"tire_slip_angle" msgpack:"tsa"`
	TireCombinedSlip     [4]float32 `parquet:"tire_combined_slip" msgpack:"tcs"`
	TireTemp             [4]float32 `parquet:"tire_temp" msgpack:"tt"`
	WheelRotationSpeed   [4]float32 `parquet:"wheel_rot_speed" msgpack:"wrs"`
	WheelOnRumbleStrip   [4]float32 `parquet:"wheel_rumble" msgpack:"wor"`
	WheelInPuddleDepth   [4]float32 `parquet:"wheel_puddle" msgpack:"wip"`
	SurfaceRumble        [4]float32 `parquet:"surface_rumble" msgpack:"sru"`
	NormSuspensionTravel [4]float32 `parquet:"susp_travel_norm" msgpack:"stn"`
	SuspensionTravelM    [4]float32 `parquet:"susp_travel_m" msgpack:"stm"`

	// --- Enriched fields (derived at ingest, never present on the wire) ---
	LateralG      float32 `parquet:"lateral_g" msgpack:"lg"`
	LongitudinalG float32 `parquet:"longitudinal_g" msgpack:"lng"`
	ThrottlePct   float32 `parquet:"throttle_pct" msgpack:"tp"`
	BrakePct      float32 `parquet:"brake_pct" msgpack:"bp"`
	RPMPct        float32 `parquet:"rpm_pct" msgpack:"rp"`
	GearShift     bool    `parquet:"gear_shift" msgpack:"gs"`
	LapDistanceM  float32 `parquet:"lap_distance_m" msgpack:"ld"`
}

// Enrich populates derived fields from raw values. Idempotent — safe to call
// multiple times. Called once per Tick at ingest immediately after parsing.
func (t *Tick) Enrich(prev *Tick) {
	t.ThrottlePct = float32(t.Accel) / 255
	t.BrakePct = float32(t.Brake) / 255
	if t.EngineMaxRPM > 0 {
		t.RPMPct = t.EngineRPM / t.EngineMaxRPM
	}
	const g = 9.80665
	t.LateralG = t.AccelerationX / g
	t.LongitudinalG = t.AccelerationZ / g
	if prev != nil {
		t.GearShift = prev.Gear != 0 && t.Gear != 0 && prev.Gear != t.Gear
		if t.LapNumber == prev.LapNumber {
			t.LapDistanceM = prev.LapDistanceM + t.Speed*deltaSeconds(prev.GameTSMillis, t.GameTSMillis)
		}
	}
}

func deltaSeconds(prevMS, currMS uint32) float32 {
	// Handles uint32 wrap implicitly via two's-complement subtraction.
	delta := currMS - prevMS
	if delta > 1_000_000 {
		// > 1000s delta is unrealistic between adjacent ticks; treat as discontinuity.
		return 0
	}
	return float32(delta) / 1000
}
