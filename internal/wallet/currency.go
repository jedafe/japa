// Currency resolver and conversion helpers for the Credit Currency System
// (Section 58, ADR-065).
//
// Provides:
//   - ResolveCurrency: determines which fiat currency to use for a given user
//     at checkout time, using the priority chain:
//     explicit > VisitorContext.ContentCountry > GeoContext.CountryCode > default.
//   - FiatToCredits: converts a fiat amount to credits using the currency's
//     credits_per_unit rate.
//   - CreditsToFiat: converts credits to a fiat amount for display.
package wallet

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// CurrencyContext holds the geo signals needed for currency resolution.
type CurrencyContext struct {
	// User's explicit preferred currency (from user_profiles.preferred_currency).
	// Empty = not set.
	PreferredCurrency string
	// VisitorContext.ContentCountry (ADR-064) — ISO 3166-1 alpha-2 country code.
	// Empty = not resolved.
	ContentCountry string
	// GeoContext.CountryCode (Section 37) — ISO 3166-1 alpha-2 from GeoIP.
	// Empty = not available.
	GeoCountry string
}

// CurrencyResolver resolves the active currency for a user and caches the
// active currency table in memory (refreshed from Valkey/DB on miss).
type CurrencyResolver struct {
	store *Store
	// defaultCurrency is the system_settings fallback (wallet.default_currency).
	// Read once at init and on settings change.
	defaultCurrency string
	mu              sync.RWMutex
}

// NewCurrencyResolver creates a resolver backed by the given store.
func NewCurrencyResolver(store *Store, defaultCurrency string) *CurrencyResolver {
	if defaultCurrency == "" {
		defaultCurrency = "NGN"
	}

	return &CurrencyResolver{
		store:           store,
		defaultCurrency: defaultCurrency,
	}
}

// SetDefaultCurrency updates the fallback currency (called when
// wallet.default_currency system setting changes).
func (r *CurrencyResolver) SetDefaultCurrency(code string) {
	if code == "" {
		return
	}

	r.mu.Lock()
	r.defaultCurrency = code
	r.mu.Unlock()
}

// DefaultCurrency returns the configured fallback currency code.
func (r *CurrencyResolver) DefaultCurrency() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.defaultCurrency
}

// ResolveCurrency determines the currency code for a user given their geo context.
// Priority: PreferredCurrency > ContentCountry → spaces.currency_code >
// GeoCountry → spaces.currency_code > default.
//
// Ref: japaconnect-engineering/sections/58-credit-currency-system.md.
func (r *CurrencyResolver) ResolveCurrency(ctx context.Context, tenantID pgtype.UUID, cc CurrencyContext) (string, error) {
	// 1. Explicit user preference.
	if cc.PreferredCurrency != "" {
		active, err := r.isCurrencyActive(ctx, cc.PreferredCurrency)
		if err == nil && active {
			return cc.PreferredCurrency, nil
		}
	}

	// 2. VisitorContext.ContentCountry → country space → currency_code.
	if cc.ContentCountry != "" {
		if code, err := r.store.getCurrencyByCountryCode(ctx, cc.ContentCountry, tenantID); err == nil {
			return code, nil
		}
	}

	// 3. GeoContext.CountryCode → country space → currency_code.
	if cc.GeoCountry != "" {
		if code, err := r.store.getCurrencyByCountryCode(ctx, cc.GeoCountry, tenantID); err == nil {
			return code, nil
		}
	}

	// 4. Fallback to system default.
	return r.DefaultCurrency(), nil
}

// isCurrencyActive checks if a currency code exists and is active.
func (r *CurrencyResolver) isCurrencyActive(ctx context.Context, code string) (bool, error) {
	c, err := r.store.getCurrencyByCode(ctx, code)
	if err != nil {
		return false, err
	}

	return c.IsActive, nil
}

// FiatToCredits converts a fiat amount to credits using the currency's
// credits_per_unit rate. fiatAmount is in the currency's minor units
// (e.g. kobo for NGN, cents for USD).
//
//	credits = fiatAmount * credits_per_unit / 100
//
// (The /100 is because amounts are stored in minor units, but
// credits_per_unit is per major unit: 1 USD = 1400 credits, not 1 cent = 1400.)
func FiatToCredits(fiatAmountMinor int64, currency gen.Currency) int {
	if currency.CreditsPerUnit <= 0 {
		return 0
	}

	return int(fiatAmountMinor * int64(currency.CreditsPerUnit) / 100)
}

// CreditsToFiat converts credits to a fiat amount in the currency's minor units
// for display purposes.
//
//	fiatMinor = credits * 100 / credits_per_unit
func CreditsToFiat(credits int, currency gen.Currency) int64 {
	if currency.CreditsPerUnit <= 0 {
		return 0
	}

	return int64(credits) * 100 / int64(currency.CreditsPerUnit)
}

// FormatFiat formats a minor-unit amount as a major-unit string with the
// currency symbol (e.g. 140000 minor → "₦1,400.00").
func FormatFiat(fiatMinor int64, currency gen.Currency) string {
	major := float64(fiatMinor) / 100.0
	return fmt.Sprintf("%s%.2f", currency.Symbol, major)
}
