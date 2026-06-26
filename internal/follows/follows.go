// Package follows implements the universal polymorphic Follow System (Section 31).
// Users can follow other users, Spaces, Tags, Organizations, and Professional
// Profiles. A single follows table handles all entity types. Follower counts
// are denormalized on entity tables and maintained in the service layer.
package follows

import (
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Entity type constants for the polymorphic follows table.
const (
	EntityTypeUser                = "user"
	EntityTypeSpace               = "space"
	EntityTypeTag                 = "tag"
	EntityTypeOrganization        = "organization"
	EntityTypeProfessionalProfile = "professional_profile"
)

// DefaultPageSize is the default number of items returned by list queries.
const DefaultPageSize = 20

// MaxPageSize is the maximum number of items a client can request.
const MaxPageSize = 100

// Service provides follow operations with reader/writer pool separation.
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
