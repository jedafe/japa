package worker

import (
	"context"
)

// AnalyticsProcessor defines the interface for analytics background task processing.
// The analytics service implements this interface and registers with the worker.
type AnalyticsProcessor interface {
	RefreshViews(ctx context.Context) error
	RunSecurityScan(ctx context.Context) error
}

var analyticsProcessor AnalyticsProcessor

// RegisterAnalyticsProcessor registers the analytics service processor for background tasks.
func RegisterAnalyticsProcessor(p AnalyticsProcessor) {
	analyticsProcessor = p
}
