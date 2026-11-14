package onboarding

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
)

// Handlers — REST чек-листа для фронта.
//
//	GET   /api/checklist             → 200 Checklist
//	PATCH /api/checklist/tasks/{id}  {"status":"done"} → 200 {"task":…, "progress":…}
//
// Ошибки: {"error":"..."}; 400 — неверный id/json/статус, 401 — нет пользователя, 404 — чужая или несуществующая задача.
type Handlers struct {
	svc *Service
}

func NewHandlers(svc *Service) *Handlers {
	return &Handlers{svc: svc}
}

// Register вешает маршруты; wrap — проверка токена (httpapi.requireAuth).
func (h *Handlers) Register(mux *http.ServeMux, wrap func(http.Handler) http.Handler) {
	if wrap == nil {
		wrap = func(next http.Handler) http.Handler { return next }
	}
	mux.Handle("GET /api/checklist", wrap(http.HandlerFunc(h.handleChecklist)))
	mux.Handle("PATCH /api/checklist/tasks/{id}", wrap(http.HandlerFunc(h.handleSetStatus)))
}

func (h *Handlers) handleChecklist(w http.ResponseWriter, r *http.Request) {
	claims, err := auth.ClaimsFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	c, err := h.svc.Checklist(r.Context(), claims.UserID)
	if err != nil {
		log.Printf("checklist user %d: %v", claims.UserID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

type setStatusRequest struct {
	Status string `json:"status"`
}

func (h *Handlers) handleSetStatus(w http.ResponseWriter, r *http.Request) {
	claims, err := auth.ClaimsFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	taskID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || taskID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}
	var req setStatusRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	res, err := h.svc.SetStatus(r.Context(), claims.UserID, taskID, Status(req.Status))
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, res)
	case errors.Is(err, ErrInvalidStatus):
		writeError(w, http.StatusBadRequest, "invalid status: use todo, in_progress or done")
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "task not found")
	default:
		log.Printf("set status user %d task %d: %v", claims.UserID, taskID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
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
