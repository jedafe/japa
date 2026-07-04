// Package content implements the Content Type System for JapaConnect
// (Section 14).
//
// The contents table uses a content_type discriminator with JSONB
// field_values for flexible per-type schemas. Every save creates a
// revision row; the live row always reflects the latest revision. Slug
// changes are tracked in slug_history for 301 redirects, preserving SEO
// equity. Reactions are polymorphic (entity_type/entity_id) and use an
// admin-configurable emoji set. Content scores (pgvector-driven) feed
// the Activity Feed sorting pipeline (Section 32).
//
// The Service struct wraps two sqlc-generated Queries instances: one
// reading from the reader pool and one writing to the writer pool.
package content

import (
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Content status values stored in the contents.status column.
const (
	StatusDraft     = "draft"
	StatusPublished = "published"
	StatusScheduled = "scheduled"
	StatusArchived  = "archived"
)

// Built-in content types stored in the contents.content_type column.
// V2 types (event, job_listing, service_listing) are added later.
const (
	TypeBlogPost   = "blog_post"
	TypeDiscussion = "discussion"
	TypePage       = "page"
	TypeComparison = "comparison"
)

// Visibility values stored in the contents.visibility column.
const (
	VisibilityPublic  = "public"
	VisibilityMembers = "members"
	VisibilityPrivate = "private"
)

// EntityType constants for the polymorphic reactions and slug_history tables.
const (
	EntityContent = "content"
	EntitySpace   = "space"
	EntityTag     = "tag"
	EntityUser    = "user"
)

// DefaultPageSize is the default number of items returned by list queries
// when no explicit limit is provided.
const DefaultPageSize = 20

// MaxPageSize is the maximum number of items a client can request in a
// single list query. Larger values are clamped to prevent abuse.
const MaxPageSize = 100

// Service provides content operations with reader/writer pool
// separation. Reads go through the reader pool; writes go through the
// writer pool. This follows the same pattern as the existing db.DB struct.
type Service struct {
	db    *db.DB
	read  *gen.Queries
	write *gen.Queries
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		db:    database,
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
	}
}
