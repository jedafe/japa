package worker

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
)

// BulkImportPayload is the Asynq payload for TypeVideoImportBulk.
type BulkImportPayload struct {
	ImportJobID string `json:"import_job_id"`
	TenantID    string `json:"tenant_id"`
	UserID      string `json:"user_id"`
}

// HandleVideoImportBulk processes a video:import_bulk task.
func HandleVideoImportBulk(ctx context.Context, t *asynq.Task) error {
	var p BulkImportPayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}

	if videoImportProcessor == nil {
		return fmt.Errorf("video import processor not registered: %w", asynq.SkipRetry)
	}

	if _, err := scanUUID(p.ImportJobID); err != nil {
		return fmt.Errorf("import_job_id: %w: %w", asynq.SkipRetry, err)
	}

	if _, err := scanUUID(p.TenantID); err != nil {
		return fmt.Errorf("tenant_id: %w: %w", asynq.SkipRetry, err)
	}

	if _, err := scanUUID(p.UserID); err != nil {
		return fmt.Errorf("user_id: %w: %w", asynq.SkipRetry, err)
	}

	return videoImportProcessor.ProcessBulkImport(ctx, p)
}
