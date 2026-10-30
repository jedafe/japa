package docs

import (
	"context"
	"errors"
	"fmt"

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
