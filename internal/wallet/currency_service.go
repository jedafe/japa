// Credit Currency System service methods (Section 58, ADR-065).
package wallet

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// errCreditPurchaseDisabled is returned when the wallet.credit_purchase_enabled
// killswitch is false.
var errCreditPurchaseDisabled = errors.New("credit purchasing is disabled")

// PurchaseCreditsInput holds parameters for initiating a credit purchase.
type PurchaseCreditsInput struct {
	CreditAmount int    // custom amount (ignored if PackID is set)
	PackID       string // optional: purchase a pre-defined pack
	Currency     string // optional: explicit currency override
	Provider     string // optional: payment provider override (default: paystack)
	// Geo signals for currency resolution (populated by the handler from
	// VisitorContext + GeoContext). When empty, the resolver falls back to
	// user preference → system default.
	ContentCountry string // VisitorContext.ContentCountry (ADR-064)
	GeoCountry     string // GeoContext.CountryCode (Section 37)
}

// PurchaseCreditsResult holds the result of initiating a credit purchase.
type PurchaseCreditsResult struct {
	PaymentID      string `json:"payment_id"`
	Provider       string `json:"provider"`
	Currency       string `json:"currency"`
	FiatAmount     int64  `json:"fiat_amount"`            // minor units (kobo/cents)
	CreditsToGrant int    `json:"credits_to_grant"`       // credit_amount + bonus_credits
	CheckoutURL    string `json:"checkout_url,omitempty"` // provider checkout URL (if external)
}

// listActiveCurrencies returns all active currencies.
func (s *Service) listActiveCurrencies(ctx context.Context) ([]gen.Currency, error) {
	return s.store.listActiveCurrencies(ctx)
}

// listAllCurrencies returns all currencies including inactive.
func (s *Service) listAllCurrencies(ctx context.Context) ([]gen.Currency, error) {
	return s.store.listAllCurrencies(ctx)
}

// getCurrencyByCode returns a single currency by code.
func (s *Service) getCurrencyByCode(ctx context.Context, code string) (gen.Currency, error) {
	return s.store.getCurrencyByCode(ctx, code)
}

// createCurrency creates a new currency.
func (s *Service) createCurrency(ctx context.Context, p gen.CreateCurrencyParams) (gen.Currency, error) {
	c, err := s.store.createCurrency(ctx, p)
	if err != nil {
		return gen.Currency{}, err
	}

	s.invalidateCurrenciesCache(ctx)

	return c, nil
}

// updateCurrency updates an existing currency.
func (s *Service) updateCurrency(ctx context.Context, p gen.UpdateCurrencyParams) (gen.Currency, error) {
	if p.Name == "" {
		return gen.Currency{}, errors.New("name is required")
	}

	if p.CreditsPerUnit <= 0 {
		return gen.Currency{}, errors.New("credits_per_unit must be positive")
	}

	c, err := s.store.updateCurrency(ctx, p)
	if err != nil {
		return gen.Currency{}, err
	}

	s.invalidateCurrenciesCache(ctx)

	return c, nil
}

// deactivateCurrency soft-deletes a currency.
func (s *Service) deactivateCurrency(ctx context.Context, code string) error {
	if err := s.store.deactivateCurrency(ctx, code); err != nil {
		return err
	}

	s.invalidateCurrenciesCache(ctx)

	return nil
}

// invalidateCurrenciesCache flushes the jc:wallet:currencies:{}
// cache after any currency CRUD operation (Section 58, Rule 4). Currencies
// are platform-wide, so the root tenant ID is used.
func (s *Service) invalidateCurrenciesCache(ctx context.Context) {
	if s.cacheInvalidator == nil {
		return
	}

	_ = s.cacheInvalidator.InvalidateCurrenciesCache(ctx)
}

// countSpacesByCurrency returns how many country spaces use a currency.
func (s *Service) countSpacesByCurrency(ctx context.Context, code string) (int32, error) {
	return s.store.countSpacesByCurrency(ctx, code)
}

