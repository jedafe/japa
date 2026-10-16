// Command jeos-indexer runs a one-shot §121 spec corpus reindex: it walks
// JC_SPEC_CORPUS_DIR (default .jeos), hash-skips unchanged spec files,
// chunk-embeds changed ones into content_embeddings as private spec_doc
// rows, and sweeps deleted paths.
//
// Same code path as the rag:reindex_specs Asynq task — the CLI exists for
// deploy hooks and local/manual runs. Requires the rag.spec_corpus feature
// flag and a DB-configured embedding provider (same providers the API
// gateway uses).
//
//	go run ./cmd/jeos-indexer [corpus-dir]
//
// Ref: japaconnect-engineering/sections/121-engineering-spec-corpus.md
//
//	japaconnect-engineering/adr/ADR-131-engineering-spec-corpus.md
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/features"
	"github.com/jedafe/japa/internal/rag"
	"github.com/jedafe/japa/internal/valkey"
)

func main() {
	if err := run(); err != nil {
		slog.Error("jeos-indexer failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	dir := cfg.SpecCorpusDir
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	database, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer database.Close()

	vkClient := valkey.NewWithFallbackConfig(valkey.ClientConfig{
		Addr:          cfg.RedisAddr,
		Password:      cfg.RedisPassword,
		DB:            cfg.RedisDB,
		PoolSize:      cfg.RedisPoolSize,
		DialTimeoutMs: cfg.RedisDialTimeoutMs,
		ReadTimeoutMs: cfg.RedisReadTimeoutMs,
		TLSEnabled:    cfg.RedisTLSEnabled,
	})

	settings := config.NewSettings(database, vkClient.Redis())
	checker := features.New(database, vkClient.Redis())

	// Register DB-backed providers so the embedder resolves the same catalog
	// the API uses.
	dispatcher := ai.NewDispatcher()
	providerStore := ai.NewProviderStore(database.Reader, database.Writer)
	if err := ai.LoadAndRegisterProviders(ctx, providerStore, dispatcher); err != nil {
		slog.Warn("jeos-indexer: loading db-backed providers", "error", err)
	}

	store := rag.NewStore(database)
	svc := rag.NewService(store, settings)
	svc.SetEmbedder(ai.NewRAGEmbedder(dispatcher))
	svc.SetFeatureChecker(checker)
	svc.SetCache(vkClient.Redis())

	stats, err := svc.ReindexSpecCorpus(ctx, dir)
	if err != nil {
		return err
	}

	slog.Info("jeos-indexer complete",
		"dir", dir,
		"files_scanned", stats.FilesScanned,
		"files_indexed", stats.FilesIndexed,
		"skipped_unchanged", stats.FilesSkipped,
		"skipped_too_big", stats.FilesSkippedTooBig,
		"chunks_written", stats.ChunksWritten,
		"sources_swept", stats.SourcesSwept)

	return nil
}
