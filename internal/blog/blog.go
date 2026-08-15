// Package blog implements the Blog Extension system (Section 24).
//
// Blog series for multi-part posts, content review workflow for editorial
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
//
// BUG-011: Section 24 mandates GET /api/v1/blog accept ?space_id, ?tag
// (tag slug), ?series_id, ?sort=latest|top|featured, and ?author_id. The
// sort=featured case is handled by the separate /blog/featured endpoint,
// so callers should map sort=featured to that endpoint before reaching
// the listing query.
type BlogListFilter struct {
	TenantID pgtype.UUID
	SpaceID  pgtype.UUID // zero → no filter
	AuthorID pgtype.UUID // zero → no filter
	SeriesID pgtype.UUID // zero → no filter
	Tag      *string     // nil → no filter; tag slug
	Sort     string      // "latest" (default) | "top"
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
