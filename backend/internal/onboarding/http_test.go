package onboarding

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
)

// newHandler: wrap кладёт в контекст claims стажёра 3, как это делает requireAuth.
func newHandler(store *fakeStore, authed bool) http.Handler {
	if !authed {
		return newHandlerAs(store, nil)
	}
	return newHandlerAs(store, &auth.Claims{UserID: 3, TeamID: 2, Role: "intern"})
}

// newHandlerAs — то же с произвольными claims (nil — без пользователя).
func newHandlerAs(store *fakeStore, claims *auth.Claims) http.Handler {
	mux := http.NewServeMux()
	wrap := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if claims != nil {
				r = r.WithContext(auth.WithClaims(r.Context(), claims))
			}
			next.ServeHTTP(w, r)
		})
	}
	NewHandlers(NewService(store)).Register(mux, wrap)
	return mux
}

var mentor = &auth.Claims{UserID: 1, TeamID: 2, Role: "mentor"}

func call(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestGetChecklistHTTP(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		store := &fakeStore{rows: sashaRows()}
		rec := call(newHandler(store, true), http.MethodGet, "/api/checklist", "")
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("status %d, ct %q", rec.Code, rec.Header().Get("Content-Type"))
		}
		var c Checklist
		if err := json.NewDecoder(rec.Body).Decode(&c); err != nil {
			t.Fatal(err)
		}
		if c.Progress.Percent != 36 || c.Next == nil || c.Next.Title != "Доступ к GitLab" || len(c.Stages) != 3 {
			t.Fatalf("checklist: %+v", c)
		}
		if store.got.userID != 3 {
			t.Fatalf("user from claims must be used: %d", store.got.userID)
		}
	})

	t.Run("empty checklist is [] and null next", func(t *testing.T) {
		rec := call(newHandler(&fakeStore{}, true), http.MethodGet, "/api/checklist", "")
		body := rec.Body.String()
		if rec.Code != http.StatusOK || !strings.Contains(body, `"stages":[]`) || !strings.Contains(body, `"next":null`) {
			t.Fatalf("status %d body %s", rec.Code, body)
		}
	})

	t.Run("no claims", func(t *testing.T) {
		if rec := call(newHandler(&fakeStore{}, false), http.MethodGet, "/api/checklist", ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("status %d", rec.Code)
		}
	})

	t.Run("store error", func(t *testing.T) {
		rec := call(newHandler(&fakeStore{ensureErr: errDB}, true), http.MethodGet, "/api/checklist", "")
		if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "db down") {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
	})
}

