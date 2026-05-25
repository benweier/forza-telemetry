package storage

import "github.com/benweier/forza-telemetry/server/internal/tick"

// parquetRow mirrors tick.Tick but uses []float32 slices for per-wheel arrays
// — parquet-go's reflection-based schema cannot describe fixed Go arrays.
//
// Field names and parquet tags MUST stay in lockstep with tick.Tick's parquet
// tags. Schema evolution rules from ADR 0003 apply equally here: additive only.
type parquetRow struct {
	GameVersion   uint8  `parquet:"game_version"`
	PacketVariant uint8  `parquet:"packet_variant"`
	GameTSMillis  uint32 `parquet:"game_ts_ms"`
	ServerRecvNS  int64  `parquet:"server_recv_ns"`

	IsRaceOn        bool    `parquet:"is_race_on"`
	LapNumber       uint16  `parquet:"lap_number"`
	RacePosition    uint8   `parquet:"race_position"`
	CurrentLapTime  float32 `parquet:"current_lap_s"`
	LastLapTime     float32 `parquet:"last_lap_s"`
	BestLapTime     float32 `parquet:"best_lap_s"`
	CurrentRaceTime float32 `parquet:"current_race_s"`

	EngineRPM     float32 `parquet:"engine_rpm"`
	EngineMaxRPM  float32 `parquet:"engine_max_rpm"`
	EngineIdleRPM float32 `parquet:"engine_idle_rpm"`
	Power         float32 `parquet:"power_w"`
	Torque        float32 `parquet:"torque_nm"`
	Boost         float32 `parquet:"boost"`
	Fuel          float32 `parquet:"fuel"`

	CarOrdinal          int32 `parquet:"car_ordinal"`
	CarClass            int32 `parquet:"car_class"`
	CarPerformanceIndex int32 `parquet:"car_pi"`
	DrivetrainType      int32 `parquet:"drivetrain"`
	NumCylinders        int32 `parquet:"num_cylinders"`
	CarGroup            int32 `parquet:"car_group"`

	SmashableVelDiff float32 `parquet:"smashable_vel_diff"`
	SmashableMass    float32 `parquet:"smashable_mass"`

	PositionX float32 `parquet:"pos_x"`
	PositionY float32 `parquet:"pos_y"`
	PositionZ float32 `parquet:"pos_z"`

	VelocityX float32 `parquet:"vel_x"`
	VelocityY float32 `parquet:"vel_y"`
	VelocityZ float32 `parquet:"vel_z"`

	AccelerationX float32 `parquet:"acc_x"`
	AccelerationY float32 `parquet:"acc_y"`
	AccelerationZ float32 `parquet:"acc_z"`

	AngularVelocityX float32 `parquet:"avel_x"`
	AngularVelocityY float32 `parquet:"avel_y"`
	AngularVelocityZ float32 `parquet:"avel_z"`

	Yaw   float32 `parquet:"yaw"`
	Pitch float32 `parquet:"pitch"`
	Roll  float32 `parquet:"roll"`

	Speed            float32 `parquet:"speed_ms"`
	DistanceTraveled float32 `parquet:"distance_m"`

	Accel     uint8 `parquet:"accel"`
	Brake     uint8 `parquet:"brake"`
	Clutch    uint8 `parquet:"clutch"`
	HandBrake uint8 `parquet:"handbrake"`
	Gear      uint8 `parquet:"gear"`
	Steer     int8  `parquet:"steer"`

	TireSlipRatio        []float32 `parquet:"tire_slip_ratio"`
	TireSlipAngle        []float32 `parquet:"tire_slip_angle"`
	TireCombinedSlip     []float32 `parquet:"tire_combined_slip"`
	TireTemp             []float32 `parquet:"tire_temp"`
	WheelRotationSpeed   []float32 `parquet:"wheel_rot_speed"`
	WheelOnRumbleStrip   []float32 `parquet:"wheel_rumble"`
	WheelInPuddleDepth   []float32 `parquet:"wheel_puddle"`
	SurfaceRumble        []float32 `parquet:"surface_rumble"`
	NormSuspensionTravel []float32 `parquet:"susp_travel_norm"`
	SuspensionTravelM    []float32 `parquet:"susp_travel_m"`

	LateralG      float32 `parquet:"lateral_g"`
	LongitudinalG float32 `parquet:"longitudinal_g"`
	ThrottlePct   float32 `parquet:"throttle_pct"`
	BrakePct      float32 `parquet:"brake_pct"`
	RPMPct        float32 `parquet:"rpm_pct"`
	GearShift     bool    `parquet:"gear_shift"`
	LapDistanceM  float32 `parquet:"lap_distance_m"`
}

