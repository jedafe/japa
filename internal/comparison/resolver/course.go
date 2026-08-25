package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/learning"
)

// courseResolver resolves learning tracks (courses) for comparison.
type courseResolver struct {
	store *learning.Store
}

// NewCourseResolver creates a resolver for courses.
func NewCourseResolver(store *learning.Store) Resolver {
	return &courseResolver{store: store}
}

func (r *courseResolver) Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error) {
	idStr, _ := ref["id"].(string)
	slug, _ := ref["slug"].(string)

	if idStr == "" && slug == "" {
		return ResolvedItem{}, false, errors.New("course item_ref missing id or slug")
	}

	var t *learning.Track
	var err error
	if idStr != "" {
		id, perr := ParseUUID(idStr)
		if perr != nil {
			return ResolvedItem{}, false, perr
		}
		t, err = r.store.GetTrackByID(ctx, tenantID, id)
	} else {
		t, err = r.store.GetTrackBySlug(ctx, tenantID, slug)
	}

	if err != nil {
		if IsNotFound(err) {
			return ResolvedItem{}, false, nil
		}
		return ResolvedItem{}, false, fmt.Errorf("fetching course: %w", err)
	}
	if t == nil || !t.IsPublished {
		return ResolvedItem{}, false, nil
	}
	return trackToResolvedItem(*t), true, nil
}

func (r *courseResolver) Search(ctx context.Context, query string, _ map[string]any, tenantID pgtype.UUID, limit int32) ([]ResolvedItem, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	tracks, err := r.store.SearchPublishedTracks(ctx, tenantID, query, limit)
	if err != nil {
		return nil, fmt.Errorf("searching courses: %w", err)
	}

	out := make([]ResolvedItem, 0, len(tracks))
	for _, t := range tracks {
		out = append(out, trackToResolvedItem(t))
	}
	return out, nil
}

func trackToResolvedItem(t learning.Track) ResolvedItem {
	return ResolvedItem{
		Type:        "course",
		ID:          t.ID.String(),
		Title:       t.Title,
		Description: Excerpt(deref(t.Description), 160),
		Metadata: map[string]any{
			"slug": t.Slug,
		},
	}
}
