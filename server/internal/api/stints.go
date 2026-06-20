package api

import (
	"database/sql"
	"errors"
	"net/http"
)

type stintDetail struct {
	ID                string         `json:"id"`
	SessionID         string         `json:"session_id"`
	Ordinal           int            `json:"ordinal"`
	StartedAtNS       int64          `json:"started_at_ns"`
	EndedAtNS         nullableInt64  `json:"ended_at_ns"`
	FirstGameTSMillis nullableInt64  `json:"first_game_ts_ms"`
	LastGameTSMillis  nullableInt64  `json:"last_game_ts_ms"`
	TickCount         int64          `json:"tick_count"`
	StintType         nullableString `json:"stint_type"`
	Car               carIdentity    `json:"car"`
	Summary           *stintSummary  `json:"summary"`

	// parquetPath is needed for tick-series queries but never serialised —
	// it is a server filesystem detail and must not leak to clients.
	parquetPath string `json:"-"`
}

type carIdentity struct {
	Ordinal          nullableInt64 `json:"ordinal"`
	Class            nullableInt64 `json:"class"`
	PerformanceIndex nullableInt64 `json:"performance_index"`
}

type stintSummary struct {
	TopSpeedMS     nullableFloat64 `json:"top_speed_ms"`
	DistanceM      nullableFloat64 `json:"distance_m"`
	AvgSpeedMS     nullableFloat64 `json:"avg_speed_ms"`
	MaxRPM         nullableFloat64 `json:"max_rpm"`
	PeakLateralG   nullableFloat64 `json:"peak_lateral_g"`
	PeakLongG      nullableFloat64 `json:"peak_long_g"`
	PeakBrakePct   nullableFloat64 `json:"peak_brake_pct"`
	GearShiftCount nullableInt64   `json:"gear_shift_count"`
}

func (s *Server) handleGetStint(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var d stintDetail
	err := s.store.DB().QueryRow(`
		SELECT id, session_id, ordinal, started_at_ns, ended_at_ns,
		       first_game_ts_ms, last_game_ts_ms, tick_count, stint_type,
		       car_ordinal, car_class, car_pi, parquet_path
		FROM stints WHERE id = ?`, id,
	).Scan(&d.ID, &d.SessionID, &d.Ordinal, &d.StartedAtNS, &d.EndedAtNS,
		&d.FirstGameTSMillis, &d.LastGameTSMillis, &d.TickCount, &d.StintType,
		&d.Car.Ordinal, &d.Car.Class, &d.Car.PerformanceIndex, &d.parquetPath,
	)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "stint not found")
		return
	}
	if err != nil {
		s.internalError(w, "get_stint", err)
		return
	}

	var sum stintSummary
	err = s.store.DB().QueryRow(`
		SELECT top_speed_ms, distance_m, avg_speed_ms, max_rpm,
		       peak_lateral_g, peak_long_g, peak_brake_pct, gear_shift_count
		FROM stint_summary WHERE stint_id = ?`, id,
	).Scan(&sum.TopSpeedMS, &sum.DistanceM, &sum.AvgSpeedMS, &sum.MaxRPM,
		&sum.PeakLateralG, &sum.PeakLongG, &sum.PeakBrakePct, &sum.GearShiftCount)
	if err == nil {
		d.Summary = &sum
	} else if !errors.Is(err, sql.ErrNoRows) {
		s.internalError(w, "get_stint summary", err)
		return
	}

	writeJSON(w, http.StatusOK, d)
}

type lapRow struct {
	LapNumber    int             `json:"lap_number"`
	LapTimeS     nullableFloat64 `json:"lap_time_s"`
	TopSpeedMS   nullableFloat64 `json:"top_speed_ms"`
	DistanceM    nullableFloat64 `json:"distance_m"`
	PeakLateralG nullableFloat64 `json:"peak_lateral_g"`
	PeakBrakePct nullableFloat64 `json:"peak_brake_pct"`
}

func (s *Server) handleListLaps(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rows, err := s.store.DB().Query(`
		SELECT lap_number, lap_time_s, top_speed_ms, distance_m, peak_lateral_g, peak_brake_pct
		FROM lap_summary WHERE stint_id = ? ORDER BY lap_number`, id)
	if err != nil {
		s.internalError(w, "list_laps", err)
		return
	}
	defer rows.Close()
	out := []lapRow{}
	for rows.Next() {
		var l lapRow
		if err := rows.Scan(&l.LapNumber, &l.LapTimeS, &l.TopSpeedMS, &l.DistanceM, &l.PeakLateralG, &l.PeakBrakePct); err != nil {
			s.internalError(w, "list_laps scan", err)
			return
		}
		out = append(out, l)
	}
	writeJSON(w, http.StatusOK, map[string]any{"laps": out})
}

type previewRow struct {
	SecondIndex    int             `json:"second_index"`
	TickNS         int64           `json:"tick_ns"`
	SpeedMS        nullableFloat64 `json:"speed_ms"`
	LateralG       nullableFloat64 `json:"lateral_g"`
	LongitudinalG  nullableFloat64 `json:"longitudinal_g"`
	ThrottlePct    nullableFloat64 `json:"throttle_pct"`
	BrakePct       nullableFloat64 `json:"brake_pct"`
	RPM            nullableFloat64 `json:"rpm"`
	PosX           nullableFloat64 `json:"pos_x"`
	PosY           nullableFloat64 `json:"pos_y"`
	PosZ           nullableFloat64 `json:"pos_z"`
	LapNumber      nullableInt64   `json:"lap_number"`
}

func (s *Server) handleListPreview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rows, err := s.store.DB().Query(`
		SELECT second_index, tick_ns, speed_ms, lateral_g, longitudinal_g,
		       throttle_pct, brake_pct, rpm, pos_x, pos_y, pos_z, lap_number
		FROM preview_samples WHERE stint_id = ? ORDER BY second_index`, id)
	if err != nil {
		s.internalError(w, "list_preview", err)
		return
	}
	defer rows.Close()
	out := []previewRow{}
	for rows.Next() {
		var p previewRow
		if err := rows.Scan(&p.SecondIndex, &p.TickNS, &p.SpeedMS, &p.LateralG, &p.LongitudinalG,
			&p.ThrottlePct, &p.BrakePct, &p.RPM, &p.PosX, &p.PosY, &p.PosZ, &p.LapNumber); err != nil {
			s.internalError(w, "list_preview scan", err)
			return
		}
		out = append(out, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"samples": out})
}
