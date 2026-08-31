package worker

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

// IngestionService is the narrow worker-side view of the JCE ingestion service.
// It is implemented by *jce.Service and registered once at server startup via
// RegisterJCEService. Keeping the interface in the worker package avoids an
// import cycle with the jce package (which imports comparison → content → ...).
type IngestionService interface {
	ProcessRefreshValues(ctx context.Context, tenantID, contentID, jobID pgtype.UUID, sourceKind *string, userID pgtype.UUID) error
}

// jceIngestionSvc is the worker's view of the JCE ingestion service. It is
// registered once at server startup via RegisterJCEService.
var jceIngestionSvc IngestionService

// RegisterJCEService wires the application JCE service into the worker package.
// This is called from internal/server/server.go after both the worker and the
// JCE service have been constructed.
func RegisterJCEService(svc IngestionService) {
	jceIngestionSvc = svc
}

// JceRefreshValuesPayload is the Asynq payload for TypeJCERefreshValues.
type JceRefreshValuesPayload struct {
	TenantID       string  `json:"tenant_id"`
	ContentID      string  `json:"content_id"`
	SourceKind     *string `json:"source_kind,omitempty"`
	IngestionJobID string  `json:"ingestion_job_id"`
	RequestedBy    string  `json:"requested_by_user_id"`
}

// HandleJceRefreshValues processes a JCE refresh-values task.
// It validates the payload, then delegates to the registered JCE ingestion service.
// Malformed payloads return asynq.SkipRetry; transient errors are returned so the
// Asynq retry/back-off logic can run.
func HandleJceRefreshValues(ctx context.Context, t *asynq.Task) error {
	var p JceRefreshValuesPayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}

	if jceIngestionSvc == nil {
		return fmt.Errorf("jce ingestion service not registered: %w", asynq.SkipRetry)
	}

	tenantID, err := scanUUID(p.TenantID)
	if err != nil {
		return fmt.Errorf("tenant_id: %w: %v", asynq.SkipRetry, err)
	}
	contentID, err := scanUUID(p.ContentID)
	if err != nil {
		return fmt.Errorf("content_id: %w: %v", asynq.SkipRetry, err)
	}
	jobID, err := scanUUID(p.IngestionJobID)
	if err != nil {
		return fmt.Errorf("ingestion_job_id: %w: %v", asynq.SkipRetry, err)
	}
	userID, err := scanUUID(p.RequestedBy)
	if err != nil {
		return fmt.Errorf("requested_by_user_id: %w: %v", asynq.SkipRetry, err)
	}

	return jceIngestionSvc.ProcessRefreshValues(ctx, tenantID, contentID, jobID, p.SourceKind, userID)
}
