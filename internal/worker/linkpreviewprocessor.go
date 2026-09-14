// Package worker — linkpreviewprocessor.go defines the LinkPreviewProcessor
// interface that the link_preview_fetch task delegates to. The linkpreview
// package's Fetcher implements this interface and registers itself at
// startup via RegisterLinkPreviewProcessor.
package worker

import "context"

// LinkPreviewProcessor defines the contract for fetching and storing link
// preview metadata for a given URL. The worker calls FetchAndStore from
// the link_preview_fetch task handler; the linkpreview package owns the
// SSRF-safe fetch, OG tag parsing, and cache/storage write.
type LinkPreviewProcessor interface {
	FetchAndStore(ctx context.Context, url, blockID string) error
}

var linkPreviewProcessor LinkPreviewProcessor

// RegisterLinkPreviewProcessor registers the link preview processor for the
// link_preview_fetch background task. Call once at startup before the worker
// begins processing tasks.
func RegisterLinkPreviewProcessor(p LinkPreviewProcessor) {
	linkPreviewProcessor = p
}
