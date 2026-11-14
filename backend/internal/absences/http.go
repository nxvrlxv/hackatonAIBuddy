package absences

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
)

// Register вешает REST-ручки заявок; wrap — проверка токена (httpapi.requireAuth).
//
//	GET  /api/request-types      → 200 [Type]
//	GET  /api/requests           → 200 [Request] (последние 100 своих)
//	POST /api/requests/time-off  {start_date, end_date, reason?, idempotency_key} → 200 Request
//
// Ошибки: {"error":"..."}; 400 — json, 401 — нет пользователя, 409 — конфликт, 422 — неверные данные.
func (s *Service) Register(mux *http.ServeMux, wrap func(http.Handler) http.Handler) {
	if wrap == nil {
		wrap = func(next http.Handler) http.Handler { return next }
	}
	mux.Handle("GET /api/request-types", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, e := auth.ClaimsFromContext(r.Context()); e != nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		v, e := s.Types(r.Context())
		respond(w, v, e)
	})))
	mux.Handle("GET /api/requests", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := auth.ClaimsFromContext(r.Context())
		if e != nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		v, e := s.List(r.Context(), c.UserID)
		respond(w, v, e)
	})))
	mux.Handle("POST /api/requests/time-off", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := auth.ClaimsFromContext(r.Context())
		if e != nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		var in Input
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
		d.DisallowUnknownFields()
		if e = d.Decode(&in); e != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		v, e := s.Create(r.Context(), c.UserID, in)
		respond(w, v, e)
	})))
}

func respond(w http.ResponseWriter, v any, e error) {
	switch {
	case e == nil:
		writeJSON(w, http.StatusOK, v)
	case errors.Is(e, ErrInvalid):
		writeError(w, http.StatusUnprocessableEntity, e.Error())
	case errors.Is(e, ErrConflict):
		writeError(w, http.StatusConflict, e.Error())
	default:
		log.Printf("requests: %v", e)
		writeError(w, http.StatusInternalServerError, "request service unavailable")
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write json: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
