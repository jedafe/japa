package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

type sourceMetadataPayload struct {
	SourceID string `json:"source_id"`
	TenantID string `json:"tenant_id"`
}

// Preview is the subset of public-site metadata needed by the source
// directory. It is defined in the worker package so the worker does not need
// to import the linkpreview package (which contains HTTP handlers and creates
// an import cycle).
type Preview struct {
	Description string
	Favicon     string
	Image       string
	Keywords    []string
}

// SourceLinkPreviewer fetches public-site metadata for a URL.
type SourceLinkPreviewer interface {
	Fetch(ctx context.Context, rawURL string, tenantID pgtype.UUID) (*Preview, error)
}

var sourceLinkPreviewerInstance SourceLinkPreviewer

// RegisterSourceLinkPreviewer wires the link preview fetcher for source
// metadata tasks.
func RegisterSourceLinkPreviewer(p SourceLinkPreviewer) {
	sourceLinkPreviewerInstance = p
}

// HandleSourceMetadata loads the source, fetches its homepage link preview,
// and writes the description and logo back to the row if the current values are
// empty. Failures are logged and swallowed so a single unreachable site does not
// retry indefinitely.
func HandleSourceMetadata(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelSourceMetadata")

	slog.Info("task:intel:source_metadata")

	if sourceLinkPreviewerInstance == nil || dbPool == nil {
		slog.Warn("task:intel:source_metadata: dependencies not wired, skipping")
		return nil
	}

	var p sourceMetadataPayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}

	sourceID, err := scanUUID(p.SourceID)
	if err != nil {
		return err
	}

	tenantID := parseTenantID(p.TenantID)
	if !tenantID.Valid {
		return fmt.Errorf("task:intel:source_metadata: invalid tenant_id")
	}

	q, err := requireDB()
	if err != nil {
		return err
	}

	src, err := q.GetIntelSource(ctx,

		sourceID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("task:intel:source_metadata: source not found", "source_id", p.SourceID)
			return nil
		}
		return fmt.Errorf("task:intel:source_metadata: get source: %w", err)
	}

	preview, ok := fetchSourcePreview(ctx, sourceLinkPreviewerInstance, tenantID, src)
	if !ok {
		slog.Debug("task:intel:source_metadata: no preview available; skipping", "source_id", p.SourceID)
		return nil
	}

	params := gen.UpdateIntelSourceMetadataParams{

		ID: sourceID,
	}

	if preview.Description != "" && (src.Description == nil || *src.Description == "") {
		desc := preview.Description
		params.Description = &desc
	}

	if src.LogoUrl == nil || *src.LogoUrl == "" {
		if preview.Favicon != "" {
			logo := preview.Favicon
			params.LogoUrl = &logo
		} else if preview.Image != "" {
			logo := preview.Image
			params.LogoUrl = &logo
		}
	}

	if len(preview.Keywords) > 0 && len(src.Keywords) == 0 {
		params.Keywords = preview.Keywords
	}

	if params.Description == nil && params.LogoUrl == nil && len(params.Keywords) == 0 {
		return nil
	}

	if err := q.UpdateIntelSourceMetadata(ctx, params); err != nil {
		return fmt.Errorf("task:intel:source_metadata: update source: %w", err)
	}

	slog.Info("task:intel:source_metadata: updated", "source_id", p.SourceID)
	return nil
}

// fetchSourcePreview tries the source's source_url first, then falls back to
// https://domain. It returns the preview and a bool indicating success.
func fetchSourcePreview(ctx context.Context, fetcher SourceLinkPreviewer, tenantID pgtype.UUID, src gen.IntelSource) (*Preview, bool) {
	if src.SourceUrl != "" {
		preview, err := fetcher.Fetch(ctx, src.SourceUrl, tenantID)
		if err == nil {
			return preview, true
		}
		slog.Debug("task:intel:source_metadata: source_url preview failed; trying domain", "source_id", src.ID.String(), "source_url", src.SourceUrl, "error", err)
	}

	if src.Domain != "" {
		pageURL := "https://" + src.Domain
		preview, err := fetcher.Fetch(ctx, pageURL, tenantID)
		if err == nil {
			return preview, true
		}
		slog.Debug("task:intel:source_metadata: domain preview failed", "source_id", src.ID.String(), "url", pageURL, "error", err)
	}

	return nil, false
}

// EnqueueSourceMetadata builds and enqueues a source metadata task.
func (e *Enqueuer) EnqueueSourceMetadata(sourceID, tenantID string) error {
	payload, err := json.Marshal(sourceMetadataPayload{
		SourceID: sourceID,
	})
	if err != nil {
		return err
	}
	return e.Enqueue(TypeIntelSourceMetadata, payload)
}
