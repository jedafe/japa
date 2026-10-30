package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
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

func (r *universityResolver) Resolve(ctx context.Context, ref map[string]any) (ResolvedItem, bool, error) {
	idStr, _ := ref["id"].(string)
	slug, _ := ref["slug"].(string)

	if idStr == "" && slug == "" {
		return ResolvedItem{}, false, errors.New("university item_ref missing id or slug")
	}

	typeID, err := r.universityTypeID(ctx)
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

		row, err := r.svc.GetSpaceByID(ctx, id)
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
			return resolveSpaceBase(ctx, r.deps, "university", base, depsFieldValues(ctx, r.deps, "university", base.ID)), true, nil
		}

		return ResolvedItem{}, false, nil
	}

	row, err := r.svc.GetSpaceBySlug(ctx, slug)
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
		return resolveSpaceBase(ctx, r.deps, "university", base, depsFieldValues(ctx, r.deps, "university", base.ID)), true, nil
	}

	return ResolvedItem{}, false, nil
}

func (r *universityResolver) Search(ctx context.Context, query string, filters map[string]any, limit, offset int32) ([]ResolvedItem, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	if offset < 0 {
		offset = 0
	}

	typeID, err := r.universityTypeID(ctx)
	if err != nil {
		return nil, err
	}

	if !typeID.Valid {
		return nil, nil
	}

	// Try ancestors from the deepest selected geo scope upward. Universities may
	// be parented to a city, a state, or only a country, so fall back if the
	// direct ancestor search returns no results (Section 39).
	var items any

	found := false

	for _, key := range []string{"city_id", "state_id", "country_id"} {
		ancestorID := parseUUIDFilter(filters, key)
		if !ancestorID.Valid {
			continue
		}

		rows, err := r.svc.ListSpacesByAncestorAndType(ctx, typeID, ancestorID, query, spaceSortArg(filters), limit, offset)
		if err != nil {
			return nil, fmt.Errorf("searching universities: %w", err)
		}

		if len(rows) > 0 {
			items = rows
			found = true

			break
		}
	}

	if !found {
		rows, err := r.svc.ListSpacesByTypePaginated(ctx, typeID, query, "", "", "", "", false, spaceSortArg(filters), limit, offset)
		if err != nil {
			return nil, fmt.Errorf("searching universities: %w", err)
		}

		items = rows
	}

	var bases []spaceBase

	switch rows := items.(type) {
	case []gen.ListSpacesByTypePaginatedRow:
		bases = make([]spaceBase, 0, len(rows))

		for _, it := range rows {
			if base, ok := spaceBaseFromRow(it); ok {
				bases = append(bases, base)
			}
		}
	case []gen.ListSpacesByAncestorAndTypeRow:
		bases = make([]spaceBase, 0, len(rows))

		for _, it := range rows {
			if base, ok := spaceBaseFromRow(it); ok {
				bases = append(bases, base)
			}
		}
	}

	// One batched cfv read for the page — no per-row queries (§80).
	cfvBySpace := batchedSpaceFieldValues(ctx, r.deps, "university", bases)

	out := make([]ResolvedItem, 0, len(bases))
	for _, base := range bases {
		out = append(out, resolveSpaceBase(ctx, r.deps, "university", base, cfvBySpace[base.ID]))
	}

	return out, nil
}

// Count returns the total number of universities matching the query.
func (r *universityResolver) Count(ctx context.Context, query string, filters map[string]any) (int64, error) {
	typeID, err := r.universityTypeID(ctx)
	if err != nil {
		return -1, err
	}

	if !typeID.Valid {
		return 0, nil
	}

	// Try ancestors from the deepest selected geo scope upward and return the
	// first non-empty count; fall back to the global count.
	for _, key := range []string{"city_id", "state_id", "country_id"} {
		ancestorID := parseUUIDFilter(filters, key)
		if !ancestorID.Valid {
			continue
		}

		total, err := r.svc.CountSpacesByAncestorAndType(ctx, typeID, ancestorID, query)
		if err != nil {
			return -1, fmt.Errorf("counting universities: %w", err)
		}

		if total > 0 {
			return total, nil
		}
	}

	return r.svc.CountSpacesByType(ctx, typeID, query, "", "", "", "", false)
}

func (r *universityResolver) universityTypeID(ctx context.Context) (pgtype.UUID, error) {
	t, err := r.svc.GetSpaceTypeBySlug(ctx, "university")
	if err != nil {
		if IsNotFound(err) {
			return pgtype.UUID{}, nil
		}

		return pgtype.UUID{}, fmt.Errorf("getting university space type: %w", err)
	}

	return t.ID, nil
}

func makeSpaceResolvedItem(entityType string, id pgtype.UUID, name, slug string, description, geoCode *string, level string, logoMediaID, headerImageID pgtype.UUID, memberCount int32) ResolvedItem {
	base := spaceBase{
		ID: id, Name: name, Slug: slug, Description: description,
		GeoCode: geoCode, Level: level,
		LogoMediaID: logoMediaID, HeaderImageID: headerImageID,
		MemberCount: memberCount,
	}

	return resolveSpaceBase(context.Background(), nil, entityType, base, nil)
}
