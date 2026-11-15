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
	select s.code, s.title, it.id, t.title, t.description, it.status, it.completed_at, it.submitted_at, it.review_comment
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
			&r.Task.Description, &status, &r.Task.CompletedAt, &r.Task.SubmittedAt, &r.Task.ReviewComment); err != nil {
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

// UpdateStatus меняет статус своей задачи (действие стажёра).
// Чужая или несуществующая → ErrNotFound; уже принятая наставником → ErrAlreadyApproved.
func (p *Postgres) UpdateStatus(ctx context.Context, userID, taskID int64, status Status) (Task, error) {
	// 1. Есть ли такая задача у пользователя и в каком она статусе.
	var current string
	err := p.db.QueryRow(ctx,
		`select status from buddy.intern_tasks where id = $1 and user_id = $2`,
		taskID, userID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, fmt.Errorf("update status: %w", err)
	}
	if current == string(StatusDone) {
		return Task{}, ErrAlreadyApproved
	}

	// 2. Обновляем. "status <> 'done'" — на случай, если наставник принял задачу
	//    между нашими двумя запросами.
	row := p.db.QueryRow(ctx, `
	with upd as (
		update buddy.intern_tasks
		set status = $3::text,
		    updated_at = now(),
		    completed_at = null,
		    -- review: дата отправки (повторная отправка дату не сдвигает); иначе — сброс
		    submitted_at = case
		        when $3::text = 'review' then coalesce(case when status = 'review' then submitted_at end, now())
		        else null end,
		    -- новая отправка стирает старый комментарий; возврат в работу его сохраняет
		    review_comment = case when $3::text = 'review' then null else review_comment end
		where id = $2 and user_id = $1 and status <> 'done'
		returning id, template_id, status, completed_at, submitted_at, review_comment
	)
	select upd.id, t.title, t.description, upd.status, upd.completed_at, upd.submitted_at, upd.review_comment
	from upd join buddy.task_templates t on t.id = upd.template_id
	`, userID, taskID, string(status))

	var task Task
	var st string
	if err := row.Scan(&task.ID, &task.Title, &task.Description, &st,
		&task.CompletedAt, &task.SubmittedAt, &task.ReviewComment); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Task{}, ErrAlreadyApproved // успели принять между запросами
		}
		return Task{}, fmt.Errorf("update status: %w", err)
	}
	task.Status = Status(st)
	return task, nil
}

func (p *Postgres) ListReviews(ctx context.Context, mentorID int64) ([]Review, error) {
	rows, err := p.db.Query(ctx, `
	select it.id, t.title, t.description, os.title, u.id, u.full_name, it.submitted_at
	from buddy.intern_tasks it
	join buddy.task_templates t on it.template_id = t.id
	join buddy.users u on it.user_id = u.id
	join buddy.onboarding_stages os on t.stage_id = os.id
	where u.mentor_id = $1 and it.status = 'review'
	order by it.submitted_at, u.id 
	`, mentorID)

	if err != nil {
		return nil, fmt.Errorf("list reviews err : %w", err)
	}

	defer rows.Close()
	result := []Review{}
	for rows.Next() {
		var review Review

		err := rows.Scan(&review.TaskID, &review.TaskTitle, &review.TaskDescription, &review.StageTitle, &review.Intern.ID, &review.Intern.FullName, &review.SubmittedAt)
		if err != nil {
			return nil, fmt.Errorf("review scan err : %w", err)
		}

		result = append(result, review)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tasks reviews: %w", err)
	}

	return result, nil
}

func (p *Postgres) Approve(ctx context.Context, mentorID, taskID int64) error {
	query := `
	update buddy.intern_tasks it
	set status = 'done', completed_at = now(), reviewed_by = $1, reviewed_at = now(), review_comment=null, submitted_at=null
	where it.id = $2 and it.status = 'review' and it.user_id in (select u.id from buddy.users u where u.mentor_id = $1)
	`
	tag, err := p.db.Exec(ctx, query, mentorID, taskID)
	if err != nil {
		return fmt.Errorf("update task err: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return p.reviewMiss(ctx, mentorID, taskID)
	}

	return nil
}

func (p *Postgres) reviewMiss(ctx context.Context, mentorID, taskID int64) error {
	var status string
	err := p.db.QueryRow(ctx, `
	select it.status from buddy.intern_tasks it
	join buddy.users u on u.id = it.user_id
	where it.id = $1 and u.mentor_id = $2
	`, taskID, mentorID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("check review task: %w", err)
	}
	return ErrNotInReview // задача твоего стажёра, но уже принята/отклонена/не отправлялась
}

func (p *Postgres) Reject(ctx context.Context, mentorID, taskID int64, rejectComment string) error {
	query := `
	update buddy.intern_tasks it
	set status = 'in_progress', reviewed_by = $1, reviewed_at = now(), review_comment=$2, submitted_at=null
	where it.id = $3 and it.status = 'review' and it.user_id in (select u.id from buddy.users u where u.mentor_id = $1)
	`
	tag, err := p.db.Exec(ctx, query, mentorID, rejectComment, taskID)
	if err != nil {
		return fmt.Errorf("update task err: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return p.reviewMiss(ctx, mentorID, taskID)
	}

	return nil
}

func (p *Postgres) ListInterns(ctx context.Context, mentorID int64) ([]InternProgress, error) {
	rows, err := p.db.Query(ctx, `
		select u.id, u.full_name,
			count(*)                                        as total,
			count(*) filter (where it.status = 'done')      as done,
			count(*) filter (where it.status = 'review')    as in_review,
			coalesce((array_agg(s.title order by s.position, t.position)
						filter (where coalesce(it.status, 'todo') <> 'done'))[1], '') as stage
		from buddy.users u
		join buddy.task_templates t    on t.team_id is null or t.team_id = u.team_id
		join buddy.onboarding_stages s on s.id = t.stage_id
		left join buddy.intern_tasks it on it.user_id = u.id and it.template_id = t.id
		where u.mentor_id = $1
		group by u.id, u.full_name
		order by u.full_name

	`, mentorID)

	if err != nil {
		return nil, fmt.Errorf("list interns err: %w", err)
	}

	defer rows.Close()
	result := []InternProgress{}
	for rows.Next() {
		var intern InternProgress

		err := rows.Scan(&intern.Intern.ID, &intern.Intern.FullName, &intern.Progress.Total, &intern.Progress.Done, &intern.InReview, &intern.Stage)

		if err != nil {
			return nil, fmt.Errorf("intern scan err: %w", err)
		}

		result = append(result, intern)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list interns onboardings: %w", err)
	}

	return result, nil
}
