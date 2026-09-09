package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// VideoSummaryProcessor generates a content draft from a video source's
// metadata, title, description, and thumbnail context.
type VideoSummaryProcessor interface {
	GenerateDraft(ctx context.Context, tenantID, userID, videoSourceID pgtype.UUID) (gen.CreateContentRow, error)
}

var videoSummaryProcessor VideoSummaryProcessor

// RegisterVideoSummaryProcessor sets the processor for the video:generate_summary
// task. The video service implements this with its GenerateDraft method.
func RegisterVideoSummaryProcessor(p VideoSummaryProcessor) {
	videoSummaryProcessor = p
}

// GenerateSummaryPayload is the Asynq payload for TypeVideoGenerateSummary.
type GenerateSummaryPayload struct {
	TenantID      string `json:"tenant_id"`
	UserID        string `json:"user_id"`
	VideoSourceID string `json:"video_source_id"`
}

// HandleVideoGenerateSummary processes a video:generate_summary task.
// It parses the payload, validates the UUIDs, and delegates to the registered
// video summary processor.
func HandleVideoGenerateSummary(ctx context.Context, t *asynq.Task) error {
	slog.InfoContext(ctx, "task:video:generate_summary")

	var p GenerateSummaryPayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}

	if videoSummaryProcessor == nil {
		return fmt.Errorf("video summary processor not registered: %w", asynq.SkipRetry)
	}

	tenantID, err := scanUUID(p.TenantID)
	if err != nil {
		return fmt.Errorf("tenant_id: %w: %w", asynq.SkipRetry, err)
	}

	userID, err := scanUUID(p.UserID)
	if err != nil {
		return fmt.Errorf("user_id: %w: %w", asynq.SkipRetry, err)
	}

	sourceID, err := scanUUID(p.VideoSourceID)
	if err != nil {
		return fmt.Errorf("video_source_id: %w: %w", asynq.SkipRetry, err)
	}

	if _, err := videoSummaryProcessor.GenerateDraft(ctx, tenantID, userID, sourceID); err != nil {
		if errors.Is(err, errpkg.ErrNotFound) || errors.Is(err, errpkg.ErrAlreadyExists) {
			return fmt.Errorf("video generate summary permanent failure: %w: %w", err, asynq.SkipRetry)
		}

		return fmt.Errorf("task:video:generate_summary: %w", err)
	}

	return nil
}
