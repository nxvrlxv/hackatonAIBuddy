-- 0004_chat.down.sql: в обратном порядке зависимостей
BEGIN;
DROP TABLE IF EXISTS buddy.messages;
DROP TABLE IF EXISTS buddy.runs;
DROP TABLE IF EXISTS buddy.conversations;
COMMIT;
