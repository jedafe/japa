package tenant

import (
	"context"
	"fmt"

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

// GetTenantByHost looks up a tenant by domain or slug matching.
// Returns nil (not an error) if no tenant matches the host.
func (s *DBStore) GetTenantByHost(ctx context.Context, host string) (*Tenant, error) {
	cleanHost := host
	for i := 0; i < len(host); i++ {
		if host[i] == ':' {
			cleanHost = host[:i]
			break
		}
	}

	query := `
		SELECT id, name, slug, domain
		FROM tenants
		WHERE domain = $1 OR slug = $1
		LIMIT 1
	`
	var t Tenant
	var domain pgtype.Text
	err := s.db.Reader.QueryRow(ctx, query, cleanHost).Scan(
		&t.ID, &t.Name, &t.Slug, &domain,
	)
	if err != nil {
		return nil, nil
	}
	t.Domain = domain
	return &t, nil
}

// GetDefaultTenant returns the first tenant in the database.
func (s *DBStore) GetDefaultTenant(ctx context.Context) (*Tenant, error) {
	query := `
		SELECT id, name, slug, domain
		FROM tenants
		ORDER BY created_at ASC
		LIMIT 1
	`
	var t Tenant
	var domain pgtype.Text
	err := s.db.Reader.QueryRow(ctx, query).Scan(
		&t.ID, &t.Name, &t.Slug, &domain,
	)
	if err != nil {
		return nil, fmt.Errorf("querying default tenant: %w", err)
	}
	t.Domain = domain
	return &t, nil
}
