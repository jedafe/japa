package docs

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/worker"
)

// GenerateDocsPage implements worker.DocsPageGenerator (TASK-004).
// It converts the worker request into a docs Service GenerateDraft call and
// persists an AI-generated draft for HITL review.
func (s *Service) GenerateDocsPage(ctx context.Context, req worker.GenerateDocsPageRequest) error {
	if req.Title == "" {
		return errors.New("docs: title is required")
	}

	audience := req.Audience
	if audience == "" {
		audience = "users"
	}

	if !ValidAudiences[audience] {
		return fmt.Errorf("docs: invalid audience %q", audience)
	}

	prompt := req.Prompt
	if prompt == "" {
		prompt = fmt.Sprintf("Generate documentation for: %s (audience: %s)", req.Title, audience)
	}

	// req.UserID is the requester who queued the generation (the enqueue
	// payload carries it). Stamping it as created_by means the §44
	// self-review rule blocks that requester from approving the draft.
	if uid := (pgtype.UUID{}); req.UserID != "" && uid.Scan(req.UserID) == nil && uid.Valid {
		if auth.FromContext(ctx) == nil {
			ctx = context.WithValue(ctx, auth.UserContextKey, &auth.AuthenticatedUser{UserID: uid})
		}
	}

	_, err := s.GenerateDraft(ctx, GenerateDraftRequest{
		Title:       req.Title,
		Slug:        req.Slug,
		Audience:    audience,
		Prompt:      prompt,
		Category:    req.DocCategory,
		Subcategory: req.DocSubcategory,
	})

	return err
}
