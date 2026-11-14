package absences

import (
	"errors"
	"time"
)

var ErrInvalid = errors.New("invalid request")
var ErrConflict = errors.New("request conflict")

type Type struct {
	Code        string         `json:"code"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Fields      map[string]any `json:"fields"`
}
type Input struct {
	StartDate      string `json:"start_date"`
	EndDate        string `json:"end_date,omitempty"` // пусто — отгул на один день (start_date)
	Reason         string `json:"reason,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}
type Request struct {
	ID        int64          `json:"id"`
	TypeCode  string         `json:"type_code"`
	Data      map[string]any `json:"data"`
	Status    string         `json:"status"`
	CreatedAt time.Time      `json:"created_at"`
}