func TestSetStatusHTTP(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		rows := sashaRows()
		rows[4].Task.Status = StatusReview
		store := &fakeStore{rows: rows, task: Task{ID: 16, Title: "Доступ к GitLab", Status: StatusReview}}
		rec := call(newHandler(store, true), http.MethodPatch, "/api/checklist/tasks/16", `{"status":"done"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
		var res UpdateResult
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatal(err)
		}
		if res.Task.ID != 16 || res.Task.Status != StatusReview || res.Progress != (Progress{Done: 4, Total: 11, Percent: 36}) {
			t.Fatalf("result: %+v", res)
		}
		if store.got.userID != 3 || store.got.taskID != 16 || store.got.status != StatusReview {
			t.Fatalf("got %+v", store.got)
		}
	})

	cases := []struct {
		name   string
		store  *fakeStore
		authed bool
		path   string
		body   string
		want   int
	}{
		{"no claims", &fakeStore{}, false, "/api/checklist/tasks/16", `{"status":"done"}`, http.StatusUnauthorized},
		{"bad id", &fakeStore{}, true, "/api/checklist/tasks/abc", `{"status":"done"}`, http.StatusBadRequest},
		{"zero id", &fakeStore{}, true, "/api/checklist/tasks/0", `{"status":"done"}`, http.StatusBadRequest},
		{"bad json", &fakeStore{}, true, "/api/checklist/tasks/16", `{"status":`, http.StatusBadRequest},
		{"too large body", &fakeStore{}, true, "/api/checklist/tasks/16", `{"status":"` + strings.Repeat("x", 2048) + `"}`, http.StatusBadRequest},
		{"invalid status", &fakeStore{}, true, "/api/checklist/tasks/16", `{"status":"closed"}`, http.StatusBadRequest},
		{"foreign task", &fakeStore{updateErr: ErrNotFound}, true, "/api/checklist/tasks/99", `{"status":"done"}`, http.StatusNotFound},
		{"already approved", &fakeStore{updateErr: ErrAlreadyApproved}, true, "/api/checklist/tasks/16", `{"status":"in_progress"}`, http.StatusConflict},
		{"db error", &fakeStore{updateErr: errDB}, true, "/api/checklist/tasks/16", `{"status":"done"}`, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := call(newHandler(tc.store, tc.authed), http.MethodPatch, tc.path, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status %d, want %d, body %s", rec.Code, tc.want, rec.Body)
			}
			var e map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&e); err != nil || e["error"] == "" {
				t.Fatalf("error body must be json: %s", rec.Body)
			}
		})
	}
}

func TestRegisterNilWrap(t *testing.T) {
	// Без wrap маршруты регистрируются, но без claims хендлер отвечает 401, а не паникует.
	mux := http.NewServeMux()
	NewHandlers(NewService(&fakeStore{})).Register(mux, nil)
	if rec := call(mux, http.MethodGet, "/api/checklist", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestListReviewsHTTP(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		store := &fakeStore{reviews: []Review{{TaskID: 17, TaskTitle: "Доступ к Jira", Intern: Intern{ID: 3, FullName: "Саша Иванов"}}}}
		rec := call(newHandlerAs(store, mentor), http.MethodGet, "/api/reviews", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
		var body struct {
			Reviews []Review `json:"reviews"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil || len(body.Reviews) != 1 || body.Reviews[0].Intern.FullName != "Саша Иванов" {
			t.Fatalf("body %+v err %v", body, err)
		}
		if store.got.mentorID != 1 {
			t.Fatalf("mentor from claims must be used: %d", store.got.mentorID)
		}
	})

	t.Run("empty list is []", func(t *testing.T) {
		rec := call(newHandlerAs(&fakeStore{reviews: []Review{}}, mentor), http.MethodGet, "/api/reviews", "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"reviews":[]`) {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
	})

	t.Run("store error is hidden", func(t *testing.T) {
		rec := call(newHandlerAs(&fakeStore{reviewsErr: errDB}, mentor), http.MethodGet, "/api/reviews", "")
		if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "db down") {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
	})
}

func TestListInternsHTTP(t *testing.T) {
	t.Run("ok: flat json with percent", func(t *testing.T) {
		store := &fakeStore{interns: []InternProgress{
			{Intern: Intern{ID: 3, FullName: "Саша Иванов"}, Progress: Progress{Done: 5, Total: 11}, InReview: 1, Stage: "Доступы и инструменты"},
		}}
		rec := call(newHandlerAs(store, mentor), http.MethodGet, "/api/interns", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
		want := `{"interns":[{"id":3,"full_name":"Саша Иванов","done":5,"total":11,"percent":45,"in_review":1,"stage":"Доступы и инструменты"}]}`
		if got := strings.TrimSpace(rec.Body.String()); got != want {
			t.Fatalf("body\n got %s\nwant %s", got, want)
		}
		if store.got.mentorID != 1 {
			t.Fatalf("mentor from claims must be used: %d", store.got.mentorID)
		}
	})

	t.Run("empty list is []", func(t *testing.T) {
		rec := call(newHandlerAs(&fakeStore{interns: []InternProgress{}}, mentor), http.MethodGet, "/api/interns", "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"interns":[]`) {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
	})

	t.Run("store error is hidden", func(t *testing.T) {
		rec := call(newHandlerAs(&fakeStore{internsErr: errDB}, mentor), http.MethodGet, "/api/interns", "")
		if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "db down") {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
	})

	t.Run("no user is 401", func(t *testing.T) {
		if rec := call(newHandler(&fakeStore{}, false), http.MethodGet, "/api/interns", ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("status %d", rec.Code)
		}
	})

	t.Run("intern is 403 and store is not called", func(t *testing.T) {
		store := &fakeStore{}
		rec := call(newHandler(store, true), http.MethodGet, "/api/interns", "")
		if rec.Code != http.StatusForbidden || len(store.calls) != 0 {
			t.Fatalf("status %d calls %v", rec.Code, store.calls)
		}
	})
}

func TestReviewActionsHTTP(t *testing.T) {
	t.Run("approve ok", func(t *testing.T) {
		store := &fakeStore{}
		rec := call(newHandlerAs(store, mentor), http.MethodPost, "/api/reviews/17/approve", "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"done"`) || !strings.Contains(rec.Body.String(), `"task_id":17`) {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
		if store.got.mentorID != 1 || store.got.taskID != 17 {
			t.Fatalf("got %+v", store.got)
		}
	})

	t.Run("reject ok", func(t *testing.T) {
		store := &fakeStore{}
		rec := call(newHandlerAs(store, mentor), http.MethodPost, "/api/reviews/17/reject", `{"comment":" Нет доступа "}`)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"in_progress"`) {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
		if store.got.comment != "Нет доступа" || store.got.taskID != 17 {
			t.Fatalf("got %+v", store.got)
		}
	})

	t.Run("reject: 500 cyrillic chars fit the body limit", func(t *testing.T) {
		body := `{"comment":"` + strings.Repeat("я\\n", MaxCommentLen/2) + `"}`
		rec := call(newHandlerAs(&fakeStore{}, mentor), http.MethodPost, "/api/reviews/17/reject", body)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
	})

	intern := &auth.Claims{UserID: 3, TeamID: 2, Role: "intern"}
	cases := []struct {
		name   string
		store  *fakeStore
		claims *auth.Claims
		method string
		path   string
		body   string
		want   int
	}{
		{"list no claims", &fakeStore{}, nil, http.MethodGet, "/api/reviews", "", http.StatusUnauthorized},
		{"list not mentor", &fakeStore{}, intern, http.MethodGet, "/api/reviews", "", http.StatusForbidden},
		{"approve no claims", &fakeStore{}, nil, http.MethodPost, "/api/reviews/17/approve", "", http.StatusUnauthorized},
		{"approve not mentor", &fakeStore{}, intern, http.MethodPost, "/api/reviews/17/approve", "", http.StatusForbidden},
		{"approve bad id", &fakeStore{}, mentor, http.MethodPost, "/api/reviews/abc/approve", "", http.StatusBadRequest},
		{"approve zero id", &fakeStore{}, mentor, http.MethodPost, "/api/reviews/0/approve", "", http.StatusBadRequest},
		{"approve not found", &fakeStore{approveErr: ErrNotFound}, mentor, http.MethodPost, "/api/reviews/17/approve", "", http.StatusNotFound},
		{"approve not in review", &fakeStore{approveErr: ErrNotInReview}, mentor, http.MethodPost, "/api/reviews/17/approve", "", http.StatusConflict},
		{"approve db error", &fakeStore{approveErr: errDB}, mentor, http.MethodPost, "/api/reviews/17/approve", "", http.StatusInternalServerError},
		{"reject not mentor", &fakeStore{}, intern, http.MethodPost, "/api/reviews/17/reject", `{"comment":"x"}`, http.StatusForbidden},
		{"reject bad id", &fakeStore{}, mentor, http.MethodPost, "/api/reviews/-1/reject", `{"comment":"x"}`, http.StatusBadRequest},
		{"reject bad json", &fakeStore{}, mentor, http.MethodPost, "/api/reviews/17/reject", `{"comment":`, http.StatusBadRequest},
		{"reject too large body", &fakeStore{}, mentor, http.MethodPost, "/api/reviews/17/reject", `{"comment":"` + strings.Repeat("x", 5000) + `"}`, http.StatusBadRequest},
		{"reject empty comment", &fakeStore{}, mentor, http.MethodPost, "/api/reviews/17/reject", `{"comment":"  "}`, http.StatusUnprocessableEntity},
		{"reject no comment field", &fakeStore{}, mentor, http.MethodPost, "/api/reviews/17/reject", `{}`, http.StatusUnprocessableEntity},
		{"reject too long comment", &fakeStore{}, mentor, http.MethodPost, "/api/reviews/17/reject", `{"comment":"` + strings.Repeat("я", MaxCommentLen+1) + `"}`, http.StatusUnprocessableEntity},
		{"reject not found", &fakeStore{rejectErr: ErrNotFound}, mentor, http.MethodPost, "/api/reviews/17/reject", `{"comment":"x"}`, http.StatusNotFound},
		{"reject not in review", &fakeStore{rejectErr: ErrNotInReview}, mentor, http.MethodPost, "/api/reviews/17/reject", `{"comment":"x"}`, http.StatusConflict},
		{"reject db error", &fakeStore{rejectErr: errDB}, mentor, http.MethodPost, "/api/reviews/17/reject", `{"comment":"x"}`, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := call(newHandlerAs(tc.store, tc.claims), tc.method, tc.path, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status %d, want %d, body %s", rec.Code, tc.want, rec.Body)
			}
			var e map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&e); err != nil || e["error"] == "" || strings.Contains(e["error"], "db down") {
				t.Fatalf("error body must be safe json: %s", rec.Body)
			}
		})
	}
}
