// Package blog implements the Blog Extension system (Section 24).
//
// Blog series for multi-part posts, content review workflow for editorial
// review, featured/curated posts, and RSS feed generation. Tags-only model
// (no categories) with granular permission nodes through dynamic RBAC.
package blog

import (
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
		},
		db: database,
	}
}
