package rag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tmc/langchaingo/embeddings"
)

type DocumentMetadata struct {
	ContentHash     string
	IndexingVersion string
	TeamID          *int64
}

type IndexedDocument struct {
	SourcePath      string
	Title           string
	TeamID          *int64
	ContentHash     string
	IndexingVersion string
	Chunks          []EmbeddedChunk
}

type IngestStore interface {
	LoadMetadata(context.Context, []string) (map[string]DocumentMetadata, error)
	LoadTeams(context.Context) (map[string]int64, error)
	SaveDocument(context.Context, IndexedDocument) error
}

type IngestOptions struct {
	KnowledgeDir        string
	MetadataBatchSize   int
	ChunkSize           int
	ChunkOverlap        int
	EmbeddingDimensions int
	EmbeddingModel      string
	EmbeddingServer     string
	IndexingVersion     string
}

type IngestStats struct {
	Files   int
	Updated int
	Skipped int
	Chunks  int
}

// EffectiveIndexingVersion fingerprints all settings that affect embeddings.
// IndexingVersion identifies model weights/manual pipeline changes; it is not
// sufficient by itself, since model, server and chunk settings also matter.
func EffectiveIndexingVersion(opts IngestOptions) string {
	settings := struct {
		Pipeline     string
		Model        string
		Server       string
		Revision     string
		Dimensions   int
		ChunkSize    int
		ChunkOverlap int
	}{"markdown-goldmark-root-headings/runes/section-prefix/l2-v1", opts.EmbeddingModel,
		strings.TrimRight(opts.EmbeddingServer, "/"), opts.IndexingVersion,
		opts.EmbeddingDimensions, opts.ChunkSize, opts.ChunkOverlap}
	data, _ := json.Marshal(settings)
	sum := sha256.Sum256(data)
	return "rag-v1:" + hex.EncodeToString(sum[:])
}

// Ingest reads only the metadata for each batch of local paths. Embeddings are
// computed before SaveDocument opens its transaction. It fails on the first error;
// already committed documents remain valid and are skipped on the next run.
// The caller must prevent concurrent ingest runs (the CLI uses an advisory lock).
func Ingest(ctx context.Context, store IngestStore, embedder *embeddings.EmbedderImpl, opts IngestOptions) (IngestStats, error) {
	var stats IngestStats
	if strings.TrimSpace(opts.KnowledgeDir) == "" || opts.MetadataBatchSize <= 0 || opts.EmbeddingDimensions <= 0 ||
		opts.ChunkSize <= 0 || opts.ChunkOverlap < 0 || opts.ChunkOverlap >= opts.ChunkSize ||
		strings.TrimSpace(opts.EmbeddingModel) == "" || strings.TrimSpace(opts.EmbeddingServer) == "" || strings.TrimSpace(opts.IndexingVersion) == "" {
		return stats, fmt.Errorf("invalid ingest settings")
	}
	root, err := filepath.Abs(opts.KnowledgeDir)
	if err != nil {
		return stats, fmt.Errorf("knowledge directory: %w", err)
	}
	info, err := os.Lstat(root)
	if err != nil {
		return stats, fmt.Errorf("knowledge directory: %w", err)
	}
	if !info.IsDir() {
		return stats, fmt.Errorf("knowledge path must be a directory, not a file or symlink")
	}
	var paths []string
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not supported in knowledge directory: %s", path)
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("knowledge file is not a regular file: %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return stats, fmt.Errorf("scan knowledge: %w", err)
	}
	stats.Files = len(paths)
	if len(paths) == 0 {
		return stats, nil
	}
	teams, err := store.LoadTeams(ctx)
	if err != nil {
		return stats, fmt.Errorf("load teams: %w", err)
	}
	version := EffectiveIndexingVersion(opts)
	for start := 0; start < len(paths); start += opts.MetadataBatchSize {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		batch := paths[start:min(start+opts.MetadataBatchSize, len(paths))]
		existing, err := store.LoadMetadata(ctx, batch)
		if err != nil {
			return stats, fmt.Errorf("load document metadata: %w", err)
		}
		for _, path := range batch {
			if err := ctx.Err(); err != nil {
				return stats, err
			}
			teamID, err := documentTeam(path, teams)
			if err != nil {
				return stats, err
			}
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
			if err != nil {
				return stats, fmt.Errorf("read %s: %w", path, err)
			}
			sum := sha256.Sum256(data)
			hash := hex.EncodeToString(sum[:])
			previous, found := existing[path]
			if found && previous.ContentHash == hash && previous.IndexingVersion == version && sameTeam(previous.TeamID, teamID) {
				stats.Skipped++
				continue
			}
			chunks, err := SplitMarkdown(string(data), opts.ChunkSize, opts.ChunkOverlap)
			if err != nil {
				return stats, fmt.Errorf("split %s: %w", path, err)
			}
			if len(chunks) == 0 {
				return stats, fmt.Errorf("%s has no indexable content", path)
			}
			embedded, err := BuildEmbeddings(ctx, embedder, chunks)
			if err != nil {
				return stats, fmt.Errorf("embed %s: %w", path, err)
			}
			for _, chunk := range embedded {
				if len(chunk.Embedding) != opts.EmbeddingDimensions {
					return stats, fmt.Errorf("%s: expected embedding dimension %d, got %d", path, opts.EmbeddingDimensions, len(chunk.Embedding))
				}
			}
			title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			if len(chunks[0].HeadingPath) > 0 {
				title = chunks[0].HeadingPath[0]
			}
			document := IndexedDocument{SourcePath: path, Title: title, TeamID: teamID,
				ContentHash: hash, IndexingVersion: version, Chunks: embedded}
			if err := store.SaveDocument(ctx, document); err != nil {
				return stats, fmt.Errorf("save %s: %w", path, err)
			}
			stats.Updated++
			stats.Chunks += len(embedded)
		}
	}
	// Missing local files are deliberately not deleted from the database.
	return stats, nil
}

func documentTeam(path string, teams map[string]int64) (*int64, error) {
	folder, _, ok := strings.Cut(path, "/")
	if !ok {
		return nil, fmt.Errorf("%s: put documents under common/ or team-*/", path)
	}
	if folder == "common" {
		return nil, nil
	}
	if !strings.HasPrefix(folder, "team-") {
		return nil, fmt.Errorf("%s: unknown knowledge scope", path)
	}
	// Prefer the literal folder code; also support teams.code without 'team-'.
	id, exists := teams[folder]
	shortID, shortExists := teams[strings.TrimPrefix(folder, "team-")]
	if exists && shortExists && id != shortID {
		return nil, fmt.Errorf("%s: ambiguous team code", path)
	}
	if !exists {
		id, exists = shortID, shortExists
	}
	if !exists {
		return nil, fmt.Errorf("%s: team is not present in buddy.teams", path)
	}
	return &id, nil
}

func sameTeam(a, b *int64) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
