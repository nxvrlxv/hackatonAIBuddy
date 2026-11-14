package absences

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// MaxDays — предельная длина одного отгула (дней включительно).
const MaxDays = 30

// DB — подмножество пула; реализуют *pgxpool.Pool и pgxmock.
type DB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Service struct {
	db  DB
	now func() time.Time
}

func New(db DB) *Service { return &Service{db: db, now: time.Now} }

var msk = time.FixedZone("MSK", 3*3600)

func validate(in Input, now time.Time) error {
	start, e1 := time.Parse(time.DateOnly, in.StartDate)
	end, e2 := time.Parse(time.DateOnly, in.EndDate)
	today := now.In(msk).Format(time.DateOnly)
	if e1 != nil || e2 != nil || end.Before(start) || in.StartDate < today {
		return fmt.Errorf("%w: даты YYYY-MM-DD, начало не раньше сегодня (Москва), конец не раньше начала", ErrInvalid)
	}
	if days := int(end.Sub(start).Hours()/24) + 1; days > MaxDays {
		return fmt.Errorf("%w: отгул не длиннее %d дней", ErrInvalid, MaxDays)
	}
	if utf8.RuneCountInString(in.Reason) > 2000 || len(strings.TrimSpace(in.IdempotencyKey)) == 0 || len(in.IdempotencyKey) > 200 {
		return fmt.Errorf("%w: причина до 2000 символов; требуется ключ повторной отправки до 200 символов", ErrInvalid)
	}
	return nil
}

func (s *Service) Types(ctx context.Context) ([]Type, error) {
	rows, err := s.db.Query(ctx, "SELECT code,title,description,fields FROM buddy.request_types WHERE enabled ORDER BY code")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Type{}
	for rows.Next() {
		var v Type
		if err = rows.Scan(&v.Code, &v.Title, &v.Description, &v.Fields); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func scan(row pgx.Row) (Request, error) {
	var v Request
	err := row.Scan(&v.ID, &v.TypeCode, &v.Data, &v.Status, &v.CreatedAt)
	return v, err
}

const columns = "id,type_code,data,status,created_at"

func (s *Service) List(ctx context.Context, uid int64) ([]Request, error) {
	rows, err := s.db.Query(ctx, "SELECT "+columns+" FROM buddy.requests WHERE user_id=$1 ORDER BY id DESC LIMIT 100", uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Request{}
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Service) Create(ctx context.Context, uid int64, in Input) (Request, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	data, _ := json.Marshal(map[string]string{"start_date": in.StartDate, "end_date": in.EndDate, "reason": in.Reason})
	// Повтор возвращает исходную заявку, даже если её даты уже прошли.
	existing, err := scan(s.db.QueryRow(ctx, "SELECT "+columns+" FROM buddy.requests WHERE user_id=$1 AND idempotency_key=$2", uid, in.IdempotencyKey))
	if err == nil {
		var same bool
		err = s.db.QueryRow(ctx, "SELECT data=$3::jsonb FROM buddy.requests WHERE user_id=$1 AND idempotency_key=$2", uid, in.IdempotencyKey, data).Scan(&same)
		if err != nil {
			return Request{}, err
		}
		if !same {
			return Request{}, fmt.Errorf("%w: ключ уже использован с другими данными", ErrConflict)
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Request{}, err
	}
	if err = validate(in, s.now()); err != nil {
		return Request{}, err
	}
	v, err := scan(s.db.QueryRow(ctx, "INSERT INTO buddy.requests(user_id,type_code,data,idempotency_key) SELECT $1,code,$2::jsonb,$3 FROM buddy.request_types WHERE code='time_off' AND enabled ON CONFLICT(user_id,idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key WHERE buddy.requests.data=EXCLUDED.data RETURNING "+columns, uid, data, in.IdempotencyKey))
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, fmt.Errorf("%w: тип отключён или ключ использован с другими данными", ErrConflict)
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		// Параллельный повтор может упереться в индекс дат раньше, чем в индекс ключа.
		var replay bool
		if e := s.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM buddy.requests WHERE user_id=$1 AND idempotency_key=$2)", uid, in.IdempotencyKey).Scan(&replay); e != nil {
			return Request{}, e
		}
		if replay {
			return s.Create(ctx, uid, in)
		}
		return Request{}, fmt.Errorf("%w: заявка на эти даты уже существует; посмотри список заявок", ErrConflict)
	}
	return v, err
}
