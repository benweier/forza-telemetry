package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/benweier/forza-telemetry/server/internal/storage"
)

type sessionRow struct {
	ID          string        `json:"id"`
	StartedAtNS int64         `json:"started_at_ns"`
	EndedAtNS   nullableInt64 `json:"ended_at_ns"`
	Pinned      bool          `json:"pinned"`
	Downsampled bool          `json:"downsampled"`
	StintCount  int           `json:"stint_count"`
}

func (s *Server) handleListSessions(w http.ResponseWriter, _ *http.Request) {
	rows, err := s.store.DB().Query(`
		SELECT s.id, s.started_at_ns, s.ended_at_ns, s.pinned, s.downsampled,
		       (SELECT COUNT(*) FROM stints st WHERE st.session_id = s.id) AS stint_count
		FROM sessions s
		ORDER BY s.started_at_ns DESC
	`)
	if err != nil {
		s.internalError(w, "list_sessions", err)
		return
	}
	defer rows.Close()

	out := []sessionRow{}
	for rows.Next() {
		var r sessionRow
		if err := rows.Scan(&r.ID, &r.StartedAtNS, &r.EndedAtNS, &r.Pinned, &r.Downsampled, &r.StintCount); err != nil {
			s.internalError(w, "list_sessions scan", err)
			return
		}
		out = append(out, r)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sessions": out,
		"total":    len(out),
	})
}

type sessionDetail struct {
	sessionRow
	Stints []stintListRow `json:"stints"`
}

type stintListRow struct {
	ID          string        `json:"id"`
	Ordinal     int           `json:"ordinal"`
	StartedAtNS int64         `json:"started_at_ns"`
	EndedAtNS   nullableInt64 `json:"ended_at_ns"`
	TickCount   int64         `json:"tick_count"`
	StintType   nullableString `json:"stint_type"`
	CarOrdinal  nullableInt64 `json:"car_ordinal"`
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var detail sessionDetail
	err := s.store.DB().QueryRow(`
		SELECT s.id, s.started_at_ns, s.ended_at_ns, s.pinned, s.downsampled,
		       (SELECT COUNT(*) FROM stints st WHERE st.session_id = s.id) AS stint_count
		FROM sessions s WHERE s.id = ?`, id,
	).Scan(&detail.ID, &detail.StartedAtNS, &detail.EndedAtNS, &detail.Pinned, &detail.Downsampled, &detail.StintCount)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if err != nil {
		s.internalError(w, "get_session", err)
		return
	}

	rows, err := s.store.DB().Query(`
		SELECT id, ordinal, started_at_ns, ended_at_ns, tick_count, stint_type, car_ordinal
		FROM stints WHERE session_id = ? ORDER BY ordinal`, id)
	if err != nil {
		s.internalError(w, "get_session stints", err)
		return
	}
	defer rows.Close()
	detail.Stints = []stintListRow{}
	for rows.Next() {
		var r stintListRow
		if err := rows.Scan(&r.ID, &r.Ordinal, &r.StartedAtNS, &r.EndedAtNS, &r.TickCount, &r.StintType, &r.CarOrdinal); err != nil {
			s.internalError(w, "get_session stint scan", err)
			return
		}
		detail.Stints = append(detail.Stints, r)
	}
	writeJSON(w, http.StatusOK, detail)
}

type patchSessionBody struct {
	Pinned *bool `json:"pinned"`
}

// handlePatchSession applies partial updates to a session row. Currently only
// `pinned` is mutable from the API — server-managed fields (started/ended_at,
// downsampled) are read-only. Body is JSON: `{"pinned": true}`.
func (s *Server) handlePatchSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body patchSessionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Pinned == nil {
		writeError(w, http.StatusBadRequest, "no mutable fields supplied")
		return
	}
	res, err := s.store.DB().Exec(`UPDATE sessions SET pinned = ? WHERE id = ?`, *body.Pinned, id)
	if err != nil {
		s.internalError(w, "patch_session", err)
		return
	}
	n, err := res.RowsAffected()
	if err != nil {
		s.internalError(w, "patch_session rows", err)
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	s.handleGetSession(w, r)
}

// handleDownsampleSession is the action endpoint for the user-triggered
// downsample (ADR 0002). The actual Parquet rewrite is deferred — for now
// this returns 501 with a clear marker so the UI can wire its affordance
// against the eventual endpoint shape.
func (s *Server) handleDownsampleSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var exists bool
	err := s.store.DB().QueryRow(`SELECT TRUE FROM sessions WHERE id = ?`, id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if err != nil {
		s.internalError(w, "downsample lookup", err)
		return
	}
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"error": "downsample action not yet implemented",
		"note":  "the endpoint shape is stable; backend job lands in handoff #9",
	})
}

// handleDeleteSession removes a session and every stint beneath it (child rows +
// Parquet files). Refuses the actively-recording session with 409.
func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	switch err := s.store.DeleteSession(id); {
	case errors.Is(err, storage.ErrNotFound):
		writeError(w, http.StatusNotFound, "session not found")
	case errors.Is(err, storage.ErrActive):
		writeError(w, http.StatusConflict, "cannot delete a session that is still recording")
	case err != nil:
		s.internalError(w, "delete_session", err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
	}
}
