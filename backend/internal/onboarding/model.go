package onboarding

import (
	"errors"
	"time"
)

var (
	ErrNotFound        = errors.New("task not found")
	ErrInvalidStatus   = errors.New("invalid status")
	ErrAlreadyApproved = errors.New("task already approved")
	ErrNotInReview     = errors.New("task not in review")
	ErrCommentRequired = errors.New("couldn't reject without leaving a comment")
	ErrCommentTooLong  = errors.New("comment must be at most 500 characters long")
)

type Status string

const (
	StatusTodo       Status = "todo"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
	StatusReview     Status = "review"
)

func (s Status) Valid() error {
	if s != StatusTodo && s != StatusInProgress && s != StatusDone && s != StatusReview {
		return ErrInvalidStatus
	}

	return nil
}

type Task struct {
	ID            int64      `json:"id"`
	Title         string     `json:"title"`
	Description   string     `json:"description"`
	Status        Status     `json:"status"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	SubmittedAt   *time.Time `json:"submitted_at,omitempty"`
	ReviewComment *string    `json:"review_comment,omitempty"`
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

type Intern struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
}

type Review struct {
	TaskID          int64     `json:"task_id"`
	TaskTitle       string    `json:"task_title"`
	TaskDescription string    `json:"task_description"`
	StageTitle      string    `json:"stage_title"`
	Intern          Intern    `json:"intern"` // {ID, FullName}
	SubmittedAt     time.Time `json:"submitted_at"`
}

type InternProgress struct {
	Intern          // встраиваем: id, full_name
	Progress        // done, total, percent — уже есть
	InReview int    `json:"in_review"`
	Stage    string `json:"stage"` // "" = всё принято
}
