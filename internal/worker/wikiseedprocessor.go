// Package worker — wikiseedprocessor.go defines the
// WikiSeedStubPagesProcessor interface that the wiki:seed_stub_pages task
// delegates to. The wiki package (or a noop implementation) implements this
// interface and registers itself at startup via
// RegisterWikiSeedStubPagesProcessor.
//
// Ref: japaconnect-engineering/sections/80-destination-wiki.md
//      japaconnect-engineering/adr/ADR-088-destination-wiki.md
package worker

import "context"

// WikiSeedStubPagesProcessor defines the contract for auto-seeding the 7
// default wiki sub-pages (Overview, Visa Routes, Cost of Living, Healthcare,
// Banking, Community, Housing) for a Country Space when wiki.enabled is
// activated.
type WikiSeedStubPagesProcessor interface {
	SeedStubPages(ctx context.Context, spaceID, tenantID string) error
}

var wikiSeedStubPagesProcessor WikiSeedStubPagesProcessor

// RegisterWikiSeedStubPagesProcessor registers the wiki seed-stub-pages
// processor for the wiki:seed_stub_pages background task. Call once at
// startup before the worker begins processing tasks.
func RegisterWikiSeedStubPagesProcessor(p WikiSeedStubPagesProcessor) {
	wikiSeedStubPagesProcessor = p
}
