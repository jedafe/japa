package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/visaroutes"
)

// visaRouteResolver resolves visa routes.
type visaRouteResolver struct {
	svc *visaroutes.Service
}

// NewVisaRouteResolver creates a resolver for visa routes.
func NewVisaRouteResolver(svc *visaroutes.Service) Resolver {
	return &visaRouteResolver{svc: svc}
}

func (r *visaRouteResolver) Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error) {
	if idStr, ok := ref["id"].(string); ok && idStr != "" {
		id, err := ParseUUID(idStr)
		if err != nil {
			return ResolvedItem{}, false, err
		}
		route, err := r.svc.GetByID(ctx, id, tenantID)
		if err != nil {
			if IsNotFound(err) {
				return ResolvedItem{}, false, nil
			}
			return ResolvedItem{}, false, fmt.Errorf("fetching visa route by id: %w", err)
		}
		return routeToResolvedItem(route), true, nil
	}

	if slug, ok := ref["slug"].(string); ok && slug != "" {
		route, err := r.svc.GetBySlug(ctx, slug, tenantID)
		if err != nil {
			if IsNotFound(err) {
				return ResolvedItem{}, false, nil
			}
			return ResolvedItem{}, false, fmt.Errorf("fetching visa route by slug: %w", err)
		}
		return routeToResolvedItem(route), true, nil
	}

	return ResolvedItem{}, false, errors.New("visa_route item_ref missing id or slug")
}

func (r *visaRouteResolver) Search(ctx context.Context, query string, filters map[string]any, tenantID pgtype.UUID, limit int32) ([]ResolvedItem, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	var category string
	if v, ok := filters["visa_category"].(string); ok {
		category = v
	}

	routes, _, err := r.svc.AdminList(ctx, tenantID, pgtype.UUID{}, "published", category, query, "", "", boolPtr(true), 1, limit)
	if err != nil {
		return nil, fmt.Errorf("searching visa routes: %w", err)
	}

	out := make([]ResolvedItem, 0, len(routes))
	for _, route := range routes {
		out = append(out, routeToResolvedItem(route))
	}
	return out, nil
}

func routeToResolvedItem(route visaroutes.RouteDTO) ResolvedItem {
	desc := route.EligibilitySummary
	if route.DestinationSpaceName != "" {
		if desc != "" {
			desc += " · "
		}
		desc += route.DestinationSpaceName
	}
	meta := map[string]any{
		"slug":            route.Slug,
		"visa_category":   route.VisaCategory,
		"destination":     route.DestinationSpaceName,
		"destination_geo": route.DestinationSpaceCode,
	}
	if route.DestinationSpaceID != "" {
		if id, err := ParseUUID(route.DestinationSpaceID); err == nil && id.Valid {
			meta["geo_space_id"] = id.String()
		}
	}
	if route.DestinationSpaceCode != "" {
		meta["country"] = route.DestinationSpaceCode
	}
	if route.VisaCategory != "" {
		meta["tags"] = []string{route.VisaCategory}
	}
	return ResolvedItem{
		Type:        "visa_route",
		ID:          route.ID,
		Title:       route.Name,
		Description: Excerpt(desc, 160),
		Metadata:    meta,
	}
}

func boolPtr(b bool) *bool { return &b }
