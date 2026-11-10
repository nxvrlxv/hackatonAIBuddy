-- Restore the schema from migration 0002 without deleting indexed documents.
BEGIN;

-- This fails atomically if equal-content documents were added after migration 0003.
-- Resolve these duplicates explicitly before attempting the rollback again.
ALTER TABLE buddy.documents
    DROP CONSTRAINT documents_source_path_key,
    DROP CONSTRAINT documents_source_path_not_empty,
    DROP CONSTRAINT documents_indexing_version_not_empty,
    DROP COLgit switch -cUMN indexing_version,
    DROP COLUMN updated_at;

ALTER TABLE buddy.chunks
    DROP CONSTRAINT chunks_index_nonnegative,
    DROP CONSTRAINT chunks_content_not_empty,
    DROP COLUMN section_title;

COMMIT;
