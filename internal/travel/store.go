// Package travel implements the Travel Search & Compare surface (Section 67,
// ADR-085). JapaConnect proxies flight and hotel search queries to a
// configured affiliate travel provider API, injects affiliate tracking
// parameters at render time (ADR-009), and caches identical queries in Valkey.
// No flight or hotel inventory is stored.
//
// Ref: japaconnect-engineering/sections/67-travel-search.md
//
//	japaconnect-engineering/adr/ADR-085-travel-search.md
package travel

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
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

// DeleteProvider deletes a travel provider row by id and tenant.
func (s *Store) DeleteProvider(ctx context.Context, id pgtype.UUID) error {
	cmd, err := s.pool.Writer.Exec(ctx, `DELETE FROM travel_providers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("travel: delete provider: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// GetProviderByID fetches a single provider by ID within a tenant.
func (s *Store) GetProviderByID(ctx context.Context, id pgtype.UUID) (gen.TravelProvider, error) {
	return s.queries.GetTravelProviderByID(ctx,

		id,
	)
}

// GetActiveProvider returns the active provider for a given type
// (flight, hotel, or both). Providers of the exact type are preferred over
// "both" providers (see the ORDER BY in the SQL).
func (s *Store) GetActiveProvider(ctx context.Context, providerType string) (gen.TravelProvider, error) {
	return s.queries.GetActiveTravelProvider(ctx,

		providerType,
	)
}

// ListProviders lists travel providers for a tenant with optional search and
// filters. isActiveFilter is tri-state: "true" = only active, "false" = only
// inactive, "" = all. q is an ILIKE search on name/slug (empty = no filter).
// providerType filters by exact type (empty = all).
func (s *Store) ListProviders(ctx context.Context, isActiveFilter, providerType, q string) ([]gen.TravelProvider, error) {
	rows, err := s.queries.ListTravelProviders(ctx, gen.ListTravelProvidersParams{

		IsActiveFilter: isActiveFilter,
		ProviderType:   providerType,
		Q:              q,
	})
	if err != nil {
		return nil, fmt.Errorf("travel: list providers: %w", err)
	}

	return rows, nil
}
