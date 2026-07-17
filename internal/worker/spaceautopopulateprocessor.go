// Package worker — spaceautopopulateprocessor.go defines the
// SpaceAutoPopulateProcessor interface that the space:auto_populate task
// delegates to. The spaces package (or a noop implementation) implements
// this interface and registers itself at startup via
// RegisterSpaceAutoPopulateProcessor.
package worker

import "context"

// SpaceAutoPopulateProcessor defines the contract for seeding a newly
// created space with default content — rules, tags, or sample posts
// based on the space type.
type SpaceAutoPopulateProcessor interface {
	AutoPopulateSpace(ctx context.Context, spaceID, tenantID string) error
}

var spaceAutoPopulateProcessor SpaceAutoPopulateProcessor

// RegisterSpaceAutoPopulateProcessor registers the space auto-populate
// processor for the space:auto_populate background task. Call once at
// startup before the worker begins processing tasks.
func RegisterSpaceAutoPopulateProcessor(p SpaceAutoPopulateProcessor) {
	spaceAutoPopulateProcessor = p
}
