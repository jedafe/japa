package analytics

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// WorkerAdapter adapts the analytics Service to the worker.AnalyticsProcessor interface.
// This allows the Asynq worker to call the analytics service methods.
type WorkerAdapter struct {
	Service *Service
}

// NewWorkerAdapter creates a WorkerAdapter wrapping the given Service.
func NewWorkerAdapter(service *Service) *WorkerAdapter {
	return &WorkerAdapter{Service: service}
}

// RefreshViews refreshes all materialized views concurrently.
func (a *WorkerAdapter) RefreshViews(ctx context.Context) error {
	return a.Service.RefreshViews(ctx)
}

// RunSecurityScan runs the security scanner for the given tenant.
func (a *WorkerAdapter) RunSecurityScan(ctx context.Context, tenantID [16]byte) error {
	id := pgtype.UUID{Bytes: tenantID, Valid: true}
	return a.Service.RunSecurityScan(ctx, id)
}
