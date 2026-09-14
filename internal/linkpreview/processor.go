// Package linkpreview — processor.go adapts the Fetcher to implement the
// worker.LinkPreviewProcessor interface so the link_preview_fetch Asynq
// handler can delegate SSRF-safe fetching and OG tag parsing to the
// existing Fetcher, then persist the preview data.
package linkpreview

import (
	"context"
	"log/slog"
)

// ProcessorAdapter wraps a Fetcher to implement the worker.LinkPreviewProcessor
// interface. It fetches and caches link previews but does not yet persist
// to a database block — that integration is wired when the content block
// system stores preview references.
type ProcessorAdapter struct {
	fetcher *Fetcher
}

// NewProcessorAdapter creates a new ProcessorAdapter.
func NewProcessorAdapter(fetcher *Fetcher) *ProcessorAdapter {
	return &ProcessorAdapter{fetcher: fetcher}
}

// FetchAndStore fetches a link preview with SSRF protection and stores it.
// The blockID and ID parameters are reserved for future use when
// content blocks store preview references in the database.
func (a *ProcessorAdapter) FetchAndStore(ctx context.Context, url, blockID string) error {
	preview, err := a.fetcher.Fetch(ctx, url)
	if err != nil {
		return err
	}

	slog.Info("link_preview: fetched and cached", "url", url, "title", preview.Title, "block_id", blockID)

	return nil
}
