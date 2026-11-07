// Package worker — search.go defines the SearchIndexer interface that the
// search_index_sync task delegates to. The search package's Indexer
// implements this interface and registers itself at startup via
// RegisterSearchIndexer, mirroring the GDPR and analytics processor pattern.
package worker

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
)

// SearchIndexer defines the contract for syncing documents into the
// Meilisearch search index. The worker calls the appropriate method from
// the search_index_sync task handler based on entity_type; the search
// package owns the DB fetch and document shaping.
type SearchIndexer interface {
	IndexContent(ctx context.Context, contentID [16]byte) error
	IndexForumThread(ctx context.Context, threadID [16]byte) error
	IndexSpace(ctx context.Context, spaceID [16]byte) error
	IndexUser(ctx context.Context, userID [16]byte) error
	IndexTag(ctx context.Context, tagID [16]byte) error
	// IndexDirectoryTag indexes a tags-table directory tag (the /tags
	// surface) into the shared tags index — entity_type "directory_tag".
	IndexDirectoryTag(ctx context.Context, tagID [16]byte) error
	IndexAcademyTrack(ctx context.Context, trackID [16]byte) error
	IndexDocPage(ctx context.Context, pageID [16]byte) error
	IndexOrganization(ctx context.Context, orgID [16]byte) error
	IndexLanguageTestRequirement(ctx context.Context, reqID [16]byte) error
	// §§107–110 SEO growth tools (ADR-118..121) — JapaBot RAG documents.
	IndexPofRequirement(ctx context.Context, reqID [16]byte) error
	IndexEcaGuide(ctx context.Context, guideID [16]byte) error
	IndexSopSample(ctx context.Context, sampleID [16]byte) error
	DeleteDocument(ctx context.Context, indexName string, id [16]byte) error
	// ReindexAll bulk-resyncs every managed entity type; the return is the
	// indexer's summary struct exposed as `any` so this seam stays free of
	// search-package types (the handler marshals it for logging).
	ReindexAll(ctx context.Context) (any, error)
	// PurgeAll clears all documents from the ReindexAll-managed indexes,
	// keeping index schemas. Pair with ReindexAll for a clean rebuild.
	PurgeAll(ctx context.Context) error
}

var searchIndexer SearchIndexer

// RegisterSearchIndexer registers the search indexer for the
// search_index_sync background task. Call once at startup before the worker
// begins processing tasks.
func RegisterSearchIndexer(ix SearchIndexer) {
	searchIndexer = ix
}

// EnqueueSearchIndexSync enqueues a search:index_entity task so the entity's
// search document is re-synced (upsert or delete, decided by the indexer
// based on current row state). Mirrors EnqueueContentEmbedding — call it on
// every lifecycle transition that changes indexability (publish, archive,
// schedule, moderation hide) not just HTTP update routes. Fire-and-forget:
// errors are logged but never propagated; a nil enqueuer is a no-op.
func EnqueueSearchIndexSync(ctx context.Context, entityType string, entityID pgtype.UUID) {
	if taskEnqueuer == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"entity_id":   entityID.String(),
		"entity_type": entityType,
		"operation":   "index",
	})
	if err != nil {
		slog.Warn("worker: search index enqueue marshal failed",
			"entity_type", entityType, "entity_id", entityID.String(), "error", err)
		return
	}

	if err := taskEnqueuer.Enqueue(TypeSearchIndexSync, payload); err != nil {
		slog.Warn("worker: search index enqueue failed",
			"entity_type", entityType, "entity_id", entityID.String(), "error", err)
	}
}
