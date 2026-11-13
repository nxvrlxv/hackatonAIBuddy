package auth

import (
	"errors"
)

var (
	ErrNotFound = errors.New("User Not Found")
)

type User struct {
	ID       int64
	Login    string
	FullName string
	Role     string
	TeamID   int64
	Team     string
	Mentor   *string
}
