-- 000002_knowledge_base.up.sql
BEGIN;

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE buddy.documents (
    id           BIGSERIAL PRIMARY KEY,
    team_id      BIGINT      REFERENCES buddy.teams (id), -- NULL = общий документ
    title        TEXT        NOT NULL,
    source_path  TEXT        NOT NULL UNIQUE,
    content_hash TEXT        NOT NULL, -- sha256: защита от дублей при повторном ingest
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE buddy.chunks (
    id          BIGSERIAL PRIMARY KEY,
    document_id BIGINT      NOT NULL REFERENCES buddy.documents (id) ON DELETE CASCADE,
    team_id     BIGINT      REFERENCES buddy.teams (id), -- копия documents.team_id для фильтра без JOIN
    chunk_index INT         NOT NULL,
    content     TEXT        NOT NULL,
    embedding   vector(1024) NOT NULL, -- размерность = размерность модели эмбеддингов
    UNIQUE (document_id, chunk_index)
);

CREATE INDEX idx_chunks_embedding ON buddy.chunks USING hnsw (embedding vector_cosine_ops);
CREATE INDEX idx_chunks_team_id   ON buddy.chunks (team_id);

COMMIT;