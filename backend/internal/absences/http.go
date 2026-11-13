package absences

import (
	"encoding/json"
	"errors"
	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
	"net/http"
)

func (s *Service) RegisterHTTP(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("GET /api/request-types", protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Types(r.Context())
		respond(w, v, e)
	})))
	mux.Handle("GET /api/requests", protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := auth.ClaimsFromContext(r.Context())
		if e != nil {
			http.Error(w, "unauthorized", 401)
			return
		}
		v, e := s.List(r.Context(), c.UserID)
		respond(w, v, e)
	})))
	mux.Handle("POST /api/requests/time-off", protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := auth.ClaimsFromContext(r.Context())
		if e != nil {
			http.Error(w, "unauthorized", 401)
			return
		}
		var in Input
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
		d.DisallowUnknownFields()
		if e = d.Decode(&in); e != nil {
			http.Error(w, "invalid json", 400)
			return
		}
		v, e := s.Create(r.Context(), c.UserID, in)
		respond(w, v, e)
	})))
}
func respond(w http.ResponseWriter, v any, e error) {
	w.Header().Set("Content-Type", "application/json")
	if e != nil {
		code := 500
		msg := "request service unavailable"
		if errors.Is(e, ErrInvalid) {
			code = 422
			msg = e.Error()
		}
		if errors.Is(e, ErrConflict) {
			code = 409
			msg = e.Error()
		}
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"error": msg})
		return
	}
	json.NewEncoder(w).Encode(v)
}
