package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/spaces"
)

// SpaceResolver resolves geo and member spaces as comparison references.
type SpaceResolver struct {
	svc  *spaces.Service
	deps *scoreDeps
}

// NewSpaceResolver creates a resolver backed by the spaces service.
func NewSpaceResolver(svc *spaces.Service, opts ...Option) *SpaceResolver {
	o := newResolverOptions(opts...)
	return &SpaceResolver{
		svc: svc,
		deps: &scoreDeps{
			queries:  o.queries,
			colStore: o.colStore,
			visaSvc:  o.visaSvc,
		},
	}
}

// Resolve fetches a space by id or slug.
func (r *SpaceResolver) Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error) {
	if r == nil || r.svc == nil {
		return ResolvedItem{}, false, errors.New("space resolver not initialized")
	}

	if idStr, ok := ref["id"].(string); ok && idStr != "" {
		id, err := ParseUUID(idStr)
		if err != nil {
			return ResolvedItem{}, false, err
		}
		row, err := r.svc.GetSpaceByID(ctx, tenantID, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ResolvedItem{}, false, nil
			}
			return ResolvedItem{}, false, fmt.Errorf("fetching space by id: %w", err)
		}
		if base, ok := spaceBaseFromRow(row); ok {
			return resolveSpaceBase(ctx, tenantID, r.deps, "space", base), true, nil
		}
		return ResolvedItem{}, false, nil
	}

	if slug, ok := ref["slug"].(string); ok && slug != "" {
		row, err := r.svc.GetSpaceBySlug(ctx, slug, tenantID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ResolvedItem{}, false, nil
			}
			return ResolvedItem{}, false, fmt.Errorf("fetching space by slug: %w", err)
		}
		if base, ok := spaceBaseFromRow(row); ok {
			return resolveSpaceBase(ctx, tenantID, r.deps, "space", base), true, nil
		}
		return ResolvedItem{}, false, nil
	}

	return ResolvedItem{}, false, errors.New("space item_ref missing id or slug")
}

func spaceFilters(filters map[string]any) (geoCode, level, region, subregion string) {
	if filters != nil {
		if v, ok := filters["geo_code"].(string); ok {
			geoCode = v
		}
		if v, ok := filters["level"].(string); ok {
			level = v
		}
		if v, ok := filters["region"].(string); ok {
			region = v
		}
		if v, ok := filters["subregion"].(string); ok {
			subregion = v
		}
	}
	return
}

// Search lists public spaces matching the query.
func (r *SpaceResolver) Search(ctx context.Context, query string, filters map[string]any, tenantID pgtype.UUID, limit int32, offset int32) ([]ResolvedItem, error) {
	if r == nil || r.svc == nil {
		return nil, errors.New("space resolver not initialized")
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}
	if offset < 0 {
		offset = 0
	}

	geoCode, level, region, subregion := spaceFilters(filters)

	viewer := pgtype.UUID{}
	items, err := r.svc.ListSpacesPaginated(ctx, tenantID, viewer, query, geoCode, level, region, subregion, "name", limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing spaces: %w", err)
	}

	out := make([]ResolvedItem, 0, len(items))
	for _, it := range items {
		if base, ok := spaceBaseFromRow(it); ok {
			out = append(out, resolveSpaceBase(ctx, tenantID, r.deps, "space", base))
		}
	}
	return out, nil
}

// Count returns the total number of public spaces matching the query.
func (r *SpaceResolver) Count(ctx context.Context, query string, filters map[string]any, tenantID pgtype.UUID) (int64, error) {
	if r == nil || r.svc == nil {
		return -1, errors.New("space resolver not initialized")
	}
	geoCode, level, region, subregion := spaceFilters(filters)
	viewer := pgtype.UUID{}
	return r.svc.CountSpaces(ctx, tenantID, viewer, query, geoCode, level, region, subregion)
}

func spaceMetadata(id pgtype.UUID, slug, level string, geoCode *string, imageID string, memberCount int32) map[string]any {
	base := spaceBase{ID: id, Slug: slug, Level: level, GeoCode: geoCode, MemberCount: memberCount}
	meta := map[string]any{
		"geo_space_id": id.String(),
		"slug":         slug,
		"level":        level,
		"geo_code":     deref(geoCode),
		"image_id":     imageID,
		"member_count": memberCount,
		"score_inputs": spaceScoreInputs(context.Background(), pgtype.UUID{}, nil, "space", base),
	}
	if level == "country" && geoCode != nil && *geoCode != "" {
		meta["country"] = *geoCode
	}
	return meta
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
