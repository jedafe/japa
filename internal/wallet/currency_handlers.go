// Credit Currency System handlers (Section 58, ADR-065).
package wallet

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/geoip"
	"github.com/jedafe/japa/internal/visitor"
)

// ---------------------------------------------------------------------------
// Public: Currencies + Credit Packs
// ---------------------------------------------------------------------------

// ListCurrencies handles GET /api/v1/wallet/currencies.
// Returns all active currencies for display in currency selectors and pricing.
func (h *Handlers) ListCurrencies(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}

	currencies, err := h.Service.listActiveCurrencies(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": currencies})
}

// ListCreditPacks handles GET /api/v1/wallet/credit-packs.
// Returns all active credit packs for the tenant.
func (h *Handlers) ListCreditPacks(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	packs, err := h.Service.listActiveCreditPacks(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": packs})
}

// ---------------------------------------------------------------------------
// User: Preferred Currency
// ---------------------------------------------------------------------------

// GetPreferredCurrency handles GET /api/v1/me/preferences/currency.
func (h *Handlers) GetPreferredCurrency(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	code, err := h.Service.getUserPreferredCurrency(r.Context(), user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"preferred_currency": code,
		},
	})
}

// SetPreferredCurrency handles PATCH /api/v1/me/preferences/currency.
func (h *Handlers) SetPreferredCurrency(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		CurrencyCode string `json:"currency_code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	code := strings.ToUpper(strings.TrimSpace(input.CurrencyCode))
	if code == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_currency_code", "currency_code is required")
		return
	}

	if err := h.Service.setPreferredCurrency(r.Context(), user.UserID, code); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"preferred_currency": code}})
}

// ---------------------------------------------------------------------------
// Control Panel: Currency CRUD (super_admin)
// ---------------------------------------------------------------------------

// ControlListCurrencies handles GET /control/wallet/currencies.
// Returns all currencies including inactive ones.
func (h *Handlers) ControlListCurrencies(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}

	currencies, err := h.Service.listAllCurrencies(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": currencies})
}

// ControlCreateCurrency handles POST /control/wallet/currencies.
func (h *Handlers) ControlCreateCurrency(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}

	var input struct {
		Code           string `json:"code"`
		Name           string `json:"name"`
		Symbol         string `json:"symbol"`
		CreditsPerUnit int    `json:"credits_per_unit"`
		Position       int    `json:"position"`
		Reason         string `json:"reason"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	code := strings.ToUpper(strings.TrimSpace(input.Code))
	if len(code) != 3 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_code", "Currency code must be a 3-letter ISO 4217 code")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	if input.Symbol == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_symbol", "symbol is required")
		return
	}

	if input.CreditsPerUnit <= 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_credits_per_unit", "credits_per_unit must be positive")
		return
	}

	if input.Reason == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_reason", "reason is required for currency adjustments (Section 58)")
		return
	}

	currency, err := h.Service.createCurrency(r.Context(), gen.CreateCurrencyParams{
		Code:           code,
		Name:           input.Name,
		Symbol:         input.Symbol,
		CreditsPerUnit: int32(input.CreditsPerUnit), // #nosec
		Position:       int32(input.Position),       // #nosec
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": currency})
}

// ControlUpdateCurrency handles PATCH /control/wallet/currencies/{code}.
func (h *Handlers) ControlUpdateCurrency(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}

	code := strings.ToUpper(chi.URLParam(r, "code"))
	if code == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_code", "Currency code is required")
		return
	}

	var input struct {
		Name           string `json:"name"`
		Symbol         string `json:"symbol"`
		CreditsPerUnit int    `json:"credits_per_unit"`
		IsActive       *bool  `json:"is_active"`
		Position       int    `json:"position"`
		Reason         string `json:"reason"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Reason == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_reason", "reason is required for currency adjustments (Section 58)")
		return
	}

	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	currency, err := h.Service.updateCurrency(r.Context(), gen.UpdateCurrencyParams{
		Code:           code,
		Name:           input.Name,
		Symbol:         input.Symbol,
		CreditsPerUnit: int32(input.CreditsPerUnit), // #nosec
		IsActive:       isActive,
		Position:       int32(input.Position), // #nosec
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": currency})
}

// ControlDeleteCurrency handles DELETE /control/wallet/currencies/{code}.
// Soft-deletes by setting is_active=false. Never hard-deletes.
func (h *Handlers) ControlDeleteCurrency(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}

	code := strings.ToUpper(chi.URLParam(r, "code"))
	if code == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_code", "Currency code is required")
		return
	}

	// Reason is required for currency adjustments (Section 58).
	var input struct {
		Reason string `json:"reason"`
	}

	_ = json.NewDecoder(r.Body).Decode(&input)

	if input.Reason == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_reason", "reason is required for currency adjustments (Section 58)")
		return
	}

	// Check how many country spaces use this currency (for the response).
	count, _ := h.Service.countSpacesByCurrency(r.Context(), code)

	if err := h.Service.deactivateCurrency(r.Context(), code); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"code":            code,
			"is_active":       false,
			"spaces_affected": count,
		},
	})
}

// ---------------------------------------------------------------------------
// Admin Panel: Credit Pack CRUD
// ---------------------------------------------------------------------------

// AdminListCreditPacks handles GET /api/v1/admin/wallet/credit-packs.
func (h *Handlers) AdminListCreditPacks(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	packs, err := h.Service.listAllCreditPacks(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": packs})
}

// AdminCreateCreditPack handles POST /api/v1/admin/wallet/credit-packs.
func (h *Handlers) AdminCreateCreditPack(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		CreditAmount int    `json:"credit_amount"`
		BonusCredits int    `json:"bonus_credits"`
		MinUserLevel string `json:"min_user_level"`
		Position     int    `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	if input.CreditAmount <= 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_credit_amount", "credit_amount must be positive")
		return
	}

	var description *string
	if input.Description != "" {
		description = &input.Description
	}

	var minUserLevel *string
	if input.MinUserLevel != "" {
		minUserLevel = &input.MinUserLevel
	}

	pack, err := h.Service.createCreditPack(r.Context(), gen.CreateCreditPackParams{

		Name:         input.Name,
		Description:  description,
		CreditAmount: int32(input.CreditAmount), // #nosec
		BonusCredits: int32(input.BonusCredits), // #nosec
		MinUserLevel: minUserLevel,
		Position:     int32(input.Position), // #nosec
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": pack})
}

// AdminUpdateCreditPack handles PATCH /api/v1/admin/wallet/credit-packs/{id}.
func (h *Handlers) AdminUpdateCreditPack(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid credit pack ID")
		return
	}

	var input struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		CreditAmount int    `json:"credit_amount"`
		BonusCredits int    `json:"bonus_credits"`
		MinUserLevel string `json:"min_user_level"`
		IsActive     *bool  `json:"is_active"`
		Position     int    `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	var description *string
	if input.Description != "" {
		description = &input.Description
	}

	var minUserLevel *string
	if input.MinUserLevel != "" {
		minUserLevel = &input.MinUserLevel
	}

	pack, err := h.Service.updateCreditPack(r.Context(), gen.UpdateCreditPackParams{
		ID: id,

		Name:         input.Name,
		Description:  description,
		CreditAmount: int32(input.CreditAmount), // #nosec
		BonusCredits: int32(input.BonusCredits), // #nosec
		MinUserLevel: minUserLevel,
		IsActive:     isActive,
		Position:     int32(input.Position), // #nosec
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": pack})
}

// AdminDeleteCreditPack handles DELETE /api/v1/admin/wallet/credit-packs/{id}.
// Soft-deletes by setting is_active=false.
func (h *Handlers) AdminDeleteCreditPack(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid credit pack ID")
		return
	}

	if err := h.Service.deactivateCreditPack(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": id, "is_active": false}})
}

// ---------------------------------------------------------------------------
// Admin Panel: Credit Purchase History
// ---------------------------------------------------------------------------

// AdminListCreditPurchases handles GET /api/v1/admin/wallet/purchases.
func (h *Handlers) AdminListCreditPurchases(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()

	var params gen.ListCreditPurchasesParams

	params.PageLimit = parsePageSize(r)

	if v := q.Get("page"); v != "" {
		page, _ := strconv.Atoi(v)
		if page < 1 {
			page = 1
		}

		params.PageOffset = int32((page - 1) * int(params.PageLimit)) // #nosec
	}

	if v := q.Get("user_id"); v != "" {
		var uid pgtype.UUID
		if err := uid.Scan(v); err == nil {
			params.UserID = pgtype.UUID{Bytes: uid.Bytes, Valid: true}
		}
	}

	if v := q.Get("currency"); v != "" {
		params.Currency = &v
	}

	if v := q.Get("from"); v != "" {
		var ts pgtype.Timestamptz
		if err := ts.Scan(v); err == nil {
			params.FromDate = pgtype.Timestamptz{Time: ts.Time, Valid: ts.Valid}
		}
	}

	if v := q.Get("to"); v != "" {
		var ts pgtype.Timestamptz
		if err := ts.Scan(v); err == nil {
			params.ToDate = pgtype.Timestamptz{Time: ts.Time, Valid: ts.Valid}
		}
	}

	purchases, err := h.Service.listCreditPurchases(r.Context(), params)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": purchases})
}

// ---------------------------------------------------------------------------
// Public: Purchase Credits
// ---------------------------------------------------------------------------

// PurchaseCredits handles POST /api/v1/wallet/purchase-credits.
// Initiates a credit purchase by creating a payments row.
func (h *Handlers) PurchaseCredits(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		CreditAmount int    `json:"credit_amount"`
		PackID       string `json:"pack_id"`
		Currency     string `json:"currency"`
		Provider     string `json:"provider"` // optional: paystack, stripe, etc.
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.CreditAmount <= 0 && input.PackID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_amount", "credit_amount or pack_id is required")
		return
	}

	// Resolve geo signals from the request context for currency resolution
	// (ADR-064 VisitorContext + Section 37 GeoContext).
	vctx := visitor.FromContext(r.Context())
	geoCtx := geoip.FromContext(r.Context())

	provider := strings.TrimSpace(input.Provider)
	if provider == "" {
		provider = "paystack" // default provider for credit purchases
	}

	result, err := h.Service.purchaseCredits(r.Context(), user.UserID, PurchaseCreditsInput{
		CreditAmount:   input.CreditAmount,
		PackID:         input.PackID,
		Currency:       strings.ToUpper(input.Currency),
		Provider:       provider,
		ContentCountry: vctx.ContentCountry,
		GeoCountry:     geoCtx.CountryCode,
	})
	if err != nil {
		if errors.Is(err, errCreditPurchaseDisabled) {
			errpkg.WriteError(w, http.StatusForbidden, "credit_purchase_disabled", "Credit purchasing is currently disabled")
			return
		}

		handleServiceError(w, err)

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}
