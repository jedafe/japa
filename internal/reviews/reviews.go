// Package reviews implements the Universal Ratings & Reviews system (Section 29).
//
// Polymorphic ratings/reviews shared across Organizations, Universities/Mentors,
// Services, and any future rateable entity. One review per user per entity,
// maintained aggregate rollup via UpsertReviewAggregate. Reviews reuse the
// existing Reporting & Appeals system (Section 22) for moderation rather than
// a review-specific moderation flow.
package reviews

import (
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Review statuses.
const (
	ReviewStatusPublished = "published"
	ReviewStatusFlagged   = "flagged"
	ReviewStatusRemoved   = "removed"
)

// Rating bounds.
const (
	RatingMin int16 = 1
	RatingMax int16 = 5
)

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Service provides review operations with reader/writer pool separation.
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
