// Package comments implements the unified comment system for JapaConnect
// (Section 49). Comments are attached to any content type via content_id.
// Nested replies support up to depth 3. Votes use upsert semantics.
package comments

import (
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Service provides comment operations with reader/writer pool separation.
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
