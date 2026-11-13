package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms/ollama"

	"github.com/nxvrlxv/ai-buddy/backend/internal/config"
	"github.com/nxvrlxv/ai-buddy/backend/internal/rag"
)

type options struct {
	indexVersion string
	knowledgeDir string
}

func parseOptions(args []string, output io.Writer) (options, error) {
	var opts options
	flags := flag.NewFlagSet("ingest", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.StringVar(&opts.indexVersion, "index-version", "", "Required indexing version (change when model or chunking changes)")
	flags.StringVar(&opts.indexVersion, "iv", "", "Alias for -index-version")
	flags.StringVar(&opts.knowledgeDir, "knowledge", "", "Override KNOWLEDGE_PATH from config")
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	if flags.NArg() != 0 {
		return opts, fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}
	opts.indexVersion = strings.TrimSpace(opts.indexVersion)
	if opts.indexVersion == "" {
		return opts, errors.New("specify -index-version or -iv, for example: -iv v1")
	}
	return opts, nil
}

func main() {
	if err := execute(os.Args[1:]); err != nil {
		log.Printf("ingest failed: %v", err)
		os.Exit(1)
	}
}

func execute(args []string) error {
	opts, err := parseOptions(args, os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, cfg, opts, log.Default())
}

func newEmbedder(cfg *config.Config) (*embeddings.EmbedderImpl, error) {
	endpoint, err := url.Parse(cfg.OllamaURL)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return nil, errors.New("OLLAMA_URL must be a valid HTTP(S) server URL")
	}
	if strings.TrimSpace(cfg.EmbedModel) == "" {
		return nil, errors.New("EMBEDDED_MODEL is required")
	}
	model, err := ollama.New(
		ollama.WithModel(cfg.EmbedModel),
		ollama.WithServerURL(cfg.OllamaURL),
		ollama.WithHTTPClient(&http.Client{Timeout: 2 * time.Minute}),
	)
	if err != nil {
		return nil, fmt.Errorf("create Ollama client: %w", err)
	}
	return embeddings.NewEmbedder(model, embeddings.WithStripNewLines(false))
}

func run(ctx context.Context, cfg *config.Config, opts options, logger *log.Logger) error {
	directory := cfg.FileStore
	if opts.knowledgeDir != "" {
		directory = opts.knowledgeDir
	}
	if strings.TrimSpace(directory) == "" {
		return errors.New("KNOWLEDGE_PATH is required")
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		return fmt.Errorf("resolve knowledge path: %w", err)
	}
	info, err := os.Stat(directory)
	if err != nil {
		return fmt.Errorf("knowledge directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("knowledge path must be a directory")
	}

	embedder, err := newEmbedder(cfg)
	if err != nil {
		return err
	}
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	db, err := pgx.Connect(connectCtx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = db.Close(cleanupCtx)
	}()
	// One dedicated connection keeps the session lock until this run ends.
	var locked bool
	if err := db.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, int64(7712839401)).Scan(&locked); err != nil {
		return fmt.Errorf("lock ingest: %w", err)
	}
	if !locked {
		return errors.New("another ingest is already running")
	}

	files, err := rag.FileSelection(directory, opts.indexVersion, ctx, db)
	if err != nil {
		return fmt.Errorf("select files: %w", err)
	}
	logger.Printf("selected=%d directory=%s version=%s model=%s", len(files), directory, opts.indexVersion, cfg.EmbedModel)
	updated, chunkCount := 0, 0
	defer func() {
		logger.Printf("ingest result: selected=%d saved=%d chunks=%d", len(files), updated, chunkCount)
	}()
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		document, err := prepareDocument(ctx, file, embedder)
		if err != nil {
			return fmt.Errorf("process %s: %w", file.SourcePath, err)
		}
		id, err := rag.SaveDocument(ctx, db, document)
		if err != nil {
			return fmt.Errorf("save %s: %w", file.SourcePath, err)
		}
		updated++
		chunkCount += len(document.Chunks)
		logger.Printf("saved %s: document_id=%d chunks=%d", file.SourcePath, id, len(document.Chunks))
	}
	return nil
}

// All model requests finish before SaveDocument begins a database transaction.
func prepareDocument(ctx context.Context, file rag.SelectedFile, embedder *embeddings.EmbedderImpl) (rag.DocumentToSave, error) {
	text, err := rag.ReadFileText(file.FullPath)
	if err != nil {
		return rag.DocumentToSave{}, err
	}
	chunks, err := rag.DefaultChunks(text)
	if err != nil {
		return rag.DocumentToSave{}, err
	}
	if len(chunks) == 0 {
		return rag.DocumentToSave{}, errors.New("document has no indexable text")
	}
	embedded, err := rag.BuildEmbeddings(ctx, embedder, chunks)
	if err != nil {
		return rag.DocumentToSave{}, err
	}
	title := strings.TrimSuffix(filepath.Base(file.SourcePath), filepath.Ext(file.SourcePath))
	for _, chunk := range chunks {
		if len(chunk.HeadingPath) > 0 && strings.TrimSpace(chunk.HeadingPath[0]) != "" {
			title = chunk.HeadingPath[0]
			break
		}
	}
	// Store the hash of the exact text embedded, even if it changed after selection.
	sum := sha256.Sum256([]byte(text))
	return rag.DocumentToSave{
		SourcePath: file.SourcePath, Title: title, TeamID: file.TeamID,
		ContentHash: hex.EncodeToString(sum[:]), IndexingVersion: file.IndexVersion,
		Chunks: embedded,
	}, nil
}
