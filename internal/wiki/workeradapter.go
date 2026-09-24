// Package wiki — workeradapter.go adapts the wiki Service to the
// worker.WikiSeedStubPagesProcessor and worker.WikiFieldsPropagator
// interfaces so the wiki:seed_stub_pages and wiki:fields_propagate Asynq
// tasks can delegate to the service.
//
// Ref: japaconnect-engineering/sections/80-destination-wiki.md
//
//	japaconnect-engineering/adr/ADR-088-destination-wiki.md
package wiki

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// WorkerAdapter adapts *Service to the worker.WikiSeedStubPagesProcessor and
// worker.WikiFieldsPropagator interfaces. It converts the string IDs from the
// Asynq payload to pgtype.UUID before delegating to Service.SeedStubPages;
// PropagateFields takes string identifiers natively and is passed through.
type WorkerAdapter struct {
	svc *Service
}

// NewWorkerAdapter creates a WorkerAdapter wrapping the given service.
func NewWorkerAdapter(svc *Service) *WorkerAdapter {
	return &WorkerAdapter{svc: svc}
}

// SeedStubPages implements worker.WikiSeedStubPagesProcessor.
func (a *WorkerAdapter) SeedStubPages(ctx context.Context, spaceID string) error {
	var sid pgtype.UUID
	if err := sid.Scan(spaceID); err != nil {
		return err
	}

	return a.svc.SeedStubPages(ctx, sid)
}

// PropagateFields implements worker.WikiFieldsPropagator — the
// entity_type/entity_id payload strings pass straight through to
// Service.PropagateFields, which delegates to the injected JCE cache buster
// (§80 Community Field Ops → §30 Sub-spec 5 propagation).
func (a *WorkerAdapter) PropagateFields(ctx context.Context, entityType, entityID string) error {
	return a.svc.PropagateFields(ctx, entityType, entityID)
}
