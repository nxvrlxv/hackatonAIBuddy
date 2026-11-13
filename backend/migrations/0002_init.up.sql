-- 0002_init.up.sql: схема базы знаний для RAG
BEGIN;

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE buddy.documents (
    id               BIGSERIAL PRIMARY KEY,
    team_id          BIGINT      REFERENCES buddy.teams (id), -- NULL = общий документ
    title            TEXT        NOT NULL,
    source_path      TEXT        NOT NULL UNIQUE,
    content_hash     TEXT        NOT NULL, -- sha256, по нему ingest решает, переиндексировать ли
    indexing_version TEXT        NOT NULL, -- без DEFAULT: ingest обязан передать явно
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT documents_source_path_not_empty      CHECK (btrim(source_path) <> ''),
    CONSTRAINT documents_indexing_version_not_empty CHECK (btrim(indexing_version) <> '')
);

CREATE TABLE buddy.chunks (
    id            BIGSERIAL PRIMARY KEY,
    document_id   BIGINT       NOT NULL REFERENCES buddy.documents (id) ON DELETE CASCADE,
    team_id       BIGINT       REFERENCES buddy.teams (id), -- копия documents.team_id для фильтра без JOIN
    chunk_index   INT          NOT NULL,
    section_title TEXT         NOT NULL DEFAULT '',
    content       TEXT         NOT NULL,
    embedding     vector(1024) NOT NULL, -- = размерность модели эмбеддингов
    UNIQUE (document_id, chunk_index),
    CONSTRAINT chunks_index_nonnegative CHECK (chunk_index >= 0),
    CONSTRAINT chunks_content_not_empty CHECK (btrim(content) <> '')
);

CREATE INDEX idx_chunks_embedding ON buddy.chunks USING hnsw (embedding vector_cosine_ops);
CREATE INDEX idx_chunks_team_id   ON buddy.chunks (team_id);

COMMIT;
