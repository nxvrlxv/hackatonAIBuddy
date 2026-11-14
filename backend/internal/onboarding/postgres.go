package onboarding

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DB — подмножество пула; реализуют *pgxpool.Pool и pgxmock.
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Postgres struct {
	db DB
}

func NewPostgres(db DB) *Postgres {
	return &Postgres{db: db}
}

// проверка на удовлетворение структуры postgres интерфейсу Store
var _ Store = (*Postgres)(nil)

// EnsureTasks создаёт стажёру задачи по шаблонам (общим и своей команды), если их ещё нет.
// Для не-стажёров ничего не вставляет: роль проверяется в WHERE.
func (p *Postgres) EnsureTasks(ctx context.Context, userID int64) error {
	_, err := p.db.Exec(ctx, `
	insert into buddy.intern_tasks (user_id, template_id)
	select u.id, t.id
	from buddy.users u
	join buddy.task_templates t on t.team_id is null or t.team_id = u.team_id
	where u.id = $1 and u.role = 'intern'
	on conflict (user_id, template_id) do nothing
	`, userID)
	if err != nil {
		return fmt.Errorf("ensure tasks: %w", err)
	}
	return nil
}

func (p *Postgres) ListTasks(ctx context.Context, userID int64) ([]Row, error) {
	rows, err := p.db.Query(ctx, `
	select s.code, s.title, it.id, t.title, t.description, it.status, it.completed_at
	from buddy.intern_tasks it
	join buddy.task_templates t on t.id = it.template_id
	join buddy.onboarding_stages s on s.id = t.stage_id
	where it.user_id = $1
	order by s.position, t.position
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	result := []Row{}
	for rows.Next() {
		var r Row
		var status string
		if err := rows.Scan(&r.StageCode, &r.StageTitle, &r.Task.ID, &r.Task.Title,
			&r.Task.Description, &status, &r.Task.CompletedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		r.Task.Status = Status(status)
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tasks rows: %w", err)
	}
	return result, nil
}

// UpdateStatus меняет статус только своей задачи: чужая или несуществующая → ErrNotFound.
// Повторный done не сдвигает completed_at (coalesce).
func (p *Postgres) UpdateStatus(ctx context.Context, userID, taskID int64, status Status) (Task, error) {
	row := p.db.QueryRow(ctx, `
	with upd as (
		update buddy.intern_tasks
		set status = $3,
		    updated_at = now(),
		    completed_at = case when $3 = 'done' then coalesce(completed_at, now()) else null end
		where id = $2 and user_id = $1
		returning id, template_id, status, completed_at
	)
	select upd.id, t.title, t.description, upd.status, upd.completed_at
	from upd join buddy.task_templates t on t.id = upd.template_id
	`, userID, taskID, string(status))

	var task Task
	var st string
	if err := row.Scan(&task.ID, &task.Title, &task.Description, &st, &task.CompletedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Task{}, ErrNotFound
		}
		return Task{}, fmt.Errorf("update status: %w", err)
	}
	task.Status = Status(st)
	return task, nil
}
