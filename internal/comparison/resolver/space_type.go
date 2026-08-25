package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/spaces"
)

// spaceTypeResolver resolves spaces of a specific space_type slug (e.g. country,
// state, city, university). It is a generalisation of the older per-type
// university resolver.
type spaceTypeResolver struct {
	svc        *spaces.Service
	slug       string
	entityType string
}

// NewSpaceTypeResolver creates a resolver for spaces of the given space type slug.
// The entityType is the item_ref.type used for resolved items.
func NewSpaceTypeResolver(svc *spaces.Service, slug, entityType string) Resolver {
	return &spaceTypeResolver{svc: svc, slug: slug, entityType: entityType}
}

func (r *spaceTypeResolver) typeID(ctx context.Context, tenantID pgtype.UUID) (pgtype.UUID, error) {
	t, err := r.svc.GetSpaceTypeBySlug(ctx, r.slug, tenantID)
	if err != nil {
		if IsNotFound(err) {
			return pgtype.UUID{}, nil
		}
		return pgtype.UUID{}, fmt.Errorf("getting %s space type: %w", r.slug, err)
	}
	return t.ID, nil
}

func (r *spaceTypeResolver) Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error) {
	idStr, _ := ref["id"].(string)
	slug, _ := ref["slug"].(string)

	if idStr == "" && slug == "" {
		return ResolvedItem{}, false, errors.New(r.entityType + " item_ref missing id or slug")
	}

	typeID, err := r.typeID(ctx, tenantID)
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
		row, err := r.svc.GetSpaceByID(ctx, tenantID, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ResolvedItem{}, false, nil
			}
			return ResolvedItem{}, false, fmt.Errorf("fetching %s by id: %w", r.entityType, err)
		}
		if row.SpaceTypeID != typeID {
			return ResolvedItem{}, false, nil
		}
		return makeSpaceResolvedItem(r.entityType, row.ID, row.Name, row.Slug, row.Description), true, nil
	}

	row, err := r.svc.GetSpaceBySlug(ctx, slug, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResolvedItem{}, false, nil
		}
		return ResolvedItem{}, false, fmt.Errorf("fetching %s by slug: %w", r.entityType, err)
	}
	if row.SpaceTypeID != typeID {
		return ResolvedItem{}, false, nil
	}
	return makeSpaceResolvedItem(r.entityType, row.ID, row.Name, row.Slug, row.Description), true, nil
}

func (r *spaceTypeResolver) Search(ctx context.Context, query string, _ map[string]any, tenantID pgtype.UUID, limit int32) ([]ResolvedItem, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	typeID, err := r.typeID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if !typeID.Valid {
		return nil, nil
	}

	items, err := r.svc.ListSpacesByTypePaginated(ctx, tenantID, typeID, query, "", "name", limit, 0)
	if err != nil {
		return nil, fmt.Errorf("searching %s: %w", r.entityType, err)
	}

	out := make([]ResolvedItem, 0, len(items))
	for _, it := range items {
		out = append(out, makeSpaceResolvedItem(r.entityType, it.ID, it.Name, it.Slug, it.Description))
	}
	return out, nil
}
