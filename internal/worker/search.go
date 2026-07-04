// Package worker — search.go defines the SearchIndexer interface that the
// search_index_sync task delegates to. The search package's Indexer
// implements this interface and registers itself at startup via
// RegisterSearchIndexer, mirroring the GDPR and analytics processor pattern.
package worker

import "context"

// SearchIndexer defines the contract for syncing documents into the
// Meilisearch search index. The worker calls the appropriate method from
// the search_index_sync task handler based on entity_type; the search
// package owns the DB fetch and document shaping.
type SearchIndexer interface {
	IndexContent(ctx context.Context, contentID, tenantID [16]byte) error
	IndexForumThread(ctx context.Context, threadID, tenantID [16]byte) error
	IndexSpace(ctx context.Context, spaceID, tenantID [16]byte) error
	IndexUser(ctx context.Context, userID, tenantID [16]byte) error
	IndexTag(ctx context.Context, tagID, tenantID [16]byte) error
	DeleteDocument(ctx context.Context, indexName string, id [16]byte) error
}

var searchIndexer SearchIndexer

// RegisterSearchIndexer registers the search indexer for the
// search_index_sync background task. Call once at startup before the worker
// begins processing tasks.
func RegisterSearchIndexer(ix SearchIndexer) {
	searchIndexer = ix
}
