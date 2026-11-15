package onboarding

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
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

// SetStatus — действие стажёра над своей задачей. Завершить её сам стажёр не может:
// done превращается в review (уходит наставнику на проверку). В ответе — фактический статус.
func (s *Service) SetStatus(ctx context.Context, userID, taskID int64, status Status) (UpdateResult, error) {
	if err := status.Valid(); err != nil {
		return UpdateResult{}, err
	}

	if status == StatusDone {
		status = StatusReview // done ставит только наставник через Approve
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

// MaxCommentLen — предел комментария при отклонении (в символах, после trim).
const MaxCommentLen = 500

// Approve — наставник принимает задачу своего стажёра (review → done).
func (s *Service) Approve(ctx context.Context, mentorID, taskID int64) error {
	err := s.store.Approve(ctx, mentorID, taskID)
	if err != nil {
		return fmt.Errorf("approve: %w", err)
	}

	return nil
}

// Reject — наставник возвращает задачу на доработку (review → in_progress) с обязательным комментарием.
func (s *Service) Reject(ctx context.Context, mentorID, taskID int64, comment string) error {
	comment = strings.TrimSpace(comment)
	if comment == "" {
		return ErrCommentRequired
	}
	if utf8.RuneCountInString(comment) > MaxCommentLen {
		return ErrCommentTooLong
	}
	err := s.store.Reject(ctx, mentorID, taskID, comment)
	if err != nil {
		return fmt.Errorf("reject: %w", err)
	}

	return nil
}

// Reviews — задачи на проверке у стажёров наставника, старые сверху.
func (s *Service) Reviews(ctx context.Context, mentorID int64) ([]Review, error) {
	result, err := s.store.ListReviews(ctx, mentorID)
	if err != nil {
		return nil, fmt.Errorf("reviews: %w", err)
	}

	return result, nil
}

// Interns — прогресс адаптации стажёров наставника.
func (s *Service) Interns(ctx context.Context, mentorID int64) ([]InternProgress, error) {
	progresses, err := s.store.ListInterns(ctx, mentorID)
	if err != nil {
		return nil, fmt.Errorf("interns onboardings: %w", err)
	}

	// по индексу: range отдаёт копию элемента, запись в неё не попала бы в срез
	for i := range progresses {
		if p := &progresses[i].Progress; p.Total > 0 {
			p.Percent = p.Done * 100 / p.Total
		}
	}

	return progresses, nil
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
		} else if c.Next == nil && r.Task.Status != StatusReview {
			next := r.Task // копия: указатель на переменную цикла не берём
			c.Next = &next
		}
	}
	if c.Progress.Total > 0 {
		c.Progress.Percent = c.Progress.Done * 100 / c.Progress.Total
	}
	return c
}
