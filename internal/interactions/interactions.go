// Package interactions implements user-facing block and mute handlers
// for the social graph (Sections 41, 52).
package interactions

import (
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Service provides block/mute operations with reader/writer pool separation.
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
