package docs

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/worker"
)

// GenerateDocsPage implements worker.DocsPageGenerator (TASK-004).
// It converts the worker request into a docs Service GenerateDraft call and
// persists an AI-generated draft for HITL review.
func (s *Service) GenerateDocsPage(ctx context.Context, req worker.GenerateDocsPageRequest) error {
	var tenantID pgtype.UUID
	if err := tenantID.Scan(req.TenantID); err != nil {
		return fmt.Errorf("docs: invalid the id %q: %w", req.TenantID, err)
	}

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

	_, err := s.GenerateDraft(ctx, tenantID, GenerateDraftRequest{
		Title:    req.Title,
		Slug:     req.Slug,
		Audience: audience,
		Prompt:   prompt,
	})

	return err
}
