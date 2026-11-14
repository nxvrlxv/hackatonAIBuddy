package onboarding

import (
	"errors"
	"time"
)

var (
	ErrNotFound      = errors.New("task not found")
	ErrInvalidStatus = errors.New("invalid status")
)

type Status string

const (
	StatusTodo       Status = "todo"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
)

func (s Status) Valid() error {
	if s != StatusTodo && s != StatusInProgress && s != StatusDone {
		return ErrInvalidStatus
	}

	return nil
}

type Task struct {
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      Status     `json:"status"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type Row struct {
	StageCode  string
	StageTitle string
	Task       Task
}

type Stage struct {
	Code  string `json:"code"`
	Title string `json:"title"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
	Tasks []Task `json:"tasks"`
}

type Progress struct {
	Done    int `json:"done"`
	Total   int `json:"total"`
	Percent int `json:"percent"`
}

type Checklist struct {
	Progress Progress `json:"progress"`
	Next     *Task    `json:"next"` // nil — всё пройдено (в JSON будет null)
	Stages   []Stage  `json:"stages"`
}
