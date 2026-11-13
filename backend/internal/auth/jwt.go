package auth

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("Invalid Token")
)

type Claims struct {
	UserID int64  `json:"user_id"`
	TeamID int64  `json:"team_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

type Manager struct {
	secret []byte
	ttl    time.Duration
}

func NewManager(secret string, ttl time.Duration) (*Manager, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("secret length must be at least 32 characters long")
	}

	if ttl <= 0 {
		return nil, fmt.Errorf("incorrect ttl")
	}

	return &Manager{
		secret: []byte(secret),
		ttl:    ttl,
	}, nil
}

// возвращает токен пользователя
func (m *Manager) Issue(user *User) (string, error) {
	if user == nil {
		return "", fmt.Errorf("user is nil")
	}

	now := time.Now()

	claims := &Claims{
		UserID: user.ID,
		TeamID: user.TeamID,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(user.ID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return token, nil

}

func (m *Manager) Parse(token string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(token, claims,
		func(*jwt.Token) (any, error) { return m.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), // отсекает alg=none и подмену алгоритма
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	return claims, nil
}
