package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/spaces"
)

// SpaceResolver resolves geo and member spaces as comparison references.
type SpaceResolver struct {
	svc *spaces.Service
}

// NewSpaceResolver creates a resolver backed by the spaces service.
func NewSpaceResolver(svc *spaces.Service) *SpaceResolver {
	return &SpaceResolver{svc: svc}
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
		return spaceItemFromGetRow(row), true, nil
	}

	if slug, ok := ref["slug"].(string); ok && slug != "" {
		row, err := r.svc.GetSpaceBySlug(ctx, slug, tenantID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ResolvedItem{}, false, nil
			}
			return ResolvedItem{}, false, fmt.Errorf("fetching space by slug: %w", err)
		}
		return spaceItemFromSlugRow(row), true, nil
	}

	return ResolvedItem{}, false, errors.New("space item_ref missing id or slug")
}

// Search lists public spaces matching the query.
func (r *SpaceResolver) Search(ctx context.Context, query string, filters map[string]any, tenantID pgtype.UUID, limit int32) ([]ResolvedItem, error) {
	if r == nil || r.svc == nil {
		return nil, errors.New("space resolver not initialized")
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	geoCode := ""
	level := ""
	if filters != nil {
		if v, ok := filters["geo_code"].(string); ok {
			geoCode = v
		}
		if v, ok := filters["level"].(string); ok {
			level = v
		}
	}

	viewer := pgtype.UUID{}
	items, err := r.svc.ListSpacesPaginated(ctx, tenantID, viewer, query, geoCode, level, "name", limit, 0)
	if err != nil {
		return nil, fmt.Errorf("listing spaces: %w", err)
	}

	out := make([]ResolvedItem, 0, len(items))
	for _, it := range items {
		out = append(out, spaceItemFromRow(it))
	}
	return out, nil
}

func spaceItemFromRow(row gen.ListSpacesPaginatedRow) ResolvedItem {
	desc := ""
	if row.Description != nil {
		desc = *row.Description
	}
	imageID := ""
	if row.LogoMediaID.Valid {
		imageID = row.LogoMediaID.String()
	} else if row.HeaderImageID.Valid {
		imageID = row.HeaderImageID.String()
	}
	return ResolvedItem{
		ID:          row.ID.String(),
		Title:       row.Name,
		Description: desc,
		ImageURL:    "",
		URL:         "",
		Metadata: map[string]any{
			"slug":         row.Slug,
			"level":        row.Level,
			"geo_code":     deref(row.GeoCode),
			"image_id":     imageID,
			"member_count": row.MemberCount,
		},
	}
}

func spaceItemFromGetRow(row gen.GetSpaceByIDRow) ResolvedItem {
	desc := ""
	if row.Description != nil {
		desc = *row.Description
	}
	imageID := ""
	if row.LogoMediaID.Valid {
		imageID = row.LogoMediaID.String()
	} else if row.HeaderImageID.Valid {
		imageID = row.HeaderImageID.String()
	}
	return ResolvedItem{
		ID:          row.ID.String(),
		Title:       row.Name,
		Description: desc,
		ImageURL:    "",
		URL:         "",
		Metadata: map[string]any{
			"slug":         row.Slug,
			"level":        row.Level,
			"geo_code":     deref(row.GeoCode),
			"image_id":     imageID,
			"member_count": row.MemberCount,
		},
	}
}

func spaceItemFromSlugRow(row gen.GetSpaceBySlugRow) ResolvedItem {
	desc := ""
	if row.Description != nil {
		desc = *row.Description
	}
	imageID := ""
	if row.LogoMediaID.Valid {
		imageID = row.LogoMediaID.String()
	} else if row.HeaderImageID.Valid {
		imageID = row.HeaderImageID.String()
	}
	return ResolvedItem{
		ID:          row.ID.String(),
		Title:       row.Name,
		Description: desc,
		ImageURL:    "",
		URL:         "",
		Metadata: map[string]any{
			"slug":         row.Slug,
			"level":        row.Level,
			"geo_code":     deref(row.GeoCode),
			"image_id":     imageID,
			"member_count": row.MemberCount,
		},
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
