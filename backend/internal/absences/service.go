package absences

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"time"
	"unicode/utf8"
)

type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool, now: time.Now} }
func validate(in Input, now time.Time) error {
	start, e1 := time.Parse("2006-01-02", in.StartDate)
	end, e2 := time.Parse("2006-01-02", in.EndDate)
	today := now.In(time.FixedZone("MSK", 3*3600)).Format("2006-01-02")
	if e1 != nil || e2 != nil || end.Before(start) || in.StartDate < today {
		return fmt.Errorf("%w: даты YYYY-MM-DD, начало не раньше сегодня (Москва), конец не раньше начала", ErrInvalid)
	}
	if utf8.RuneCountInString(in.Reason) > 2000 || len(strings.TrimSpace(in.IdempotencyKey)) == 0 || len(in.IdempotencyKey) > 200 {
		return fmt.Errorf("%w: причина до 2000 символов; требуется ключ повторной отправки до 200 символов", ErrInvalid)
	}
	return nil
}
func (s *Service) Types(ctx context.Context) ([]Type, error) {
	rows, err := s.pool.Query(ctx, "SELECT code,title,description,fields FROM buddy.request_types WHERE enabled ORDER BY code")
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
	rows, err := s.pool.Query(ctx, "SELECT "+columns+" FROM buddy.requests WHERE user_id=$1 ORDER BY id DESC LIMIT 100", uid)
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
	// Replays return the original even after its dates have passed.
	existing, err := scan(s.pool.QueryRow(ctx, "SELECT "+columns+" FROM buddy.requests WHERE user_id=$1 AND idempotency_key=$2", uid, in.IdempotencyKey))
	if err == nil {
		var same bool
		err = s.pool.QueryRow(ctx, "SELECT data=$3::jsonb FROM buddy.requests WHERE user_id=$1 AND idempotency_key=$2", uid, in.IdempotencyKey, data).Scan(&same)
		if err != nil {
			return Request{}, err
		}
		if !same {
			return Request{}, ErrConflict
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Request{}, err
	}
	if err = validate(in, s.now()); err != nil {
		return Request{}, err
	}
	v, err := scan(s.pool.QueryRow(ctx, "INSERT INTO buddy.requests(user_id,type_code,data,idempotency_key) SELECT $1,code,$2::jsonb,$3 FROM buddy.request_types WHERE code='time_off' AND enabled ON CONFLICT(user_id,idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key WHERE buddy.requests.data=EXCLUDED.data RETURNING "+columns, uid, data, in.IdempotencyKey))
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, fmt.Errorf("%w: тип отключён или ключ использован с другими данными", ErrConflict)
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		// A concurrent replay can hit the date index before the key index.
		var replay bool
		if e := s.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM buddy.requests WHERE user_id=$1 AND idempotency_key=$2)", uid, in.IdempotencyKey).Scan(&replay); e != nil {
			return Request{}, e
		}
		if replay {
			return s.Create(ctx, uid, in)
		}
		return Request{}, fmt.Errorf("%w: заявка на эти даты уже существует; посмотри список заявок", ErrConflict)
	}
	return v, err
}
