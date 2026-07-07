package worker

import "context"

// TrendingProcessor defines the interface for trending background task processing.
// The trending service implements this interface and registers with the worker.
type TrendingProcessor interface {
	ComputeTrending(ctx context.Context) error
	ComputeSuggestions(ctx context.Context) error
}

var trendingProcessor TrendingProcessor

// RegisterTrendingProcessor registers the trending service processor for background tasks.
func RegisterTrendingProcessor(p TrendingProcessor) {
	trendingProcessor = p
}
