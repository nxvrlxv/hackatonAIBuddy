BEGIN;

CREATE SCHEMA agent;

-- Пользователь может иметь несколько независимых чатов
CREATE TABLE agent.conversations (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES buddy.users (id) ON DELETE CASCADE,
    title      TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_agent_conversations_user_updated
    ON agent.conversations (user_id, updated_at DESC, id DESC);

-- Полная история
-- sequence_number назначает приложение последовательно внутри чата
CREATE TABLE agent.messages (
    id                BIGSERIAL PRIMARY KEY,
    conversation_id   BIGINT NOT NULL REFERENCES agent.conversations (id) ON DELETE CASCADE,
    sequence_number   BIGINT NOT NULL CHECK (sequence_number > 0),
    role              TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'tool')),
    -- Содержимое и поля конкретных вызовов инструментов; токены не сохранять.
    payload           JSONB NOT NULL DEFAULT '{"content": ""}'::jsonb,
    -- Ключ повторной отправки пользовательского сообщения; уникален внутри чата.
    client_message_id TEXT,
    status            TEXT NOT NULL DEFAULT 'completed'
                      CHECK (status IN ('streaming', 'completed', 'interrupted', 'failed')),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (conversation_id, sequence_number),
    CONSTRAINT messages_payload_object CHECK (jsonb_typeof(payload) = 'object'),
    CONSTRAINT messages_payload_content CHECK (
        payload ? 'content'
        AND jsonb_typeof(payload->'content') IN ('string', 'array')
    ),
    CONSTRAINT messages_payload_tool_calls CHECK (
        NOT (payload ? 'tool_calls')
        OR jsonb_typeof(payload->'tool_calls') = 'array'
    ),
    CHECK (
        client_message_id IS NULL
        OR (role = 'user' AND btrim(client_message_id) <> '')
    )
);

CREATE UNIQUE INDEX idx_agent_messages_client_id
    ON agent.messages (conversation_id, client_message_id)
    WHERE client_message_id IS NOT NULL;

-- Одно актуальное резюме на чат. До первого сжатия записи нет
-- Граница включительна 
CREATE TABLE agent.conversation_summaries (
    conversation_id  BIGINT PRIMARY KEY
                     REFERENCES agent.conversations (id) ON DELETE CASCADE,
    summary_text     TEXT NOT NULL CHECK (btrim(summary_text) <> ''),
    through_sequence BIGINT NOT NULL,
    version          BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (conversation_id, through_sequence)
        REFERENCES agent.messages (conversation_id, sequence_number) ON DELETE CASCADE
);

-- Поисковый индекс истории, отдельно от резюме.
CREATE TABLE agent.conversation_chunks (
    id               BIGSERIAL PRIMARY KEY,
    conversation_id  BIGINT NOT NULL
                     REFERENCES agent.conversations (id) ON DELETE CASCADE,
    start_sequence   BIGINT NOT NULL,
    end_sequence     BIGINT NOT NULL,
    content          TEXT NOT NULL CHECK (btrim(content) <> ''),
    embedding        vector(1024) NOT NULL,
    indexing_version TEXT NOT NULL CHECK (btrim(indexing_version) <> ''),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (start_sequence > 0 AND end_sequence >= start_sequence),
    UNIQUE (conversation_id, start_sequence, end_sequence),
    FOREIGN KEY (conversation_id, start_sequence)
        REFERENCES agent.messages (conversation_id, sequence_number) ON DELETE CASCADE,
    FOREIGN KEY (conversation_id, end_sequence)
        REFERENCES agent.messages (conversation_id, sequence_number) ON DELETE CASCADE
);

CREATE INDEX idx_agent_chunks_embedding
    ON agent.conversation_chunks USING hnsw (embedding vector_cosine_ops);
    
COMMIT;
