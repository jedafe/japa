// Package worker — wikifieldsprocessor.go defines the WikiFieldsPropagator
// interface that the wiki:fields_propagate task delegates to. The wiki
// package's WorkerAdapter implements this interface and registers itself at
// startup via RegisterWikiFieldsPropagator.
//
// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Community Field Ops)
//
//	japaconnect-engineering/sections/30-generic-comparison-system.md (Sub-spec 5)
//	japaconnect-engineering/adr/ADR-088-destination-wiki.md
package worker

import "context"

// WikiFieldsPropagator defines the contract for propagating an approved wiki
// community-fields revision into JCE comparison caches — busting every
// jc:jce:* cache key for comparisons containing the target entity's item_ref
// (§80 Community Field Ops → §30 Sub-spec 5 propagation).
type WikiFieldsPropagator interface {
	PropagateFields(ctx context.Context, entityType, entityID string) error
}

var wikiFieldsPropagator WikiFieldsPropagator

// RegisterWikiFieldsPropagator registers the wiki fields propagator for the
// wiki:fields_propagate background task. Call once at startup before the
// worker begins processing tasks.
func RegisterWikiFieldsPropagator(p WikiFieldsPropagator) {
	wikiFieldsPropagator = p
}
