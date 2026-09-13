// Package blog implements the Blog Extension system (Section 24).
// // Blog series for multi-part posts, content review workflow for editorial
// review, featured/curated posts, and RSS feed generation. Tags-only model
// (no categories) with granular permission nodes through dynamic RBAC.
package blog

import (
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Series statuses.
const (
	SeriesStatusActive   = "active"
	SeriesStatusArchived = "archived"
)

// Review statuses.
const (
	ReviewStatusPending          = "pending"
	ReviewStatusApproved         = "approved"
	ReviewStatusChangesRequested = "changes_requested"
	ReviewStatusRejected         = "rejected"
)

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// BlogListFilter carries the §24-mandated GET /blog query filters. All
// fields are optional — a zero value (pgtype.UUID{} or nil) means "no
// filter on this field". Sort defaults to "latest" when empty.
// // BUG-011: Section 24 mandates GET /api/v1/blog accept ?space_id, ?tag
// (tag slug), ?series_id, ?sort=latest|top|featured, and ?author_id. The
// sort=featured case is handled by the separate /blog/featured endpoint,
// so callers should map sort=featured to that endpoint before reaching
// the listing query.
// //nolint:revive // exported: stutter naming is intentional
type BlogListFilter struct {
	TenantID pgtype.UUID
	SpaceID  pgtype.UUID // zero → no filter
	AuthorID pgtype.UUID // zero → no filter
	SeriesID pgtype.UUID // zero → no filter
	Tag      *string     // nil → no filter; tag slug
	Sort     string      // "latest" (default) | "top"
}

// Series is the public DTO for blog series list/detail endpoints.
// It mirrors the underlying blog_series row plus a computed cover_image_url
// built from the linked media storage bucket/path (Section 34).
type Series struct {
	ID            pgtype.UUID        `json:"id"`
	TenantID      pgtype.UUID        
	Title         string             `json:"title"`
	Slug          string             `json:"slug"`
	Description   *string            `json:"description"`
	CoverImageID  pgtype.UUID        `json:"cover_image_id"`
	CoverImageURL string             `json:"cover_image_url"`
	AuthorUserID  pgtype.UUID        `json:"author_user_id"`
	Status        string             `json:"status"`
	CreatedAt     pgtype.Timestamptz `json:"created_at"`
	UpdatedAt     pgtype.Timestamptz `json:"updated_at"`
	DeletedAt     pgtype.Timestamptz `json:"deleted_at"`
}

// seriesFromRow builds a public Series DTO from a sqlc row that carries
// cover_storage_bucket and cover_storage_path. It uses the package-level
// mediaURL helper so the public API returns a ready-to-use cover_image_url.
func seriesFromRow(row any) Series {
	switch r := row.(type) {
	case gen.GetBlogSeriesBySlugRow:
		return Series{
			ID: r.ID,

			Title:         r.Title,
			Slug:          r.Slug,
			Description:   r.Description,
			CoverImageID:  r.CoverImageID,
			CoverImageURL: coverImageURL(r.CoverStorageBucket, r.CoverStoragePath),
			AuthorUserID:  r.AuthorUserID,
			Status:        r.Status,
			CreatedAt:     r.CreatedAt,
			UpdatedAt:     r.UpdatedAt,
			DeletedAt:     r.DeletedAt,
		}
	case gen.ListBlogSeriesByTenantRow:
		return Series{
			ID: r.ID,

			Title:         r.Title,
			Slug:          r.Slug,
			Description:   r.Description,
			CoverImageID:  r.CoverImageID,
			CoverImageURL: coverImageURL(r.CoverStorageBucket, r.CoverStoragePath),
			AuthorUserID:  r.AuthorUserID,
			Status:        r.Status,
			CreatedAt:     r.CreatedAt,
			UpdatedAt:     r.UpdatedAt,
			DeletedAt:     r.DeletedAt,
		}
	case gen.ListBlogSeriesByAuthorRow:
		return Series{
			ID: r.ID,

			Title:         r.Title,
			Slug:          r.Slug,
			Description:   r.Description,
			CoverImageID:  r.CoverImageID,
			CoverImageURL: coverImageURL(r.CoverStorageBucket, r.CoverStoragePath),
			AuthorUserID:  r.AuthorUserID,
			Status:        r.Status,
			CreatedAt:     r.CreatedAt,
			UpdatedAt:     r.UpdatedAt,
			DeletedAt:     r.DeletedAt,
		}
	}

	return Series{}
}

// coverImageURL builds a signed imgproxy URL for the given storage bucket and
// path, returning an empty string when either side of the join is missing.
func coverImageURL(bucket, path *string) string {
	if bucket != nil && path != nil && *bucket != "" && *path != "" {
		return mediaURL(*bucket, *path)
	}

	return ""
}

// Service provides blog operations with reader/writer pool separation.
type Service struct {
	store *Store
	db    *db.DB
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
			pool:  database.Writer,
		},
		db: database,
	}
}
