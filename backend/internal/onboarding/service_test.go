package onboarding

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

// fakeStore записывает вызовы и возвращает заданные результаты.
type fakeStore struct {
	rows      []Row
	task      Task
	ensureErr error
	listErr   error
	updateErr error

	calls []string
	got   struct {
		userID, taskID int64
		status         Status
	}
}

func (f *fakeStore) EnsureTasks(_ context.Context, userID int64) error {
	f.calls = append(f.calls, "ensure")
	f.got.userID = userID
	return f.ensureErr
}

func (f *fakeStore) ListTasks(_ context.Context, userID int64) ([]Row, error) {
	f.calls = append(f.calls, "list")
	f.got.userID = userID
	return f.rows, f.listErr
}

func (f *fakeStore) UpdateStatus(_ context.Context, userID, taskID int64, status Status) (Task, error) {
	f.calls = append(f.calls, "update")
	f.got.userID, f.got.taskID, f.got.status = userID, taskID, status
	return f.task, f.updateErr
}

func row(stage, title string, id int64, taskTitle string, status Status) Row {
	r := Row{StageCode: stage, StageTitle: title, Task: Task{ID: id, Title: taskTitle, Status: status}}
	if status == StatusDone {
		now := time.Now()
		r.Task.CompletedAt = &now
	}
	return r
}

// sashaRows повторяет данные миграции 0005 для Саши: первый этап пройден, GitLab в работе.
func sashaRows() []Row {
	const team, tools, first = "Знакомство с командой", "Доступы и инструменты", "Первые задачи"
	return []Row{
		row("team", team, 20, "Встреча с наставником", StatusDone),
		row("team", team, 21, "Знакомство с командой", StatusDone),
		row("team", team, 22, "Миссия и ценности Авито", StatusDone),
		row("team", team, 12, "Обзор продукта Путешествий", StatusDone),
		row("tools", tools, 16, "Доступ к GitLab", StatusInProgress),
		row("tools", tools, 17, "Доступ к Jira", StatusTodo),
		row("tools", tools, 18, "Настроить VPN", StatusTodo),
		row("tools", tools, 19, "Почта и календарь", StatusTodo),
		row("first_tasks", first, 13, "Собрать проект локально", StatusTodo),
		row("first_tasks", first, 14, "Взять первую задачу", StatusTodo),
		row("first_tasks", first, 15, "Пройти первое ревью", StatusTodo),
	}
}

func TestBuild(t *testing.T) {
	t.Run("sasha", func(t *testing.T) {
		c := build(sashaRows())

		if c.Progress != (Progress{Done: 4, Total: 11, Percent: 36}) {
			t.Fatalf("progress: %+v", c.Progress)
		}
		if c.Next == nil || c.Next.ID != 16 || c.Next.Status != StatusInProgress {
			t.Fatalf("next must be GitLab (in_progress counts as not done): %+v", c.Next)
		}
		want := []struct {
			code        string
			done, total int
		}{{"team", 4, 4}, {"tools", 0, 4}, {"first_tasks", 0, 3}}
		if len(c.Stages) != len(want) {
			t.Fatalf("stages: %+v", c.Stages)
		}
		for i, w := range want {
			st := c.Stages[i]
			if st.Code != w.code || st.Done != w.done || st.Total != w.total || len(st.Tasks) != w.total {
				t.Fatalf("stage %d: %+v, want %+v", i, st, w)
			}
		}
		if c.Stages[0].Title != "Знакомство с командой" || c.Stages[2].Tasks[2].Title != "Пройти первое ревью" {
			t.Fatalf("titles or order broken: %+v", c.Stages)
		}
	})

	t.Run("next is a copy", func(t *testing.T) {
		rows := sashaRows()
		c := build(rows)
		c.Next.Title = "changed"
		if c.Stages[1].Tasks[0].Title != "Доступ к GitLab" || rows[4].Task.Title != "Доступ к GitLab" {
			t.Fatal("Next must not alias stage tasks or input rows")
		}
	})

	t.Run("all done", func(t *testing.T) {
		rows := []Row{
			row("team", "A", 1, "a", StatusDone),
			row("tools", "B", 2, "b", StatusDone),
		}
		c := build(rows)
		if c.Next != nil || c.Progress != (Progress{Done: 2, Total: 2, Percent: 100}) {
			t.Fatalf("all done: %+v", c)
		}
	})

	t.Run("empty (mentor)", func(t *testing.T) {
		c := build(nil)
		if c.Next != nil || c.Progress != (Progress{}) || c.Stages == nil || len(c.Stages) != 0 {
			t.Fatalf("empty: %+v", c)
		}
	})

	t.Run("percent rounds down", func(t *testing.T) {
		rows := []Row{
			row("team", "A", 1, "a", StatusDone),
			row("team", "A", 2, "b", StatusTodo),
			row("team", "A", 3, "c", StatusTodo),
		}
		if p := build(rows).Progress.Percent; p != 33 {
			t.Fatalf("percent = %d, want 33", p)
		}
	})
}

