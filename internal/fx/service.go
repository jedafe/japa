// Package fx — service.go holds the business logic for the FX Currency
// Converter (Section 112, ADR-125): latest-matrix resolution with Valkey
// caching, any-to-any cross-rate conversion, matrix persistence for the
// rate-engine adapters, and retention pruning.
// It reads fx.* system_settings via the hot-reloadable config.Settings.
//
// Ref: japaconnect-engineering/sections/112-fx-converter.md
//
//	japaconnect-engineering/adr/ADR-125-fx-converter.md
package fx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
)

// ErrUnsupportedCurrency is returned when a currency code is absent from
// the latest matrix. Handlers map it to 400 unsupported_currency.
var ErrUnsupportedCurrency = errors.New("fx: unsupported currency")

// ErrEmptyMatrix is returned when no matrix has been fetched yet.
// Handlers map it to 503 fx_unavailable.
var ErrEmptyMatrix = errors.New("fx: no rate matrix available")

// ErrDisabled is returned when the fx.enabled flag is off. The route-level
// features.Gate normally 404s first; this is the service-level backstop.
// Handlers map it to 404 feature_disabled.
var ErrDisabled = errors.New("fx: converter disabled")

// FeatureChecker resolves feature flag state from the feature_flags table —
// the single source of truth for reachability (Ref: internal/features/features.go).
// system_settings is consulted only as a fallback when no checker is injected.
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// Service holds the FX converter business logic.
type Service struct {
	store    *Store
	settings *config.Settings
	cache    valkeycompat.Cmdable // nil = caching disabled
	checker  FeatureChecker
}

// NewService creates an fx service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// SetCache injects the Valkey client for matrix caching.
func (s *Service) SetCache(c valkeycompat.Cmdable) { s.cache = c }

// SetFeatureChecker injects the feature flag checker so IsEnabled reads the
// feature_flags table instead of drifting against system_settings.
func (s *Service) SetFeatureChecker(c FeatureChecker) { s.checker = c }

// IsEnabled returns whether the FX converter is globally enabled.
// Prefers the injected FeatureChecker; falls back to system_settings.
func (s *Service) IsEnabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "fx.enabled")
	}

	if s.settings == nil {
		return true
	}

	return s.settings.GetBoolOr(ctx, "fx.enabled", true)
}

// cacheTTL returns the configured Valkey TTL for the cached matrix.
func (s *Service) cacheTTL(ctx context.Context) time.Duration {
	if s.settings == nil {
		return 120 * time.Minute
	}

	return time.Duration(s.settings.GetIntOr(ctx, "fx.cache_ttl_minutes", 120)) * time.Minute
}

// staleHours returns the configured staleness threshold in hours.
func (s *Service) staleHours(ctx context.Context) int {
	if s.settings == nil {
		return 48
	}

	return s.settings.GetIntOr(ctx, "fx.stale_hours", 48)
}

// HistoryRetentionDays returns the configured fx_rates retention period.
func (s *Service) HistoryRetentionDays(ctx context.Context) int {
	if s.settings == nil {
		return 90
	}

	return s.settings.GetIntOr(ctx, "fx.history_retention_days", 90)
}

// DefaultBaseCurrency returns the base currency the matrix is stored under.
func (s *Service) DefaultBaseCurrency(ctx context.Context) string {
	if s.settings == nil {
		return "USD"
	}

	return s.settings.GetStringOr(ctx, "fx.default_base_currency", "USD")
}

// ---------------------------------------------------------------------------
// Numeric helpers
// ---------------------------------------------------------------------------

// numericToFloat64 converts a pgtype.Numeric to float64, returning 0 for NULL.
func numericToFloat64(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}

	f, err := n.Float64Value()
	if err != nil {
		return 0
	}

	return f.Float64
}

// numericFromFloat converts a float64 to a pgtype.Numeric.
func numericFromFloat(f float64) pgtype.Numeric {
	n := pgtype.Numeric{}
	_ = n.Scan(strconv.FormatFloat(f, 'f', -1, 64))

	return n
}

// ---------------------------------------------------------------------------
// Matrix
// ---------------------------------------------------------------------------

// Matrix is the latest mid-market rate set for a base currency.
// Rates maps ISO quote currency → units of quote per 1 unit of base.
type Matrix struct {
	Base      string             `json:"base"`
	Rates     map[string]float64 `json:"rates"`
	FetchedAt time.Time          `json:"fetched_at"`
}

// matrixCacheKey builds the Valkey key for a cached base matrix.
// The empty platform segment ("::") matches the jc:remit:rates:: convention
// for platform-global data (Rule 4).
// Ref: conventions/valkey-keys-catalog.md (jc:fx:matrix).
func matrixCacheKey(base string) string {
	return valkey.MustKey("fx:matrix::%s", base)
}

