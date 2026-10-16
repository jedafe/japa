package content

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/literals"
)

// ResolveContent resolves template literals on a full *gen.Content row.
// It is a thin wrapper around ResolveContentFields for callers that already
// have a concrete Content model.
func (s *Service) ResolveContent(ctx context.Context, c *gen.Content, rc literals.RenderContext) error {
	return s.ResolveContentFields(ctx, c, rc)
}

// ResolveContentFields resolves {{...}} placeholders in the title, body,
// body_html, meta_title, and meta_description fields of any content-like
// struct or slice of structs. It works with the various sqlc-generated
// row types (GetContentBySlugRow, ListContentsRow, etc.) by using
// reflection to find and mutate those fields by name.
func (s *Service) ResolveContentFields(ctx context.Context, v any, rc literals.RenderContext) error {
	if v == nil {
		return nil
	}

	tokens, err := s.loadTemplateLiterals(ctx)
	if err != nil {
		return fmt.Errorf("loading template literals: %w", err)
	}

	return literals.ResolveFields(ctx, v, rc, tokens)
}

func (s *Service) loadTemplateLiterals(ctx context.Context) ([]literals.TemplateLiteral, error) {
	if s.catalog != nil {
		return s.catalog.GetActive(ctx)
	}

	active := true

	rows, err := s.read.ListTemplateLiterals(ctx, gen.ListTemplateLiteralsParams{
		Search:     "",
		IsActive:   &active,
		Kind:       "",
		Sort:       "",
		Dir:        "",
		PageOffset: 0,
		PageLimit:  literals.CatalogMaxLimit,
	})
	if err != nil {
		return nil, err
	}

	out := make([]literals.TemplateLiteral, len(rows))
	for i, r := range rows {
		out[i] = literals.TemplateLiteral{
			ID: r.ID.String(),

			Key:         r.Key,
			Kind:        r.Kind,
			Value:       r.Value,
			Fallback:    r.Fallback,
			Label:       r.Label,
			Description: r.Description,
			Category:    r.Category,
			IsSystem:    r.IsSystem,
			IsActive:    r.IsActive,
		}
	}

	return out, nil
}

// buildRenderContext constructs a literals.RenderContext for a content request.
// Geo fields come from the ADR-064 VisitorContext (profile origin_country /
// destination_country custom fields, jc_journey_token for guests, GeoIP
// fallback) and the GeoIP middleware context (detected country name/code).
func buildRenderContext(r *http.Request, platformName string) literals.RenderContext {
	return literals.NewRequestContext(r, platformName)
}