// listActiveCreditPacks returns all active credit packs for a tenant.
func (s *Service) listActiveCreditPacks(ctx context.Context) ([]gen.CreditPack, error) {
	return s.store.listActiveCreditPacks(ctx)
}

// listAllCreditPacks returns all credit packs (including inactive) for a tenant.
func (s *Service) listAllCreditPacks(ctx context.Context) ([]gen.CreditPack, error) {
	return s.store.listAllCreditPacks(ctx)
}

// createCreditPack creates a new credit pack.
func (s *Service) createCreditPack(ctx context.Context, p gen.CreateCreditPackParams) (gen.CreditPack, error) {
	return s.store.createCreditPack(ctx, p)
}

// updateCreditPack updates an existing credit pack.
func (s *Service) updateCreditPack(ctx context.Context, p gen.UpdateCreditPackParams) (gen.CreditPack, error) {
	return s.store.updateCreditPack(ctx, p)
}

// deactivateCreditPack soft-deletes a credit pack.
func (s *Service) deactivateCreditPack(ctx context.Context, id pgtype.UUID) error {
	return s.store.deactivateCreditPack(ctx, id)
}

// getUserPreferredCurrency returns the user's explicit currency preference.
func (s *Service) getUserPreferredCurrency(ctx context.Context, userID pgtype.UUID) (string, error) {
	return s.store.getUserPreferredCurrency(ctx, userID)
}

// setPreferredCurrency sets the user's explicit currency preference.
func (s *Service) setPreferredCurrency(ctx context.Context, userID pgtype.UUID, currencyCode string) error {
	// Validate the currency exists and is active.
	c, err := s.store.getCurrencyByCode(ctx, currencyCode)
	if err != nil {
		return fmt.Errorf("invalid currency code: %w", err)
	}

	if !c.IsActive {
		return errors.New("currency is not active")
	}

	return s.store.setUserPreferredCurrency(ctx, userID, currencyCode)
}

// listCreditPurchases returns credit purchase payment rows for admin viewing.
func (s *Service) listCreditPurchases(ctx context.Context, p gen.ListCreditPurchasesParams) ([]gen.ListCreditPurchasesRow, error) {
	if p.PageLimit <= 0 || p.PageLimit > MaxPageSize {
		p.PageLimit = DefaultPageSize
	}

	return s.store.listCreditPurchases(ctx, p)
}

