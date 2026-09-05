// Package wiki — workeradapter.go adapts the wiki Service to the
// worker.WikiSeedStubPagesProcessor interface so the wiki:seed_stub_pages
// Asynq task can delegate to the service.
//
// Ref: japaconnect-engineering/sections/80-destination-wiki.md
//
//	japaconnect-engineering/adr/ADR-088-destination-wiki.md
package wiki

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// WorkerAdapter adapts *Service to the worker.WikiSeedStubPagesProcessor
// interface. It converts the string IDs from the Asynq payload to pgtype.UUID
// before delegating to Service.SeedStubPages.
type WorkerAdapter struct {
	svc *Service
}

// NewWorkerAdapter creates a WorkerAdapter wrapping the given service.
func NewWorkerAdapter(svc *Service) *WorkerAdapter {
	return &WorkerAdapter{svc: svc}
}

// SeedStubPages implements worker.WikiSeedStubPagesProcessor.
func (a *WorkerAdapter) SeedStubPages(ctx context.Context, spaceID, tenantID string) error {
	var tid, sid pgtype.UUID
	if err := tid.Scan(tenantID); err != nil {
		return err
	}

	if err := sid.Scan(spaceID); err != nil {
		return err
	}

	return a.svc.SeedStubPages(ctx, tid, sid)
}
