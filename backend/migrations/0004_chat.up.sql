-- 0004_chat.up.sql: чаты, ходы агента и сообщения
BEGIN;

CREATE TABLE buddy.conversations (
    id            BIGSERIAL PRIMARY KEY,
    user_id       BIGINT NOT NULL REFERENCES buddy.users(id) ON DELETE CASCADE,
    title         TEXT   NOT NULL DEFAULT '',
    last_sequence BIGINT NOT NULL DEFAULT 0, -- последний выданный sequence_number в чате
    -- резюме истории (пока SUMMARY_ENABLED=false, колонки заложены заранее)
    summary_text             TEXT   NOT NULL DEFAULT '',
    summary_through_sequence BIGINT NOT NULL DEFAULT 0,
    summary_version          INT    NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_conversations_user ON buddy.conversations (user_id, updated_at DESC);

CREATE TABLE buddy.runs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id   BIGINT NOT NULL REFERENCES buddy.conversations(id) ON DELETE CASCADE,
    client_message_id TEXT   NOT NULL,
    status            TEXT   NOT NULL DEFAULT 'running'
        CHECK (status IN ('running','completed','failed','interrupted')),
    started_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_activity_at  TIMESTAMPTZ NOT NULL DEFAULT now(), -- продлевается каждым append, протухание через 5 минут
    finished_at       TIMESTAMPTZ,
    UNIQUE (conversation_id, client_message_id)
);
-- в одном чате не больше одного активного хода
CREATE UNIQUE INDEX runs_one_running ON buddy.runs (conversation_id) WHERE status = 'running';

CREATE TABLE buddy.messages (
    id              BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES buddy.conversations(id) ON DELETE CASCADE,
    run_id          UUID   NOT NULL REFERENCES buddy.runs(id) ON DELETE CASCADE,
    sequence_number BIGINT NOT NULL,
    event_key       TEXT   NOT NULL, -- ключ идемпотентности append внутри хода
    role            TEXT   NOT NULL CHECK (role IN ('user','assistant','tool','system')),
    payload         JSONB  NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (conversation_id, sequence_number),
    UNIQUE (run_id, event_key)
);

COMMIT;