func toParquetRow(t *tick.Tick) parquetRow {
	return parquetRow{
		GameVersion:          uint8(t.GameVersion),
		PacketVariant:        uint8(t.PacketVariant),
		GameTSMillis:         t.GameTSMillis,
		ServerRecvNS:         t.ServerRecvNS,
		IsRaceOn:             t.IsRaceOn,
		LapNumber:            t.LapNumber,
		RacePosition:         t.RacePosition,
		CurrentLapTime:       t.CurrentLapTime,
		LastLapTime:          t.LastLapTime,
		BestLapTime:          t.BestLapTime,
		CurrentRaceTime:      t.CurrentRaceTime,
		EngineRPM:            t.EngineRPM,
		EngineMaxRPM:         t.EngineMaxRPM,
		EngineIdleRPM:        t.EngineIdleRPM,
		Power:                t.Power,
		Torque:               t.Torque,
		Boost:                t.Boost,
		Fuel:                 t.Fuel,
		CarOrdinal:           t.CarOrdinal,
		CarClass:             t.CarClass,
		CarPerformanceIndex:  t.CarPerformanceIndex,
		DrivetrainType:       t.DrivetrainType,
		NumCylinders:         t.NumCylinders,
		CarGroup:             t.CarGroup,
		SmashableVelDiff:     t.SmashableVelDiff,
		SmashableMass:        t.SmashableMass,
		PositionX:            t.PositionX,
		PositionY:            t.PositionY,
		PositionZ:            t.PositionZ,
		VelocityX:            t.VelocityX,
		VelocityY:            t.VelocityY,
		VelocityZ:            t.VelocityZ,
		AccelerationX:        t.AccelerationX,
		AccelerationY:        t.AccelerationY,
		AccelerationZ:        t.AccelerationZ,
		AngularVelocityX:     t.AngularVelocityX,
		AngularVelocityY:     t.AngularVelocityY,
		AngularVelocityZ:     t.AngularVelocityZ,
		Yaw:                  t.Yaw,
		Pitch:                t.Pitch,
		Roll:                 t.Roll,
		Speed:                t.Speed,
		DistanceTraveled:     t.DistanceTraveled,
		Accel:                t.Accel,
		Brake:                t.Brake,
		Clutch:               t.Clutch,
		HandBrake:            t.HandBrake,
		Gear:                 t.Gear,
		Steer:                t.Steer,
		TireSlipRatio:        t.TireSlipRatio[:],
		TireSlipAngle:        t.TireSlipAngle[:],
		TireCombinedSlip:     t.TireCombinedSlip[:],
		TireTemp:             t.TireTemp[:],
		WheelRotationSpeed:   t.WheelRotationSpeed[:],
		WheelOnRumbleStrip:   t.WheelOnRumbleStrip[:],
		WheelInPuddleDepth:   t.WheelInPuddleDepth[:],
		SurfaceRumble:        t.SurfaceRumble[:],
		NormSuspensionTravel: t.NormSuspensionTravel[:],
		SuspensionTravelM:    t.SuspensionTravelM[:],
		LateralG:             t.LateralG,
		LongitudinalG:        t.LongitudinalG,
		ThrottlePct:          t.ThrottlePct,
		BrakePct:             t.BrakePct,
		RPMPct:               t.RPMPct,
		GearShift:            t.GearShift,
		LapDistanceM:         t.LapDistanceM,
	}
}
