package buddy

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
)

func (r *Repository) GetUserByLogin(ctx context.Context, login string) (*auth.User, error) {
	row := r.pool.QueryRow(ctx, `
	select u.id, u.login, u.full_name, u.role, t.id, t.name, m.full_name
	from buddy.users u
	join buddy.teams t on u.team_id = t.id
	left join buddy.users m on m.id = u.mentor_id
	where u.login = $1
	`, login)

	var user auth.User

	err := row.Scan(&user.ID, &user.Login, &user.FullName, &user.Role, &user.TeamID, &user.Team, &user.Mentor)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user not found: %w", auth.ErrNotFound)
		}
		return nil, fmt.Errorf("get user by login: %w", err)
	}

	return &user, nil
}
