-- Restore the schema from migration 0002, preserving its unique source paths.
BEGIN;
ALTER TABLE buddy.chunks
    DROP CONSTRAINT chunks_index_nonnegative,
    DROP CONSTRAINT chunks_content_not_empty,
    DROP COLUMN section_title;

ALTER TABLE buddy.documents
    DROP CONSTRAINT documents_source_path_not_empty,
    DROP CONSTRAINT documents_indexing_version_not_empty,
    DROP COLUMN indexing_version,
    DROP COLUMN updated_at;
COMMIT;
