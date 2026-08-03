// Package travel implements the Travel Search & Compare surface (Section 67,
// ADR-085). JapaConnect proxies flight and hotel search queries to a
// configured affiliate travel provider API, injects affiliate tracking
// parameters at render time (ADR-009), and caches identical queries in Valkey.
// No flight or hotel inventory is stored.
//
// Ref: japaconnect-engineering/sections/67-travel-search.md
//      japaconnect-engineering/adr/ADR-085-travel-search.md
package travel

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated Queries for travel_providers operations.
// It is the data-access layer — no business logic here.
type Store struct {
	queries *gen.Queries
	pool    *db.DB
}

// NewStore creates a travel store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Reader),
		pool:    database,
	}
}

// CreateProvider inserts a new travel provider.
func (s *Store) CreateProvider(ctx context.Context, arg gen.CreateTravelProviderParams) (gen.TravelProvider, error) {
	return s.queries.CreateTravelProvider(ctx, arg)
}

// UpdateProvider updates a travel provider's mutable fields.
func (s *Store) UpdateProvider(ctx context.Context, arg gen.UpdateTravelProviderParams) (gen.TravelProvider, error) {
	return s.queries.UpdateTravelProvider(ctx, arg)
}

// GetProviderByID fetches a single provider by ID within a tenant.
func (s *Store) GetProviderByID(ctx context.Context, tenantID, id pgtype.UUID) (gen.TravelProvider, error) {
	return s.queries.GetTravelProviderByID(ctx, gen.GetTravelProviderByIDParams{
		TenantID: tenantID,
		ID:       id,
	})
}

// GetActiveProvider returns the active provider for a given type
// (flight, hotel, or both). Providers of the exact type are preferred over
// "both" providers (see the ORDER BY in the SQL).
func (s *Store) GetActiveProvider(ctx context.Context, tenantID pgtype.UUID, providerType string) (gen.TravelProvider, error) {
	return s.queries.GetActiveTravelProvider(ctx, gen.GetActiveTravelProviderParams{
		TenantID:     tenantID,
		ProviderType: providerType,
	})
}

// ListProviders lists travel providers for a tenant, optionally filtering to
// active only and/or by provider type.
func (s *Store) ListProviders(ctx context.Context, tenantID pgtype.UUID, activeOnly bool, providerType string) ([]gen.TravelProvider, error) {
	rows, err := s.queries.ListTravelProviders(ctx, gen.ListTravelProvidersParams{
		TenantID:     tenantID,
		ActiveOnly:   activeOnly,
		ProviderType: providerType,
	})
	if err != nil {
		return nil, fmt.Errorf("travel: list providers: %w", err)
	}
	return rows, nil
}