// matrixFromRows folds GetLatestFxMatrix rows into a Matrix. The base→base
// cell is always present (=1.0) even if the source engine omitted it.
// FetchedAt is the newest fetched_at across the rows.
func matrixFromRows(base string, rows []gen.GetLatestFxMatrixRow) (*Matrix, error) {
	if len(rows) == 0 {
		return nil, ErrEmptyMatrix
	}

	m := &Matrix{Base: base, Rates: make(map[string]float64, len(rows)+1)}

	for _, r := range rows {
		m.Rates[r.QuoteCurrency] = numericToFloat64(r.Rate)
		if r.FetchedAt.Valid && r.FetchedAt.Time.After(m.FetchedAt) {
			m.FetchedAt = r.FetchedAt.Time
		}
	}

	// Canonical base→base cell — set last so a corrupt stored row can never
	// displace it.
	m.Rates[base] = 1

	return m, nil
}

// matrixStale reports whether a matrix fetched at fetchedAt is older than
// the configured staleness threshold, evaluated against now.
func matrixStale(fetchedAt time.Time, staleHours int, now time.Time) bool {
	return now.Sub(fetchedAt) > time.Duration(staleHours)*time.Hour
}

// GetMatrix returns the latest matrix for a base: Valkey first, DB fallback.
// On a DB hit the matrix is cached for fx.cache_ttl_minutes.
func (s *Service) GetMatrix(ctx context.Context, base string) (*Matrix, error) {
	key := matrixCacheKey(base)
	if s.cache != nil {
		if cached, err := s.cache.Get(ctx, key).Result(); err == nil && cached != "" {
			var m Matrix
			if err := json.Unmarshal([]byte(cached), &m); err == nil {
				return &m, nil
			}
		}
	}

	rows, err := s.store.GetLatestMatrix(ctx, base)
	if err != nil {
		return nil, fmt.Errorf("fx: get matrix: %w", err)
	}

	m, err := matrixFromRows(base, rows)
	if err != nil {
		return nil, err
	}

	if s.cache != nil {
		if data, err := json.Marshal(m); err == nil {
			if err := s.cache.Set(ctx, key, data, s.cacheTTL(ctx)).Err(); err != nil {
				slog.Warn("fx: matrix cache set failed", "error", err)
			}
		}
	}

	return m, nil
}

// InvalidateMatrix drops the cached matrix for a base (called post-fetch).
func (s *Service) InvalidateMatrix(ctx context.Context, base string) {
	if s.cache == nil {
		return
	}

	if err := s.cache.Del(ctx, matrixCacheKey(base)).Err(); err != nil {
		slog.Warn("fx: invalidate matrix cache failed", "base", base, "error", err)
	}
}

// ---------------------------------------------------------------------------
// Conversion
// ---------------------------------------------------------------------------

// ConvertResult is the JSON-serialisable conversion output.
type ConvertResult struct {
	Amount    float64 `json:"amount"`
	From      string  `json:"from"`
	To        string  `json:"to"`
	Rate      float64 `json:"rate"`
	Converted float64 `json:"converted"`
	FetchedAt string  `json:"fetched_at"`
	Stale     bool    `json:"stale"`
}

// computeConvert does the pure cross-rate math (unit-testable, no I/O):
// rate = matrix[to] / matrix[from], converted = rate * amount.
func computeConvert(m *Matrix, amount float64, from, to string) (rate, converted float64, err error) {
	if amount <= 0 {
		return 0, 0, errors.New("fx: amount must be positive")
	}

	fromRate, ok := m.Rates[from]
	if !ok || fromRate <= 0 {
		return 0, 0, fmt.Errorf("%w: %s", ErrUnsupportedCurrency, from)
	}

	toRate, ok := m.Rates[to]
	if !ok || toRate <= 0 {
		return 0, 0, fmt.Errorf("%w: %s", ErrUnsupportedCurrency, to)
	}

	rate = toRate / fromRate

	return rate, rate * amount, nil
}

// Convert converts amount of `from` to `to` against the latest matrix.
// `from`/`to` are normalised to uppercase ISO codes by the caller (handler).
func (s *Service) Convert(ctx context.Context, amount float64, from, to string) (*ConvertResult, error) {
	if !s.IsEnabled(ctx) {
		return nil, ErrDisabled
	}

	m, err := s.GetMatrix(ctx, s.DefaultBaseCurrency(ctx))
	if err != nil {
		return nil, err
	}

	rate, converted, err := computeConvert(m, amount, from, to)
	if err != nil {
		return nil, err
	}

	return &ConvertResult{
		Amount:    amount,
		From:      from,
		To:        to,
		Rate:      rate,
		Converted: converted,
		FetchedAt: m.FetchedAt.UTC().Format(time.RFC3339),
		Stale:     matrixStale(m.FetchedAt, s.staleHours(ctx), time.Now()),
	}, nil
}

