// Package travel implements the Travel Search & Compare surface (Section 67,
// ADR-085). JapaConnect proxies flight and hotel search queries to a
// configured affiliate travel provider API, injects affiliate tracking
// parameters at render time (ADR-009), and serves repeat queries from a
// layered store: Valkey cache, then persisted travel_search_results rows.
//
// Ref: japaconnect-engineering/sections/67-travel-search.md
//
//	japaconnect-engineering/adr/ADR-085-travel-search.md
package travel

import (
	"context"
	"fmt"
	"strings"

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

// DeleteProvider deletes a travel provider row by id and platform.
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

// GetProviderByID fetches a single provider by ID within a platform.
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

// ListProviders lists travel providers for a platform with optional search and
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

// escapeILike escapes ILIKE wildcards so user input matches literally — a
// typed "%" must not turn the airport search into a catalog dump.
var ilikeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// SearchAirports searches the IATA airport catalog for the /travel/locations
// autocomplete (§67 amendment — flight-location pickers, not geo spaces).
func (s *Store) SearchAirports(ctx context.Context, q string, limit int32) ([]gen.SearchTravelAirportsRow, error) {
	rows, err := s.queries.SearchTravelAirports(ctx, gen.SearchTravelAirportsParams{
		Q:        ilikeEscaper.Replace(q),
		RowLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("travel: search airports: %w", err)
	}

	return rows, nil
}

// GetSearchResult returns the persisted result row for a canonical query, or
// pgx.ErrNoRows when none exists (§67 stored-results layer).
func (s *Store) GetSearchResult(ctx context.Context, searchType, queryHash string) (gen.TravelSearchResult, error) {
	return s.queries.GetTravelSearchResult(ctx, gen.GetTravelSearchResultParams{
		SearchType: searchType,
		QueryHash:  queryHash,
	})
}

// UpsertSearchResult inserts or refreshes the persisted result row for a
// canonical query (§67 stored-results layer).
func (s *Store) UpsertSearchResult(ctx context.Context, arg gen.UpsertTravelSearchResultParams) (gen.TravelSearchResult, error) {
	return s.queries.UpsertTravelSearchResult(ctx, arg)
}

// BumpSearchResultHit increments the served-from-store counter on a result
// row — observability for how often the stored layer satisfies a search.
func (s *Store) BumpSearchResultHit(ctx context.Context, id pgtype.UUID) {
	// Best-effort counter — a failed UPDATE must not fail the request.
	_ = s.queries.BumpTravelSearchResultHit(ctx, id)
}

// ---------------------------------------------------------------------------
// Offer inventory (ADR-124)
// ---------------------------------------------------------------------------

// UpsertFlightOffer inserts or refreshes a single provider offer row.
func (s *Store) UpsertFlightOffer(ctx context.Context, arg gen.UpsertFlightOfferParams) error {
	return s.queries.UpsertFlightOffer(ctx, arg)
}

// UpsertHotelOffer inserts or refreshes a single provider offer row.
func (s *Store) UpsertHotelOffer(ctx context.Context, arg gen.UpsertHotelOfferParams) error {
	return s.queries.UpsertHotelOffer(ctx, arg)
}

// ListFlightOffersByCoverage returns offer rows for a flight coverage —
// fresh-only when includeExpired is false, expired rows included for the
// stale-fallback path otherwise.
func (s *Store) ListFlightOffersByCoverage(ctx context.Context, arg gen.ListFlightOffersByCoverageParams) ([]gen.TravelFlightOffer, error) {
	return s.queries.ListFlightOffersByCoverage(ctx, arg)
}

// ListHotelOffersByCoverage mirrors ListFlightOffersByCoverage for hotels.
func (s *Store) ListHotelOffersByCoverage(ctx context.Context, arg gen.ListHotelOffersByCoverageParams) ([]gen.TravelHotelOffer, error) {
	return s.queries.ListHotelOffersByCoverage(ctx, arg)
}

// GetCoverage returns the coverage bookkeeping row for a coverage hash +
// provider, or pgx.ErrNoRows when the coverage was never fetched.
func (s *Store) GetCoverage(ctx context.Context, searchType, coverageKey string, providerID pgtype.UUID) (gen.TravelSearchCoverage, error) {
	return s.queries.GetTravelSearchCoverage(ctx, gen.GetTravelSearchCoverageParams{
		SearchType:  searchType,
		CoverageKey: coverageKey,
		ProviderID:  providerID,
	})
}

// UpsertCoverage records that a coverage space was fetched from a provider.
func (s *Store) UpsertCoverage(ctx context.Context, arg gen.UpsertTravelSearchCoverageParams) error {
	return s.queries.UpsertTravelSearchCoverage(ctx, arg)
}

// UpsertTravelHotel inserts or refreshes a hotel-directory property row.
// Identity is (provider_id, hotel_key) — entity-scoped, not coverage-scoped.
func (s *Store) UpsertTravelHotel(ctx context.Context, arg gen.UpsertTravelHotelParams) error {
	return s.queries.UpsertTravelHotel(ctx, arg)
}

// ListTravelHotels pages the hotel directory with optional city, name and
// star filters. name_q is ILIKE-escaped here so callers pass raw input.
func (s *Store) ListTravelHotels(ctx context.Context, arg gen.ListTravelHotelsParams) ([]gen.TravelHotel, error) {
	arg.NameQ = ilikeEscaper.Replace(arg.NameQ)
	return s.queries.ListTravelHotels(ctx, arg)
}

// CountTravelHotels returns the filtered directory size for pagination.
func (s *Store) CountTravelHotels(ctx context.Context, arg gen.CountTravelHotelsParams) (int64, error) {
	arg.NameQ = ilikeEscaper.Replace(arg.NameQ)
	return s.queries.CountTravelHotels(ctx, arg)
}

// PruneInventory deletes expired offers and coverage rows past the retention
// window — runs alongside PruneTravelSearchResults in the weekly task.
func (s *Store) PruneInventory(ctx context.Context) error {
	if err := s.queries.PruneTravelFlightOffers(ctx); err != nil {
		return fmt.Errorf("travel: prune flight offers: %w", err)
	}

	if err := s.queries.PruneTravelHotelOffers(ctx); err != nil {
		return fmt.Errorf("travel: prune hotel offers: %w", err)
	}

	if err := s.queries.PruneTravelSearchCoverage(ctx); err != nil {
		return fmt.Errorf("travel: prune coverage: %w", err)
	}

	return nil
}

// FindTravelAirlineByIATA resolves an airline by its 2-char IATA code,
// preferring 'iata:'-keyed rows when real-world code reuse creates dupes.
func (s *Store) FindTravelAirlineByIATA(ctx context.Context, iata string) (gen.TravelAirline, error) {
	return s.queries.FindTravelAirlineByIATA(ctx, &iata)
}

// FindTravelAirlineByName resolves an airline by exact case-insensitive
// name match.
func (s *Store) FindTravelAirlineByName(ctx context.Context, name string) (gen.TravelAirline, error) {
	return s.queries.FindTravelAirlineByName(ctx, name)
}

// UpsertTravelAirline inserts an observed airline or refreshes
// last_seen_at on an existing catalog row.
func (s *Store) UpsertTravelAirline(ctx context.Context, arg gen.UpsertTravelAirlineParams) error {
	return s.queries.UpsertTravelAirline(ctx, arg)
}

// UpsertTravelProviderAirline records that a provider's offers carried an
// airline (the catalog ↔ travel_providers coverage join).
func (s *Store) UpsertTravelProviderAirline(ctx context.Context, arg gen.UpsertTravelProviderAirlineParams) error {
	return s.queries.UpsertTravelProviderAirline(ctx, arg)
}

// ListTravelAirlines pages the airline directory with optional q (name /
// IATA / ICAO / country), country and provider-slug filters. Filter inputs
// are ILIKE-escaped here so callers pass raw input.
func (s *Store) ListTravelAirlines(ctx context.Context, arg gen.ListTravelAirlinesParams) ([]gen.ListTravelAirlinesRow, error) {
	arg.Q = ilikeEscaper.Replace(arg.Q)
	arg.Country = ilikeEscaper.Replace(arg.Country)
	return s.queries.ListTravelAirlines(ctx, arg)
}

// CountTravelAirlines returns the filtered directory size for pagination.
func (s *Store) CountTravelAirlines(ctx context.Context, arg gen.CountTravelAirlinesParams) (int64, error) {
	arg.Q = ilikeEscaper.Replace(arg.Q)
	arg.Country = ilikeEscaper.Replace(arg.Country)
	return s.queries.CountTravelAirlines(ctx, arg)
}
