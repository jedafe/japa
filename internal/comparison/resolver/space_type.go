package resolver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/spaces"
)

// spaceTypeResolver resolves spaces of a specific space_type slug (e.g. country,
// state, city, university). It is a generalisation of the older per-type
// university resolver.
type spaceTypeResolver struct {
	svc        *spaces.Service
	slug       string
	entityType string
	deps       *scoreDeps
}

// NewSpaceTypeResolver creates a resolver for spaces of the given space type slug.
// The entityType is the item_ref.type used for resolved items.
func NewSpaceTypeResolver(svc *spaces.Service, slug, entityType string, opts ...Option) Resolver {
	o := newResolverOptions(opts...)

	return &spaceTypeResolver{
		svc:        svc,
		slug:       slug,
		entityType: entityType,
		deps: &scoreDeps{
			queries:  o.queries,
			colStore: o.colStore,
			visaSvc:  o.visaSvc,
		},
	}
}

func (r *spaceTypeResolver) typeID(ctx context.Context) (pgtype.UUID, error) {
	t, err := r.svc.GetSpaceTypeBySlug(ctx, r.slug)
	if err != nil {
		if IsNotFound(err) {
			return pgtype.UUID{}, nil
		}

		return pgtype.UUID{}, fmt.Errorf("getting %s space type: %w", r.slug, err)
	}

	return t.ID, nil
}

func (r *spaceTypeResolver) Resolve(ctx context.Context, ref map[string]any) (ResolvedItem, bool, error) {
	idStr, _ := ref["id"].(string)
	slug, _ := ref["slug"].(string)

	if idStr == "" && slug == "" {
		return ResolvedItem{}, false, errors.New(r.entityType + " item_ref missing id or slug")
	}

	typeID, err := r.typeID(ctx)
	if err != nil {
		return ResolvedItem{}, false, err
	}

	if !typeID.Valid {
		return ResolvedItem{}, false, errors.New("no " + r.entityType + " space type found")
	}

	if idStr != "" {
		id, err := ParseUUID(idStr)
		if err != nil {
			return ResolvedItem{}, false, err
		}

		row, err := r.svc.GetSpaceByID(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ResolvedItem{}, false, nil
			}

			return ResolvedItem{}, false, fmt.Errorf("fetching %s by id: %w", r.entityType, err)
		}

		if row.SpaceTypeID != typeID {
			return ResolvedItem{}, false, nil
		}

		if base, ok := spaceBaseFromRow(row); ok {
			return resolveSpaceBase(ctx, r.deps, r.entityType, base), true, nil
		}

		return ResolvedItem{}, false, nil
	}

	row, err := r.svc.GetSpaceBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResolvedItem{}, false, nil
		}

		return ResolvedItem{}, false, fmt.Errorf("fetching %s by slug: %w", r.entityType, err)
	}

	if row.SpaceTypeID != typeID {
		return ResolvedItem{}, false, nil
	}

	if base, ok := spaceBaseFromRow(row); ok {
		return resolveSpaceBase(ctx, r.deps, r.entityType, base), true, nil
	}

	return ResolvedItem{}, false, nil
}

func (r *spaceTypeResolver) Search(ctx context.Context, query string, filters map[string]any, limit, offset int32) ([]ResolvedItem, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	if offset < 0 {
		offset = 0
	}

	typeID, err := r.typeID(ctx)
	if err != nil {
		return nil, err
	}

	if !typeID.Valid {
		return nil, nil
	}

	search := strings.TrimSpace(query)

	var items any

	switch r.entityType {
	case "country":
		region := parseStringFilter(filters, "region")
		subregion := parseStringFilter(filters, "subregion")

		rows, err := r.svc.ListSpacesByTypePaginated(ctx, typeID, search, "", region, subregion, "name", limit, offset)
		if err != nil {
			return nil, fmt.Errorf("searching %s: %w", r.entityType, err)
		}

		items = rows
	case "state":
		countryID := parseUUIDFilter(filters, "country_id")
		if !countryID.Valid {
			rows, err := r.svc.ListSpacesByTypePaginated(ctx, typeID, search, "", "", "", "name", limit, offset)
			if err != nil {
				return nil, fmt.Errorf("searching %s: %w", r.entityType, err)
			}

			items = rows
		} else {
			rows, err := r.svc.ListSpacesByAncestorAndType(ctx, typeID, countryID, search, "name", limit, offset)
			if err != nil {
				return nil, fmt.Errorf("searching %s: %w", r.entityType, err)
			}

			items = rows
		}
	case "city":
		stateID := parseUUIDFilter(filters, "state_id")
		if !stateID.Valid {
			rows, err := r.svc.ListSpacesByTypePaginated(ctx, typeID, search, "", "", "", "name", limit, offset)
			if err != nil {
				return nil, fmt.Errorf("searching %s: %w", r.entityType, err)
			}

			items = rows
		} else {
			rows, err := r.svc.ListSpacesByAncestorAndType(ctx, typeID, stateID, search, "name", limit, offset)
			if err != nil {
				return nil, fmt.Errorf("searching %s: %w", r.entityType, err)
			}

			items = rows
		}
	default:
		rows, err := r.svc.ListSpacesByTypePaginated(ctx, typeID, search, "", "", "", "name", limit, offset)
		if err != nil {
			return nil, fmt.Errorf("searching %s: %w", r.entityType, err)
		}

		items = rows
	}

	out := make([]ResolvedItem, 0)
	switch rows := items.(type) {
	case []gen.ListSpacesByTypePaginatedRow:
		out = make([]ResolvedItem, 0, len(rows))

		for _, it := range rows {
			if base, ok := spaceBaseFromRow(it); ok {
				out = append(out, resolveSpaceBase(ctx, r.deps, r.entityType, base))
			}
		}
	case []gen.ListSpacesByAncestorAndTypeRow:
		out = make([]ResolvedItem, 0, len(rows))

		for _, it := range rows {
			if base, ok := spaceBaseFromRow(it); ok {
				out = append(out, resolveSpaceBase(ctx, r.deps, r.entityType, base))
			}
		}
	}

	return out, nil
}

// Count returns the total number of spaces of this type matching the query.
func (r *spaceTypeResolver) Count(ctx context.Context, query string, filters map[string]any) (int64, error) {
	typeID, err := r.typeID(ctx)
	if err != nil {
		return -1, err
	}

	if !typeID.Valid {
		return 0, nil
	}

	search := strings.TrimSpace(query)

	switch r.entityType {
	case "country":
		region := parseStringFilter(filters, "region")
		subregion := parseStringFilter(filters, "subregion")

		return r.svc.CountSpacesByType(ctx, typeID, search, "", region, subregion)
	case "state":
		countryID := parseUUIDFilter(filters, "country_id")
		if !countryID.Valid {
			return r.svc.CountSpacesByType(ctx, typeID, search, "", "", "")
		}

		return r.svc.CountSpacesByAncestorAndType(ctx, typeID, countryID, search)
	case "city":
		stateID := parseUUIDFilter(filters, "state_id")
		if !stateID.Valid {
			return r.svc.CountSpacesByType(ctx, typeID, search, "", "", "")
		}

		return r.svc.CountSpacesByAncestorAndType(ctx, typeID, stateID, search)
	default:
		return r.svc.CountSpacesByType(ctx, typeID, search, "", "", "")
	}
}