// baseRateFromMatrix resolves units of `quote` per 1 unit of the matrix
// base — the pure core of BaseRate. The base→base cell (=1.0) is always
// present in a well-formed matrix (matrixFromRows guarantees it), so the
// base quote resolves without a special case.
func baseRateFromMatrix(m *Matrix, quote string) (float64, error) {
	rate, ok := m.Rates[quote]
	if !ok || rate <= 0 {
		return 0, fmt.Errorf("%w: %s", ErrUnsupportedCurrency, quote)
	}

	return rate, nil
}

// BaseRate returns units of `quote` per 1 unit of the configured default
// base currency (fx.default_base_currency) plus the base code and matrix
// freshness — the read seam for "(base)" display equivalents and
// cost-of-living normalization (base amount = local amount / rate).
// `quote` is normalised to uppercase by the caller's convention.
func (s *Service) BaseRate(ctx context.Context, quote string) (rate float64, base string, fetchedAt time.Time, stale bool, err error) {
	if !s.IsEnabled(ctx) {
		return 0, "", time.Time{}, false, ErrDisabled
	}

	base = s.DefaultBaseCurrency(ctx)

	m, err := s.GetMatrix(ctx, base)
	if err != nil {
		return 0, base, time.Time{}, false, err
	}

	stale = matrixStale(m.FetchedAt, s.staleHours(ctx), time.Now())

	rate, err = baseRateFromMatrix(m, strings.ToUpper(quote))
	if err != nil {
		return 0, base, m.FetchedAt, stale, err
	}

	return rate, base, m.FetchedAt, stale, nil
}

// GetRates returns the matrix re-based to `base` (cross-computed when the
// requested base differs from the stored base). Same payload shape as a
// Matrix plus the stale flag.
func (s *Service) GetRates(ctx context.Context, base string) (*Matrix, bool, error) {
	if !s.IsEnabled(ctx) {
		return nil, false, ErrDisabled
	}

	m, err := s.GetMatrix(ctx, s.DefaultBaseCurrency(ctx))
	if err != nil {
		return nil, false, err
	}

	stale := matrixStale(m.FetchedAt, s.staleHours(ctx), time.Now())

	if base == m.Base {
		return m, stale, nil
	}

	pivot, ok := m.Rates[base]
	if !ok || pivot <= 0 {
		return nil, false, fmt.Errorf("%w: %s", ErrUnsupportedCurrency, base)
	}

	rebased := &Matrix{Base: base, Rates: make(map[string]float64, len(m.Rates)), FetchedAt: m.FetchedAt}
	for q, r := range m.Rates {
		rebased.Rates[q] = r / pivot
	}

	return rebased, stale, nil
}

// ListCurrencies returns the picker currency list — active currencies that
// have matrix coverage.
func (s *Service) ListCurrencies(ctx context.Context) ([]gen.ListFxCurrenciesRow, error) {
	if !s.IsEnabled(ctx) {
		return nil, ErrDisabled
	}

	return s.store.ListCurrencies(ctx)
}

// LastFetchTime returns the newest fetch timestamp for a source engine —
// used by the adapter throttle. Returns zero time when nothing fetched yet.
func (s *Service) LastFetchTime(ctx context.Context, source string) (time.Time, error) {
	t, err := s.store.LastFetchTime(ctx, source)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, nil
		}

		return time.Time{}, fmt.Errorf("fx: last fetch time: %w", err)
	}

	return t, nil
}

// InsertMatrix persists a full base→quote matrix atomically in one
// transaction. The rates map should include the base→base=1.0 cell (the
// adapter adds it if absent). The cached matrix is invalidated on success.
func (s *Service) InsertMatrix(ctx context.Context, base, source string, rates map[string]float64) error {
	tx, err := s.store.pool.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("fx: insert matrix: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	q := gen.New(tx)
	for quote, rate := range rates {
		if _, err := q.InsertFxRate(ctx, gen.InsertFxRateParams{
			BaseCurrency:  base,
			QuoteCurrency: quote,
			Rate:          numericFromFloat(rate),
			Source:        &source,
		}); err != nil {
			return fmt.Errorf("fx: insert matrix row %s: %w", quote, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("fx: insert matrix: commit: %w", err)
	}

	s.InvalidateMatrix(ctx, base)

	return nil
}

// PruneRates deletes matrix rows past fx.history_retention_days, keeping one
// snapshot per base/quote/day for older data (fx:prune_rates task, §112).
func (s *Service) PruneRates(ctx context.Context) (int, error) {
	cutoff := time.Now().AddDate(0, 0, -s.HistoryRetentionDays(ctx))

	n, err := s.store.PruneRates(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("fx: prune rates: %w", err)
	}

	return int(n), nil
}
