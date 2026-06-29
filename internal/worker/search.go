// Package worker — search.go defines the SearchIndexer interface that the
// search_index_sync task delegates to. The search package's Indexer
// implements this interface and registers itself at startup via
// RegisterSearchIndexer, mirroring the GDPR and analytics processor pattern.
package worker

import "context"

// SearchIndexer defines the contract for syncing a single content document
// into the Meilisearch search index. The worker calls IndexContent from the
// search_index_sync task handler; the search package owns the DB fetch and
// document shaping.
type SearchIndexer interface {
	IndexContent(ctx context.Context, contentID, tenantID [16]byte) error
}

var searchIndexer SearchIndexer

// RegisterSearchIndexer registers the search indexer for the
// search_index_sync background task. Call once at startup before the worker
// begins processing tasks.
func RegisterSearchIndexer(ix SearchIndexer) {
	searchIndexer = ix
}
