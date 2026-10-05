// Package worker — wikiseedattractionsprocessor.go defines the
// WikiSeedAttractionsProcessor interface that the wiki:seed_attractions task
// delegates to. The spaces package (ProcessorAdapter) implements this
// interface and registers itself at startup via
// RegisterWikiSeedAttractionsProcessor.
//
// Ref: japaconnect-engineering/sections/80-destination-wiki.md
// (Wiki-Only Space Types & Attractions amendment)
package worker

import "context"

// WikiSeedAttractionsProcessor defines the contract for bulk-seeding
// attraction wiki spaces under a geo parent (country/state/city) from a
// Wikidata SPARQL query. dryRun reports candidates without writes.
type WikiSeedAttractionsProcessor interface {
	SeedAttractions(ctx context.Context, parentSpaceID string, limit int, dryRun bool) error
}

var wikiSeedAttractionsProcessor WikiSeedAttractionsProcessor

// RegisterWikiSeedAttractionsProcessor registers the processor for the
// wiki:seed_attractions background task. Call once at startup before the
// worker begins processing tasks.
func RegisterWikiSeedAttractionsProcessor(p WikiSeedAttractionsProcessor) {
	wikiSeedAttractionsProcessor = p
}