func TestChecklist(t *testing.T) {
	ctx := context.Background()

	t.Run("ok", func(t *testing.T) {
		store := &fakeStore{rows: sashaRows()}
		c, err := NewService(store).Checklist(ctx, 3)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(store.calls, []string{"ensure", "list"}) || store.got.userID != 3 {
			t.Fatalf("calls: %v, user %d", store.calls, store.got.userID)
		}
		if c.Progress.Total != 11 || len(c.Stages) != 3 {
			t.Fatalf("checklist: %+v", c)
		}
	})

	t.Run("ensure error stops before list", func(t *testing.T) {
		store := &fakeStore{ensureErr: errDB}
		if _, err := NewService(store).Checklist(ctx, 3); !errors.Is(err, errDB) {
			t.Fatalf("want errDB, got %v", err)
		}
		if !slices.Equal(store.calls, []string{"ensure"}) {
			t.Fatalf("calls: %v", store.calls)
		}
	})

	t.Run("list error", func(t *testing.T) {
		store := &fakeStore{listErr: errDB}
		if _, err := NewService(store).Checklist(ctx, 3); !errors.Is(err, errDB) {
			t.Fatalf("want errDB, got %v", err)
		}
	})
}

func TestSetStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("ok returns task and new progress", func(t *testing.T) {
		rows := sashaRows()
		rows[4].Task.Status = StatusDone // так база выглядит после обновления
		gitlab := Task{ID: 16, Title: "Доступ к GitLab", Status: StatusDone}
		store := &fakeStore{rows: rows, task: gitlab}

		res, err := NewService(store).SetStatus(ctx, 3, 16, StatusDone)
		if err != nil {
			t.Fatal(err)
		}
		if res.Task != gitlab || res.Progress != (Progress{Done: 5, Total: 11, Percent: 45}) {
			t.Fatalf("result: %+v", res)
		}
		if !slices.Equal(store.calls, []string{"update", "list"}) ||
			store.got.taskID != 16 || store.got.status != StatusDone || store.got.userID != 3 {
			t.Fatalf("calls: %v, got %+v", store.calls, store.got)
		}
	})

	t.Run("invalid status does not touch store", func(t *testing.T) {
		store := &fakeStore{}
		if _, err := NewService(store).SetStatus(ctx, 3, 16, "closed"); !errors.Is(err, ErrInvalidStatus) {
			t.Fatalf("want ErrInvalidStatus, got %v", err)
		}
		if len(store.calls) != 0 {
			t.Fatalf("store must not be called: %v", store.calls)
		}
	})

	t.Run("not found passes through", func(t *testing.T) {
		store := &fakeStore{updateErr: ErrNotFound}
		if _, err := NewService(store).SetStatus(ctx, 3, 99, StatusDone); !errors.Is(err, ErrNotFound) {
			t.Fatalf("want ErrNotFound, got %v", err)
		}
		if !slices.Equal(store.calls, []string{"update"}) {
			t.Fatalf("progress must not be recalculated on error: %v", store.calls)
		}
	})

	t.Run("recalc error", func(t *testing.T) {
		store := &fakeStore{listErr: errDB}
		if _, err := NewService(store).SetStatus(ctx, 3, 16, StatusDone); !errors.Is(err, errDB) {
			t.Fatalf("want errDB, got %v", err)
		}
	})
}
