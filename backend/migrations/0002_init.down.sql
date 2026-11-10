-- 000002_knowledge_base.down.sql
BEGIN;
DROP TABLE IF EXISTS buddy.chunks;
DROP TABLE IF EXISTS buddy.documents;
DROP EXTENSION IF EXISTS vector;
COMMIT;