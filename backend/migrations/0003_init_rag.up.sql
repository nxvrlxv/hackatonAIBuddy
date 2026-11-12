-- Extend the knowledge-base schema created by migration 0002.
BEGIN;

-- Add indexing metadata; source_path is already unique in migration 0002.
ALTER TABLE buddy.documents
    ADD COLUMN indexing_version TEXT NOT NULL DEFAULT 'legacy',
    ADD COLUMN updated_at TIMESTAMPTZ;

UPDATE buddy.documents SET updated_at = created_at;

ALTER TABLE buddy.documents
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL,
    ADD CONSTRAINT documents_source_path_not_empty CHECK (btrim(source_path) <> ''),
    ADD CONSTRAINT documents_indexing_version_not_empty CHECK (btrim(indexing_version) <> '');

-- Ingest must explicitly supply its indexing version on each insert/update.
-- Existing documents keep 'legacy', forcing reindexing with the actual version.
ALTER TABLE buddy.documents ALTER COLUMN indexing_version DROP DEFAULT;

-- Keep vector(1024) and the cosine HNSW index from migration 0002.
ALTER TABLE buddy.chunks
    ADD COLUMN section_title TEXT NOT NULL DEFAULT '',
    ADD CONSTRAINT chunks_index_nonnegative CHECK (chunk_index >= 0),
    ADD CONSTRAINT chunks_content_not_empty CHECK (btrim(content) <> '');

-- Ingest updates updated_at and replaces all document chunks in one transaction.
-- Queries allow chunks.team_id IS NULL or the authenticated user's team_id.
COMMIT;
