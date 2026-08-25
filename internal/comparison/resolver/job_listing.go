package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/content/visibility"
	"github.com/jedafe/japa/internal/db/gen"
)

// JobListingResolver resolves public job listings (contents with
// content_type='job_listing') into JCE comparison references.
type JobListingResolver struct {
	svc *content.Service
}

// NewJobListingResolver creates a resolver backed by the content service.
func NewJobListingResolver(svc *content.Service) *JobListingResolver {
	return &JobListingResolver{svc: svc}
}

// Resolve fetches a job listing by id or slug.
func (r *JobListingResolver) Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error) {
	if r == nil || r.svc == nil {
		return ResolvedItem{}, false, errors.New("job_listing resolver not initialized")
	}

	if idStr, ok := ref["id"].(string); ok && idStr != "" {
		id, err := ParseUUID(idStr)
		if err != nil {
			return ResolvedItem{}, false, err
		}
		row, err := r.svc.GetContent(ctx, id, tenantID)
		if err != nil {
			if IsNotFound(err) {
				return ResolvedItem{}, false, nil
			}
			return ResolvedItem{}, false, fmt.Errorf("fetching job listing by id: %w", err)
		}
		if !isJobListing(row.ContentType, row.Status, row.Visibility) {
			return ResolvedItem{}, false, nil
		}
		return jobListingFromIDRow(row), true, nil
	}

	if slug, ok := ref["slug"].(string); ok && slug != "" {
		row, err := r.svc.GetContentBySlug(ctx, slug, tenantID)
		if err != nil {
			if IsNotFound(err) {
				return ResolvedItem{}, false, nil
			}
			return ResolvedItem{}, false, fmt.Errorf("fetching job listing by slug: %w", err)
		}
		if !isJobListing(row.ContentType, row.Status, row.Visibility) {
			return ResolvedItem{}, false, nil
		}
		return jobListingFromSlugRow(row), true, nil
	}

	return ResolvedItem{}, false, errors.New("job_listing item_ref missing id or slug")
}

// Search lists published, public job listings matching the query.
func (r *JobListingResolver) Search(ctx context.Context, query string, filters map[string]any, tenantID pgtype.UUID, limit int32) ([]ResolvedItem, error) {
	if r == nil || r.svc == nil {
		return nil, errors.New("job_listing resolver not initialized")
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	emptyCursor := pgtype.Timestamptz{}
	items, err := r.svc.ListContentsByType(ctx, tenantID, "job_listing", emptyCursor, limit, visibility.VisibilityFilter{})
	if err != nil {
		return nil, fmt.Errorf("listing job listings: %w", err)
	}

	out := make([]ResolvedItem, 0, len(items))
	for _, it := range items {
		if query != "" && !containsQuery(it.Title, it.Slug, it.Body, query) {
			continue
		}
		out = append(out, ResolvedItem{
			Type:        "job_listing",
			ID:          it.ID.String(),
			Title:       it.Title,
			Description: Excerpt(it.Body, 160),
			Metadata: map[string]any{
				"content_type": it.ContentType,
				"slug":         it.Slug,
				"status":       it.Status,
				"visibility":   it.Visibility,
			},
		})
		if int32(len(out)) >= limit {
			break
		}
	}
	return out, nil
}

func isJobListing(contentType, status, visibility string) bool {
	return contentType == "job_listing" && status == "published" && visibility == "public"
}

func jobListingFromIDRow(row gen.GetContentByIDRow) ResolvedItem {
	return ResolvedItem{
		Type:        "job_listing",
		ID:          row.ID.String(),
		Title:       row.Title,
		Description: Excerpt(row.Body, 160),
		Metadata: map[string]any{
			"content_type": row.ContentType,
			"slug":         row.Slug,
			"status":       row.Status,
			"visibility":   row.Visibility,
		},
	}
}

func jobListingFromSlugRow(row gen.GetContentBySlugRow) ResolvedItem {
	return ResolvedItem{
		Type:        "job_listing",
		ID:          row.ID.String(),
		Title:       row.Title,
		Description: Excerpt(row.Body, 160),
		Metadata: map[string]any{
			"content_type": row.ContentType,
			"slug":         row.Slug,
			"status":       row.Status,
			"visibility":   row.Visibility,
		},
	}
}
