-- 000001_users_onboarding.down.sql
BEGIN;
DROP TABLE IF EXISTS buddy.intern_tasks;
DROP TABLE IF EXISTS buddy.task_templates;
DROP TABLE IF EXISTS buddy.onboarding_stages;
DROP TABLE IF EXISTS buddy.users;
DROP TABLE IF EXISTS buddy.teams;
DROP SCHEMA IF EXISTS buddy; -- без CASCADE: упадёт, если в схеме что-то осталось
DROP SCHEMA IF EXISTS corporate; -- no CASCADE: preserve unrelated objects
-- The vector extension is retained as shared infrastructure.
COMMIT;