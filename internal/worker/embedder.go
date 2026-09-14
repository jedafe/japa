// Package worker — embedder.go defines the ContentEmbedder interface that the
// ai:generate_content_embedding task delegates to. The ai package's embedding
// logic implements this interface and registers itself at startup via
// RegisterContentEmbedder, mirroring the SearchIndexer and GDPR processor patterns.
package worker

import "context"

// ContentEmbedder defines the contract for generating a vector embedding for
// a single content document and storing it in the database. The worker calls
// EmbedContent from the ai:generate_content_embedding task handler; the ai
// package owns the provider call and DB write.
type ContentEmbedder interface {
	// EmbedContent generates an embedding for the content identified by
	// contentID+tenantID and stores it in content_scores.relevance_embedding.
	// The text parameter contains the content body to embed.
	EmbedContent(ctx context.Context, contentID [16]byte, text string) error
}

var contentEmbedder ContentEmbedder

// RegisterContentEmbedder registers the content embedder for the
// ai:generate_content_embedding background task. Call once at startup before
// the worker begins processing tasks.
func RegisterContentEmbedder(ce ContentEmbedder) {
	contentEmbedder = ce
}
