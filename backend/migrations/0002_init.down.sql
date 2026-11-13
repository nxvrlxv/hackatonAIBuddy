-- Drop dependent chunks before their documents.
BEGIN;
DROP TABLE IF EXISTS buddy.chunks;
DROP TABLE IF EXISTS buddy.documents;
-- The vector extension is shared infrastructure and is retained.
COMMIT;
