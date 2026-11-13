BEGIN;
CREATE TABLE buddy.request_types (
 code TEXT PRIMARY KEY,
 title TEXT NOT NULL,
 description TEXT NOT NULL,
 fields JSONB NOT NULL CHECK (jsonb_typeof(fields) = 'object'),
 enabled BOOLEAN NOT NULL DEFAULT true
);
INSERT INTO buddy.request_types VALUES ('time_off', 'Отгул', 'Заявка на отсутствие на целые дни. Создание не означает согласование.',
 '{"start_date":{"type":"date","required":true},"end_date":{"type":"date","required":true},"reason":{"type":"string","required":false,"maxLength":2000}}', true);
CREATE TABLE buddy.requests (
 id BIGSERIAL PRIMARY KEY,
 user_id BIGINT NOT NULL REFERENCES buddy.users(id),
 type_code TEXT NOT NULL REFERENCES buddy.request_types(code),
 data JSONB NOT NULL CHECK(jsonb_typeof(data)='object'),
 status TEXT NOT NULL DEFAULT 'submitted' CHECK(status IN ('submitted','approved','rejected','cancelled')),
 idempotency_key TEXT NOT NULL CHECK(length(btrim(idempotency_key)) BETWEEN 1 AND 200),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(user_id,idempotency_key)
);
CREATE INDEX requests_user_created ON buddy.requests(user_id,created_at DESC);
-- Exact dates cannot be submitted twice while the request is active, even in another chat.
CREATE UNIQUE INDEX requests_active_dates ON buddy.requests
 (user_id,type_code,(data->>'start_date'),(data->>'end_date'))
 WHERE status IN ('submitted','approved');
COMMIT;
