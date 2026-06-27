package gdpr

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// WorkerAdapter adapts the GDPR Service to the worker.GDPRProcessor interface.
// This allows the Asynq worker to call the GDPR service methods.
type WorkerAdapter struct {
	Service *Service
}

// NewWorkerAdapter creates a WorkerAdapter wrapping the given Service.
func NewWorkerAdapter(service *Service) *WorkerAdapter {
	return &WorkerAdapter{Service: service}
}

// ProcessDataExport processes a data export request by ID.
func (a *WorkerAdapter) ProcessDataExport(ctx context.Context, requestID [16]byte) error {
	id := pgtype.UUID{Bytes: requestID, Valid: true}
	return a.Service.ProcessDataExport(ctx, id)
}

// ProcessAccountDeletion processes an account deletion request by ID.
func (a *WorkerAdapter) ProcessAccountDeletion(ctx context.Context, requestID [16]byte) error {
	id := pgtype.UUID{Bytes: requestID, Valid: true}
	return a.Service.ProcessAccountDeletion(ctx, id)
}
