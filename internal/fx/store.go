// Package fx implements the FX Currency Converter (Section 112, ADR-125):
// a mid-market base→quote matrix fed by pluggable rate engines, powering the
// any-to-any converter and the remittance benchmark provider.
//
// Ref: japaconnect-engineering/sections/112-fx-converter.md
//
//	japaconnect-engineering/adr/ADR-125-fx-converter.md
package fx

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated Queries for fx operations.
// It is the data-access layer — no business logic here.
type Store struct {
	queries *gen.Queries
	pool    *db.DB
}

// NewStore creates an fx store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Reader),
		pool:    database,
	}
}

// InsertRate inserts a single matrix cell row.
func (s *Store) InsertRate(ctx context.Context, arg gen.InsertFxRateParams) (gen.FxRate, error) {
	return s.queries.InsertFxRate(ctx, arg)
}

// GetLatestMatrix returns the latest rate per quote for a base currency.
func (s *Store) GetLatestMatrix(ctx context.Context, base string) ([]gen.GetLatestFxMatrixRow, error) {
	return s.queries.GetLatestFxMatrix(ctx, base)
}

// LastFetchTime returns the newest fetched_at for a source engine.
func (s *Store) LastFetchTime(ctx context.Context, source string) (time.Time, error) {
	t, err := s.queries.GetLatestFxFetchTime(ctx, source)
	if err != nil {
		return time.Time{}, err
	}

	return t.Time, nil
}

// ListCurrencies returns active §58 currencies that have matrix coverage.
func (s *Store) ListCurrencies(ctx context.Context) ([]gen.ListFxCurrenciesRow, error) {
	return s.queries.ListFxCurrencies(ctx)
}

// PruneRates deletes rows older than cutoff, keeping one snapshot per
// base/quote/day for older data (same downsampling as remittance §74).
// Returns the number of rows deleted.
func (s *Store) PruneRates(ctx context.Context, cutoff time.Time) (int64, error) {
	return s.queries.PruneFxRates(ctx,
		pgtype.Timestamptz{Time: cutoff, Valid: true},
	)
}
