package server

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/publicapi"
	"github.com/jedafe/japa/internal/tags"
)

// publicDraftAdapter satisfies publicapi.ContentDraftService over the
// session-side content pipeline — CreateContent already enforces status
// validation, space posting permissions (spaceChecker), org-post rules,
// content hashing, and webhook emission, so the adapter only constrains the
// input surface to drafts.
//
// Ref: spec 2026-09-24-agent-data-api-phase3 (Phase 3 content drafts).
type publicDraftAdapter struct {
	content *content.Service
	tags    *tags.Service
}

func (a *publicDraftAdapter) CreateDraft(ctx context.Context, ownerID pgtype.UUID, in publicapi.CreateDraftInput) (publicapi.PublicDraft, error) {
	var spaceID pgtype.UUID
	if in.SpaceID != "" {
		if err := spaceID.Scan(in.SpaceID); err != nil {
			return publicapi.PublicDraft{}, fmt.Errorf("%w: space_id must be a UUID", errpkg.ErrInvalid)
		}
	}

	// Derive a plain-text body from body_html when the caller sends only
	// HTML — same derivation the session handler performs, so the
	// content_hash dedupe column never sees an empty body.
	body := in.Body
	if body == "" && in.BodyHTML != "" {
		body = content.SanitizeText(in.BodyHTML)
	}

	row, err := a.content.CreateContent(ctx, content.CreateContentInput{
		AuthorID:      ownerID,
		ContentType:   in.ContentType,
		Title:         in.Title,
		Slug:          in.Slug,
		Body:          content.SanitizeText(body),
		BodyHtml:      content.SanitizeHTML(in.BodyHTML),
		FieldValues:   in.FieldValues,
		SpaceID:       spaceID,
		Status:        content.StatusDraft,
		Visibility:    content.VisibilityPublic,
		SeoIndexable:  true,
		JourneyStage:  in.JourneyStage,
		VisaRoute:     in.VisaRoute,
		SourceCountry: in.SourceCountry,
		TargetCountry: in.TargetCountry,
	})
	if err != nil {
		return publicapi.PublicDraft{}, err
	}

	// Optional tag slugs — get-or-create + attach, mirroring
	// POST /content/{id}/tags (§14 composer flow). Tag failures degrade to
	// an untagged draft rather than failing the create — the row already
	// exists at this point.
	if len(in.Tags) > 0 && a.tags != nil {
		if slugMap, terr := a.tags.GetOrCreateTagSlugs(ctx, in.Tags); terr == nil {
			for _, slug := range in.Tags {
				if id, ok := slugMap[slug]; ok {
					_ = a.content.AddContentTag(ctx, row.ID, id)
				}
			}
		}
	}

	return publicapi.PublicDraft{
		ID:          row.ID.String(),
		ContentType: row.ContentType,
		Title:       row.Title,
		Slug:        row.Slug,
		Status:      row.Status,
		CreatedAt:   row.CreatedAt.Time.UTC().Format(time.RFC3339),
	}, nil
}

var _ publicapi.ContentDraftService = (*publicDraftAdapter)(nil)
