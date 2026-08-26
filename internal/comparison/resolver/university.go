package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/spaces"
)

// universityResolver resolves spaces with the "university" space type.
type universityResolver struct {
	svc  *spaces.Service
	deps *scoreDeps
}

// NewUniversityResolver creates a resolver for university spaces.
func NewUniversityResolver(svc *spaces.Service, opts ...Option) Resolver {
	o := newResolverOptions(opts...)
	return &universityResolver{
		svc: svc,
		deps: &scoreDeps{
			queries:  o.queries,
			colStore: o.colStore,
			visaSvc:  o.visaSvc,
		},
	}
}

func (r *universityResolver) Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error) {
	idStr, _ := ref["id"].(string)
	slug, _ := ref["slug"].(string)

	if idStr == "" && slug == "" {
		return ResolvedItem{}, false, errors.New("university item_ref missing id or slug")
	}

	typeID, err := r.universityTypeID(ctx, tenantID)
	if err != nil {
		return ResolvedItem{}, false, err
	}
	if !typeID.Valid {
		return ResolvedItem{}, false, errors.New("no university space type found")
	}

	if idStr != "" {
		id, err := ParseUUID(idStr)
		if err != nil {
			return ResolvedItem{}, false, err
		}
		row, err := r.svc.GetSpaceByID(ctx, tenantID, id)
		if err != nil {
			if IsNotFound(err) {
				return ResolvedItem{}, false, nil
			}
			return ResolvedItem{}, false, fmt.Errorf("fetching university by id: %w", err)
		}
		if row.SpaceTypeID != typeID {
			return ResolvedItem{}, false, nil
		}
		if base, ok := spaceBaseFromRow(row); ok {
			return resolveSpaceBase(ctx, tenantID, r.deps, "university", base), true, nil
		}
		return ResolvedItem{}, false, nil
	}

	row, err := r.svc.GetSpaceBySlug(ctx, slug, tenantID)
	if err != nil {
		if IsNotFound(err) {
			return ResolvedItem{}, false, nil
		}
		return ResolvedItem{}, false, fmt.Errorf("fetching university by slug: %w", err)
	}
	if row.SpaceTypeID != typeID {
		return ResolvedItem{}, false, nil
	}
	if base, ok := spaceBaseFromRow(row); ok {
		return resolveSpaceBase(ctx, tenantID, r.deps, "university", base), true, nil
	}
	return ResolvedItem{}, false, nil
}

func (r *universityResolver) Search(ctx context.Context, query string, _ map[string]any, tenantID pgtype.UUID, limit int32) ([]ResolvedItem, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	typeID, err := r.universityTypeID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if !typeID.Valid {
		return nil, nil
	}

	items, err := r.svc.ListSpacesByTypePaginated(ctx, tenantID, typeID, query, "", "name", limit, 0)
	if err != nil {
		return nil, fmt.Errorf("searching universities: %w", err)
	}

	out := make([]ResolvedItem, 0, len(items))
	for _, it := range items {
		if base, ok := spaceBaseFromRow(it); ok {
			out = append(out, resolveSpaceBase(ctx, tenantID, r.deps, "university", base))
		}
	}
	return out, nil
}

func (r *universityResolver) universityTypeID(ctx context.Context, tenantID pgtype.UUID) (pgtype.UUID, error) {
	t, err := r.svc.GetSpaceTypeBySlug(ctx, "university", tenantID)
	if err != nil {
		if IsNotFound(err) {
			return pgtype.UUID{}, nil
		}
		return pgtype.UUID{}, fmt.Errorf("getting university space type: %w", err)
	}
	return t.ID, nil
}

func makeSpaceResolvedItem(entityType string, id pgtype.UUID, name, slug string, description *string, geoCode *string, level string, logoMediaID, headerImageID pgtype.UUID, memberCount int32) ResolvedItem {
	base := spaceBase{
		ID: id, Name: name, Slug: slug, Description: description,
		GeoCode: geoCode, Level: level,
		LogoMediaID: logoMediaID, HeaderImageID: headerImageID,
		MemberCount: memberCount,
	}
	return resolveSpaceBase(context.Background(), pgtype.UUID{}, nil, entityType, base)
}
