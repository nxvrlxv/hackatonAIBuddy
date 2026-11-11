
BEGIN;

CREATE EXTENSION IF NOT EXISTS vector;

CREATE SCHEMA IF NOT EXISTS buddy;
CREATE SCHEMA IF NOT EXISTS corporate;

CREATE TABLE buddy.teams (
    id      bigserial primary key,
    code    text not null, -- код команды для сравнения с папками из /data
    name    text not null --имя команды в человекочитаеемом виде
);

CREATE TABLE buddy.users (
    id            BIGSERIAL PRIMARY KEY,
    login         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    full_name     TEXT        NOT NULL,
    role          TEXT        NOT NULL CHECK (role IN ('intern', 'mentor', 'lead')),
    team_id       BIGINT      NOT NULL REFERENCES buddy.teams (id),
    mentor_id     BIGINT      REFERENCES buddy.users (id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (mentor_id IS NULL OR mentor_id <> id)
);

CREATE INDEX idx_users_team_id   ON buddy.users (team_id);
CREATE INDEX idx_users_mentor_id ON buddy.users (mentor_id);

-- чек-лист адаптации: стадии онбординга, шаблоны задач и задачи конкретных стажеров
CREATE TABLE buddy.onboarding_stages (
    id       BIGSERIAL PRIMARY KEY,
    code     TEXT NOT NULL UNIQUE, -- team, tools, first_tasks
    title    TEXT NOT NULL,
    position INT  NOT NULL UNIQUE
);

CREATE TABLE buddy.task_templates (
    id          BIGSERIAL PRIMARY KEY,
    stage_id    BIGINT NOT NULL REFERENCES buddy.onboarding_stages (id),
    team_id     BIGINT REFERENCES buddy.teams (id), -- NULL = задача для всех команд
    title       TEXT   NOT NULL,
    description TEXT   NOT NULL DEFAULT '',
    position    INT    NOT NULL
);

CREATE INDEX idx_task_templates_stage ON buddy.task_templates (stage_id, position);

CREATE TABLE buddy.intern_tasks (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT      NOT NULL REFERENCES buddy.users (id) ON DELETE CASCADE,
    template_id  BIGINT      NOT NULL REFERENCES buddy.task_templates (id),
    status       TEXT        NOT NULL DEFAULT 'todo'
                             CHECK (status IN ('todo', 'in_progress', 'done')),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    UNIQUE (user_id, template_id),
    CHECK ((status = 'done') = (completed_at IS NOT NULL))
);

COMMIT;