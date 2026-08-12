package tenant

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

// DBStore implements TenantStore using the database reader pool.
type DBStore struct {
	db *db.DB
}

// NewDBStore creates a DBStore backed by the given database.
func NewDBStore(database *db.DB) *DBStore {
	return &DBStore{db: database}
}

// GetTenantByHost looks up an active tenant by domain or slug matching.
// Returns nil (not an error) when no active tenant matches the host, so
// the middleware can fall back to the default tenant. Real database errors
// (connection failures, context cancellation) are propagated so the caller
// never silently routes to the wrong tenant.
func (s *DBStore) GetTenantByHost(ctx context.Context, host string) (*Tenant, error) {
	cleanHost := host

	for i := range len(host) {
		if host[i] == ':' {
			cleanHost = host[:i]
			break
		}
	}

	query := `
		SELECT id, name, slug, domain, status
		FROM tenants
		WHERE (domain = $1 OR slug = $1) AND status = 'active'
		LIMIT 1
	`

	var t Tenant

	var domain pgtype.Text

	err := s.db.Reader.QueryRow(ctx, query, cleanHost).Scan(
		&t.ID, &t.Name, &t.Slug, &domain, &t.Status,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("looking up tenant by host: %w", err)
	}

	t.Domain = domain

	return &t, nil
}

// GetDefaultTenant returns the first active tenant in the database.
// Soft-deleted and suspended tenants are excluded so requests are never
// routed to a dead tenant.
func (s *DBStore) GetDefaultTenant(ctx context.Context) (*Tenant, error) {
	query := `
		SELECT id, name, slug, domain, status
		FROM tenants
		WHERE status = 'active'
		ORDER BY created_at ASC
		LIMIT 1
	`

	var t Tenant

	var domain pgtype.Text

	err := s.db.Reader.QueryRow(ctx, query).Scan(
		&t.ID, &t.Name, &t.Slug, &domain, &t.Status,
	)
	if err != nil {
		return nil, fmt.Errorf("querying default tenant: %w", err)
	}

	t.Domain = domain

	return &t, nil
}
