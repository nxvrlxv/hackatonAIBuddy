package onboarding

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
)

// Handlers — REST чек-листа и проверки задач для фронта. Контракт: docs/approvals-contract.md.
//
//	GET   /api/checklist                  → 200 Checklist
//	PATCH /api/checklist/tasks/{id}       {"status":"review"} → 200 {"task":…, "progress":…} (done от стажёра = review)
//	GET   /api/reviews                    → 200 {"reviews":[Review]} — задачи на проверке у стажёров наставника
//	POST  /api/reviews/{id}/approve       → 200 {"task_id":…, "status":"done"}
//	POST  /api/reviews/{id}/reject        {"comment":"…"} → 200 {"task_id":…, "status":"in_progress"}
//	GET   /api/interns                    → 200 {"interns":[InternProgress]} — прогресс стажёров наставника
//
// Ошибки: {"error":"..."}; 400 — неверный id/json/статус, 401 — нет пользователя, 403 — ручки проверки не для наставника,
// 404 — чужая или несуществующая задача, 409 — задача уже принята / не на проверке, 422 — нет комментария при отклонении.
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
	mux.Handle("GET /api/reviews", wrap(http.HandlerFunc(h.handleListReviews)))
	mux.Handle("POST /api/reviews/{id}/approve", wrap(http.HandlerFunc(h.handleApprove)))
	mux.Handle("POST /api/reviews/{id}/reject", wrap(http.HandlerFunc(h.handleReject)))
	mux.Handle("GET /api/interns", wrap(http.HandlerFunc(h.handleListInterns)))
}

// reviewerClaims — пользователь из токена, если он наставник. Иначе пишет 401/403 и возвращает nil.
// Главная защита всё равно в SQL (mentor_id = я): чужую задачу не принять и с ролью mentor.
func reviewerClaims(w http.ResponseWriter, r *http.Request) *auth.Claims {
	claims, err := auth.ClaimsFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return nil
	}
	if claims.Role != "mentor" {
		writeError(w, http.StatusForbidden, "forbidden")
		return nil
	}
	return claims
}

// pathTaskID — id задачи из пути; неверный или неположительный → 400.
func pathTaskID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return 0, false
	}
	return id, true
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
		writeError(w, http.StatusBadRequest, "invalid status: use todo, in_progress, review or done")
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "task not found")
	case errors.Is(err, ErrAlreadyApproved):
		writeError(w, http.StatusConflict, "task already approved")
	default:
		log.Printf("set status user %d task %d: %v", claims.UserID, taskID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func (h *Handlers) handleListReviews(w http.ResponseWriter, r *http.Request) {
	claims := reviewerClaims(w, r)
	if claims == nil {
		return
	}
	reviews, err := h.svc.Reviews(r.Context(), claims.UserID)
	if err != nil {
		log.Printf("list reviews mentor %d: %v", claims.UserID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reviews": reviews})
}

func (h *Handlers) handleListInterns(w http.ResponseWriter, r *http.Request) {
	claims := reviewerClaims(w, r)
	if claims == nil {
		return
	}
	interns, err := h.svc.Interns(r.Context(), claims.UserID)
	if err != nil {
		log.Printf("list interns mentor %d: %v", claims.UserID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"interns": interns})
}

func (h *Handlers) handleApprove(w http.ResponseWriter, r *http.Request) {
	claims := reviewerClaims(w, r)
	if claims == nil {
		return
	}
	taskID, ok := pathTaskID(w, r)
	if !ok {
		return
	}
	err := h.svc.Approve(r.Context(), claims.UserID, taskID)
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"task_id": taskID, "status": StatusDone})
		return
	}
	writeReviewError(w, "approve", claims.UserID, taskID, err)
}

type rejectRequest struct {
	Comment string `json:"comment"`
}

func (h *Handlers) handleReject(w http.ResponseWriter, r *http.Request) {
	claims := reviewerClaims(w, r)
	if claims == nil {
		return
	}
	taskID, ok := pathTaskID(w, r)
	if !ok {
		return
	}
	// 4 КБ: 500 символов кириллицы с переносами и эмодзи в JSON занимают больше 1 КБ.
	var req rejectRequest
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	err := h.svc.Reject(r.Context(), claims.UserID, taskID, req.Comment)
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"task_id": taskID, "status": StatusInProgress})
		return
	}
	writeReviewError(w, "reject", claims.UserID, taskID, err)
}

// writeReviewError переводит ошибки approve/reject в HTTP. Комментарий наставника в лог не пишем.
func writeReviewError(w http.ResponseWriter, op string, mentorID, taskID int64, err error) {
	switch {
	case errors.Is(err, ErrCommentRequired), errors.Is(err, ErrCommentTooLong):
		writeError(w, http.StatusUnprocessableEntity, "comment is required")
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, ErrNotInReview):
		writeError(w, http.StatusConflict, "task is not in review")
	default:
		log.Printf("%s mentor %d task %d: %v", op, mentorID, taskID, err)
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
