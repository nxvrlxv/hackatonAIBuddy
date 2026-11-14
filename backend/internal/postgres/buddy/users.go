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

func (r *Repository) GetUserByID(ctx context.Context, userID int64) (*auth.User, error) {
	row := r.pool.QueryRow(ctx, `
	select u.id, u.login, u.full_name, u.role, t.id, t.name, m.full_name
	from buddy.users u
	join buddy.teams t on u.team_id = t.id
	left join buddy.users m on m.id = u.mentor_id
	where u.id = $1
	`, userID)

	var user auth.User

	err := row.Scan(&user.ID, &user.Login, &user.FullName, &user.Role, &user.TeamID, &user.Team, &user.Mentor)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user not found: %w", auth.ErrNotFound)
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}

	return &user, nil
}

// TeamMember — коллега по команде. Логин и хеш пароля наружу не отдаём.
type TeamMember struct {
	ID         int64
	FullName   string
	Role       string
	IsMyMentor bool
}

// ListTeamMembers — все в команде, кроме самого пользователя: лиды, наставники, стажёры.
// teamID и userID берутся из токена, поэтому чужую команду так не посмотреть.
func (r *Repository) ListTeamMembers(ctx context.Context, teamID, userID int64) ([]TeamMember, error) {
	rows, err := r.pool.Query(ctx, `
	select u.id, u.full_name, u.role, coalesce(u.id = me.mentor_id, false)
	from buddy.users u
	left join buddy.users me on me.id = $2
	where u.team_id = $1 and u.id <> $2
	order by case u.role when 'lead' then 0 when 'mentor' then 1 else 2 end, u.full_name
	`, teamID, userID)
	if err != nil {
		return nil, fmt.Errorf("list team members: %w", err)
	}
	defer rows.Close()

	members := []TeamMember{}
	for rows.Next() {
		var m TeamMember
		if err := rows.Scan(&m.ID, &m.FullName, &m.Role, &m.IsMyMentor); err != nil {
			return nil, fmt.Errorf("scan team member: %w", err)
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list team members rows: %w", err)
	}
	return members, nil
}
