package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	pgvectorpgx "github.com/pgvector/pgvector-go/pgx"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms/openai"

	"github.com/nxvrlxv/ai-buddy/backend/internal/config"
	"github.com/nxvrlxv/ai-buddy/backend/internal/rag"
)

func main() {
	cfg := readConfig()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

// Replace this adapter with your shared config loader when it is ready.
// No model or embedding server is selected implicitly.
func readConfig() config.Ingest {
	var cfg config.Ingest
	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	cfg.EmbeddingAPIKey = os.Getenv("EMBEDDING_API_KEY")
	flag.StringVar(&cfg.KnowledgeDir, "knowledge", "../data/knowledge", "knowledge directory (from backend/)")
	flag.StringVar(&cfg.EmbeddingModel, "model", os.Getenv("EMBEDDING_MODEL"), "embedding model")
	flag.StringVar(&cfg.EmbeddingServer, "server", os.Getenv("EMBEDDING_BASE_URL"), "OpenAI-compatible API base URL")
	flag.IntVar(&cfg.EmbeddingDimensions, "dimensions", 1024, "expected vector dimension; must match database")
	flag.IntVar(&cfg.MetadataBatchSize, "metadata-batch", 200, "source paths per metadata query")
	flag.IntVar(&cfg.EmbeddingBatchSize, "embedding-batch", 32, "texts per embedding request")
	flag.IntVar(&cfg.ChunkSize, "chunk-size", 600, "maximum embedding text length in Unicode code points")
	flag.IntVar(&cfg.ChunkOverlap, "chunk-overlap", 65, "section body overlap in Unicode code points")
	flag.DurationVar(&cfg.RequestTimeout, "request-timeout", 60*time.Second, "embedding HTTP request timeout")
	flag.StringVar(&cfg.IndexingVersion, "indexing-version", "v1", "manual model/pipeline revision")
	flag.Parse()
	return cfg
}

func run(ctx context.Context, cfg config.Ingest) error {
	endpoint, err := url.Parse(cfg.EmbeddingServer)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil {
		return errors.New("embedding server must be an HTTP(S) API base URL without credentials")
	}
	if cfg.DatabaseURL == "" || strings.TrimSpace(cfg.EmbeddingModel) == "" || cfg.RequestTimeout <= 0 ||
		cfg.EmbeddingBatchSize <= 0 || cfg.MetadataBatchSize <= 0 || cfg.EmbeddingDimensions <= 0 ||
		cfg.ChunkSize <= 0 || cfg.ChunkOverlap < 0 || cfg.ChunkOverlap >= cfg.ChunkSize || strings.TrimSpace(cfg.IndexingVersion) == "" {
		return errors.New("database URL, embedding model and valid ingest settings are required")
	}
	db, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = db.Close(cleanupCtx)
	}()
	// Session lock stays held on this dedicated connection across document transactions.
	const lockID int64 = 7712839401
	var locked bool
	if err := db.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, lockID).Scan(&locked); err != nil {
		return fmt.Errorf("lock ingest: %w", err)
	}
	if !locked {
		return errors.New("another ingest is already running")
	}
	// Closing this session releases the lock on both success and failure.
	var dimensions int
	err = db.QueryRow(ctx, `SELECT atttypmod FROM pg_attribute
        WHERE attrelid = 'buddy.chunks'::regclass AND attname = 'embedding' AND NOT attisdropped`).Scan(&dimensions)
	if err != nil {
		return fmt.Errorf("read database vector dimension (apply migrations first): %w", err)
	}
	if dimensions != cfg.EmbeddingDimensions {
		return fmt.Errorf("database vector dimension is %d, configured %d", dimensions, cfg.EmbeddingDimensions)
	}
	if err := pgvectorpgx.RegisterTypes(ctx, db); err != nil {
		return fmt.Errorf("register vector type: %w", err)
	}
	token := cfg.EmbeddingAPIKey
	if token == "" {
		token = "local-no-auth"
	} // SDK requires a token even for an unauthenticated local server.
	model, err := openai.New(openai.WithBaseURL(strings.TrimRight(cfg.EmbeddingServer, "/")),
		openai.WithEmbeddingModel(cfg.EmbeddingModel), openai.WithToken(token),
		openai.WithHTTPClient(&http.Client{Timeout: cfg.RequestTimeout}))
	if err != nil {
		return fmt.Errorf("create embedding client: %w", err)
	}
	embedder, err := embeddings.NewEmbedder(model, embeddings.WithBatchSize(cfg.EmbeddingBatchSize), embeddings.WithStripNewLines(false))
	if err != nil {
		return fmt.Errorf("create embedder: %w", err)
	}
	stats, err := rag.Ingest(ctx, rag.PostgresIngestStore{DB: db}, embedder, rag.IngestOptions{
		KnowledgeDir: cfg.KnowledgeDir, MetadataBatchSize: cfg.MetadataBatchSize,
		ChunkSize: cfg.ChunkSize, ChunkOverlap: cfg.ChunkOverlap,
		EmbeddingDimensions: cfg.EmbeddingDimensions, EmbeddingModel: cfg.EmbeddingModel,
		EmbeddingServer: cfg.EmbeddingServer, IndexingVersion: cfg.IndexingVersion,
	})
	log.Printf("ingest: files=%d updated=%d skipped=%d chunks=%d", stats.Files, stats.Updated, stats.Skipped, stats.Chunks)
	return err
}
