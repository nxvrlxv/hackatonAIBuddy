package config

import "time"

// Ingest is the settings contract; populate it using your configuration loader.
type Ingest struct {
	DatabaseURL         string
	KnowledgeDir        string
	EmbeddingModel      string
	EmbeddingServer     string // OpenAI-compatible API base URL, including /v1 if required.
	EmbeddingAPIKey     string
	EmbeddingDimensions int
	RequestTimeout      time.Duration
	MetadataBatchSize   int
	EmbeddingBatchSize  int
	ChunkSize           int
	ChunkOverlap        int
	IndexingVersion     string // Bump when replacing weights under the same model name.
}
