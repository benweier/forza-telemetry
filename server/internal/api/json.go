package api

import (
	"encoding/json"
	"errors"
	"net/http"
)

// writeJSON serialises body as JSON with the given HTTP status.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError emits a uniform `{"error": "..."}` JSON body.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// internalError logs the underlying error and returns a generic 500. Avoid
// leaking SQL or filesystem details to clients.
func (s *Server) internalError(w http.ResponseWriter, op string, err error) {
	s.logger.Error("rest handler", "op", op, "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

// nullableInt64 is a JSON-friendly nullable integer. SQL NULL becomes JSON
// null; non-null serialises as an integer.
type nullableInt64 struct {
	Value int64
	Valid bool
}

func (n nullableInt64) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(n.Value)
}

func (n *nullableInt64) Scan(src any) error {
	if src == nil {
		n.Valid = false
		return nil
	}
	switch v := src.(type) {
	case int64:
		n.Value = v
	case int32:
		n.Value = int64(v)
	default:
		return errors.New("nullableInt64: unsupported source type")
	}
	n.Valid = true
	return nil
}

// nullableString mirrors nullableInt64 for nullable TEXT columns.
type nullableString struct {
	Value string
	Valid bool
}

func (n nullableString) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(n.Value)
}

func (n *nullableString) Scan(src any) error {
	if src == nil {
		n.Valid = false
		return nil
	}
	switch v := src.(type) {
	case string:
		n.Value = v
	case []byte:
		n.Value = string(v)
	default:
		return errors.New("nullableString: unsupported source type")
	}
	n.Valid = true
	return nil
}

// nullableFloat64 mirrors nullableInt64 for nullable DOUBLE columns.
type nullableFloat64 struct {
	Value float64
	Valid bool
}

func (n nullableFloat64) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(n.Value)
}

func (n *nullableFloat64) Scan(src any) error {
	if src == nil {
		n.Valid = false
		return nil
	}
	switch v := src.(type) {
	case float64:
		n.Value = v
	case float32:
		n.Value = float64(v)
	default:
		return errors.New("nullableFloat64: unsupported source type")
	}
	n.Valid = true
	return nil
}
