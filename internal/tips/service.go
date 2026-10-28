package tips

import (
	"github.com/jedafe/japa/internal/db"
)

// NewService creates a tips Service backed by the given database pool.
// Inject optional seams via the Set* methods (settings, perms, payments,
// orgs, OC client, notifier, enqueuer) — nil seams degrade gracefully.
func NewService(database *db.DB) *Service {
	return &Service{store: NewStore(database)}
}