// purchaseCredits initiates a credit purchase by creating a payments row.
// The actual wallet crediting happens async via the wallet:process_credit_purchase
// Asynq task after the payment webhook confirms success.
// // Ref: japaconnect-engineering/sections/58-credit-currency-system.md.
func (s *Service) purchaseCredits(ctx context.Context, userID pgtype.UUID, input PurchaseCreditsInput) (PurchaseCreditsResult, error) {
	// Killswitch: wallet.credit_purchase_enabled (default true). When false,
	// the endpoint returns 403 credit_purchase_disabled but spending existing
	// credits continues to work (Section 58).
	if s.settings != nil {
		if !s.settings.GetBoolOr(ctx, "wallet.credit_purchase_enabled", true) {
			return PurchaseCreditsResult{}, errCreditPurchaseDisabled
		}
	}

	// Resolve the currency. Priority: explicit override > user preference >
	// VisitorContext.ContentCountry → country space → currency_code >
	// GeoContext.CountryCode → country space → currency_code > system default
	// (Section 58, ADR-065).
	currencyCode := strings.ToUpper(input.Currency)
	if currencyCode == "" && s.currencyResolver != nil {
		// Fetch the user's explicit preferred currency from user_profiles.
		preferred, _ := s.store.getUserPreferredCurrency(ctx, userID)
		// VisitorContext.ContentCountry and GeoContext.CountryCode are
		// populated by the handler from the request context (ADR-064 + §37).
		resolved, err := s.currencyResolver.ResolveCurrency(ctx, CurrencyContext{
			PreferredCurrency: preferred,
			ContentCountry:    input.ContentCountry,
			GeoCountry:        input.GeoCountry,
		})
		if err == nil {
			currencyCode = resolved
		}
	}

	if currencyCode == "" {
		// Final fallback: system default via settings, then compiled default.
		if s.settings != nil {
			currencyCode = s.settings.GetStringOr(ctx, "wallet.default_currency", "NGN")
		} else {
			currencyCode = "NGN"
		}
	}

	currency, err := s.store.getCurrencyByCode(ctx, currencyCode)
	if err != nil {
		return PurchaseCreditsResult{}, fmt.Errorf("resolving currency: %w", err)
	}

	if !currency.IsActive {
		return PurchaseCreditsResult{}, errors.New("currency is not active")
	}

	// Determine the credit amount and bonus.
	creditsToGrant := input.CreditAmount
	packName := ""

	if input.PackID != "" {
		var packID pgtype.UUID
		if err := packID.Scan(input.PackID); err != nil {
			return PurchaseCreditsResult{}, fmt.Errorf("invalid pack_id: %w", err)
		}

		pack, err := s.store.getCreditPackByID(ctx, packID)
		if err != nil {
			return PurchaseCreditsResult{}, fmt.Errorf("resolving credit pack: %w", err)
		}

		if !pack.IsActive {
			return PurchaseCreditsResult{}, errors.New("credit pack is not active")
		}

		creditsToGrant = int(pack.CreditAmount + pack.BonusCredits)
		packName = pack.Name
	}

	if creditsToGrant <= 0 {
		return PurchaseCreditsResult{}, errors.New("credits_to_grant must be positive")
	}

	// BUG-W-002: enforce the server-side minimum for custom-amount purchases.
	// The frontend enforces min_purchase_credits client-side, but a direct API
	// caller can bypass it. Pack purchases are exempt (the pack defines its
	// own amount). Defense in depth (Rule 16).
	if input.PackID == "" && s.settings != nil {
		minPurchase := s.settings.GetIntOr(ctx, "wallet.min_purchase_credits", 100)
		if creditsToGrant < minPurchase {
			return PurchaseCreditsResult{}, fmt.Errorf("minimum %d credits required for purchase", minPurchase)
		}
	}

	// Compute fiat price in minor units.
	// credits = fiatMinor * credits_per_unit / 100
	// → fiatMinor = credits * 100 / credits_per_unit
	fiatMinor := int64(creditsToGrant) * 100 / int64(currency.CreditsPerUnit)
	if fiatMinor <= 0 {
		fiatMinor = 1 // minimum 1 minor unit
	}

	// Create the payments row via the Section 17 payment provider abstraction.
	// purpose_type="credit_purchase" so the payment webhook handler can route
	// the success event to the wallet:process_credit_purchase Asynq task.
	// When no payments service is wired (tests), return computed pricing only.
	if s.paymentsService != nil {
		metadata := map[string]interface{}{
			"credits_to_grant": creditsToGrant,
			"currency_code":    currencyCode,
		}
		if packName != "" {
			metadata["pack_name"] = packName
		}

		if input.PackID != "" {
			metadata["pack_id"] = input.PackID
		}

		payment, err := s.paymentsService.CreatePayment(ctx, userID, PaymentCreateInput{
			Provider:    input.Provider,
			PurposeType: "credit_purchase",
			Amount:      fiatMinor,
			Currency:    currencyCode,
			Metadata:    metadata,
		})
		if err != nil {
			return PurchaseCreditsResult{}, fmt.Errorf("creating payment: %w", err)
		}

		return PurchaseCreditsResult{
			PaymentID:      payment.ID,
			Provider:       payment.Provider,
			Currency:       currencyCode,
			FiatAmount:     fiatMinor,
			CreditsToGrant: creditsToGrant,
			CheckoutURL:    payment.CheckoutURL,
		}, nil
	}

	// No payments service wired — return computed pricing (test path).
	return PurchaseCreditsResult{
		PaymentID:      "",
		Provider:       input.Provider,
		Currency:       currencyCode,
		FiatAmount:     fiatMinor,
		CreditsToGrant: creditsToGrant,
	}, nil
}
