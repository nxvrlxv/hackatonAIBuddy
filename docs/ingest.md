# Knowledge ingestion

Run from `backend/` after applying SQL migrations 0001 through 0004 and seeding `buddy.teams`:

```sh
export DATABASE_URL='postgres://user:password@localhost:5432/buddy'
export EMBEDDING_BASE_URL='http://localhost:8000/v1'
export EMBEDDING_MODEL='your-embedding-model'
# Set EMBEDDING_API_KEY if the model server requires authentication.
go run ./cmd/ingest -knowledge ../data/knowledge -dimensions 1024
```

The model endpoint must implement the OpenAI-compatible embeddings API. The model
name and API base URL are required; no provider/model is selected by default.
The current database migration uses `vector(1024)`. Ingest checks the actual database
column dimension before embedding and checks every returned vector before writing.
Changing a CLI dimension does not change the database schema.

## Configuration contract

`internal/config.Ingest` defines the settings expected by `cmd/ingest.run`.
Replace `cmd/ingest.readConfig` with your shared configuration loader when ready.
The current adapter accepts the environment variables above and these flags:

- `-model`, `-server`: model and API base URL (environment values are defaults).
- `-knowledge`: source directory, default `../data/knowledge` relative to the working directory.
- `-dimensions`: expected embedding dimension, default 1024.
- `-metadata-batch`: paths per metadata query, default 200.
- `-embedding-batch`: texts per embedding API request, default 32.
- `-chunk-size`, `-chunk-overlap`: Unicode code point budgets, defaults 600 and 65.
- `-request-timeout`: timeout per model HTTP request, default 60s.
- `-indexing-version`: manual model/pipeline revision, default `v1`.

## Incremental indexing

1. Discover Markdown source paths relative to the knowledge directory.
2. Read team codes once. Read existing document metadata in batches using
   `WHERE source_path = ANY($1::text[])`; chunks and vectors are not fetched.
3. Compute SHA-256 of each file's raw bytes. Skip only when its hash, effective
   indexing version and team scope all match the stored document.
4. Split changed documents by Markdown heading, then split each section. Embed
   the heading prefix and body together and L2-normalize the returned vector.
5. In one transaction per document, upsert metadata, remove old chunks, and bulk
   insert new chunks with COPY. Embedding requests occur before the transaction.

The effective version fingerprints the model, server URL, dimension, chunk size,
overlap, manual revision and a pipeline marker. Bump the manual revision if model
weights change under the same name. When changing pipeline code, bump its marker.
The same pipeline/model settings must be used for search. Switching models requires
reindexing the full corpus; avoid serving a mixed old/new index during that change.

A session advisory lock permits only one CLI ingest process at a time. A failed run
exits nonzero and keeps already committed documents; the next run skips them.
Deleted/renamed local files are not automatically removed from the database.
Source paths must be under `common/` or `team-*/`. Team folders match `teams.code`
literally, or without the `team-` prefix; ambiguous or missing codes are errors.
Symlinks, empty files and heading-only files are rejected.

Migration 0004 repairs source identity even if the modified 0003 was already applied:
paths become unique and equal content is allowed in different documents. Existing
duplicate paths must be resolved explicitly before applying it. Rollback to the old
content-hash uniqueness can fail if equal-content sources have since been indexed;
it does not delete documents to make the rollback succeed.

## Checks

```sh
go test ./...
go vet ./...
```

Tests use a fake embedding client and mocked PostgreSQL transactions. They do not
require or call a real model/database. Apply migrations and run against your actual
PostgreSQL + pgvector deployment for an end-to-end check.
