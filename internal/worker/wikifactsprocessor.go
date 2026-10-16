// Package worker — wikifactsprocessor.go defines the WikiFactsProcessor
// interface that the wiki:populate_facts task delegates to. The spaces
// package (ProcessorAdapter) implements this interface and registers itself
// at startup via RegisterWikiFactsProcessor.
//
// Ref: japaconnect-engineering/sections/11-job-queue-asynq.md
package worker

import "context"

// WikiFactsProcessor defines the contract for enriching a space's wiki page
// custom fields with Wikidata general-info facts (geo: population, languages,
// currency, postal codes, calling code, capital, founded, head of government,
// time zone; university: founded year, students, website, initials).
// dryRun reports planned writes without persisting; overwrite replaces
// existing field values (default fills empty fields only).
type WikiFactsProcessor interface {
	PopulateWikiFacts(ctx context.Context, spaceID string, dryRun, overwrite bool) error
}

var wikiFactsProcessor WikiFactsProcessor

// RegisterWikiFactsProcessor registers the processor for the
// wiki:populate_facts background task. Call once at startup before the
// worker begins processing tasks.
func RegisterWikiFactsProcessor(p WikiFactsProcessor) {
	wikiFactsProcessor = p
}
