package onboarding

import (
	"context"
	"fmt"
)

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

// UpdateResult — ответ на смену статуса: задача и новый общий прогресс.
type UpdateResult struct {
	Task     Task     `json:"task"`
	Progress Progress `json:"progress"`
}

// Checklist возвращает чек-лист пользователя; стажёру при первом обращении создаются задачи.
func (s *Service) Checklist(ctx context.Context, userID int64) (Checklist, error) {
	if err := s.store.EnsureTasks(ctx, userID); err != nil {
		return Checklist{}, fmt.Errorf("checklist: %w", err)
	}
	rows, err := s.store.ListTasks(ctx, userID)
	if err != nil {
		return Checklist{}, fmt.Errorf("checklist: %w", err)
	}
	return build(rows), nil
}

// SetStatus меняет статус своей задачи и возвращает её вместе с пересчитанным прогрессом.
func (s *Service) SetStatus(ctx context.Context, userID, taskID int64, status Status) (UpdateResult, error) {
	if err := status.Valid(); err != nil {
		return UpdateResult{}, err
	}
	task, err := s.store.UpdateStatus(ctx, userID, taskID, status)
	if err != nil {
		return UpdateResult{}, fmt.Errorf("set status: %w", err)
	}
	rows, err := s.store.ListTasks(ctx, userID)
	if err != nil {
		return UpdateResult{}, fmt.Errorf("set status: %w", err)
	}
	return UpdateResult{Task: task, Progress: build(rows).Progress}, nil
}

// build собирает чек-лист из строк, отсортированных по этапам и позициям.
func build(rows []Row) Checklist {
	c := Checklist{Stages: []Stage{}}
	for _, r := range rows {
		if n := len(c.Stages); n == 0 || c.Stages[n-1].Code != r.StageCode {
			c.Stages = append(c.Stages, Stage{Code: r.StageCode, Title: r.StageTitle, Tasks: []Task{}})
		}
		st := &c.Stages[len(c.Stages)-1]
		st.Tasks = append(st.Tasks, r.Task)
		st.Total++
		c.Progress.Total++

		if r.Task.Status == StatusDone {
			st.Done++
			c.Progress.Done++
		} else if c.Next == nil {
			next := r.Task // копия: указатель на переменную цикла не берём
			c.Next = &next
		}
	}
	if c.Progress.Total > 0 {
		c.Progress.Percent = c.Progress.Done * 100 / c.Progress.Total
	}
	return c
}
