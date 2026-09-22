package curator

import (
	"context"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// WebSub curates webhook crawl-method channels: a hub delivery body is an
// Atom or RSS feed fragment — exactly the shapes the RSSNews curator
// normalizes, so this is a thin Handles() adapter over the shared feed
// pipeline.
type WebSub struct{}

// Handles returns the crawl method this curator serves.
func (WebSub) Handles() string { return string(sources.CrawlMethodWebhook) }

// Curate delegates to the shared feed parser (RSS items, Atom entries).
func (WebSub) Curate(_ context.Context, body []byte, _ string, ch sources.Channel) ([]Record, error) {
	return curateFeedItems(body, ch)
}
