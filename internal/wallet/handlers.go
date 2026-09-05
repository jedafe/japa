// Package wallet — HTTP handlers for the Wallet & Credits System.
//
// Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package wallet

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

const defaultPageSize int32 = 20

// Handlers holds dependencies for wallet HTTP handlers.
type Handlers struct {
	Service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// parseUUIDParam extracts a URL parameter and scans it into a pgtype.UUID.
func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, err
	}

	return id, nil
}

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
}

// stripSentinel removes the sentinel error prefix from an error message.
// For fmt.Errorf("%w: detail", sentinel) it returns just "detail".
func stripSentinel(err error) string {
	msg := err.Error()
	// Find the last ": " separator which separates sentinel from detail
	if i := strings.LastIndex(msg, ": "); i >= 0 {
		return msg[i+2:]
	}

	return msg
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > MaxPageSize {
		return MaxPageSize
	}

	// #nosec G109 // value bounded by MaxPageSize check
	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

// parsePageSizeOffset extracts the page_offset query parameter.
func parsePageSizeOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		return 0
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}

	// #nosec G109 // value bounded by MaxPageSize check
	// #nosec G115 // value bounded by caller
	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

// ---------------------------------------------------------------------------
// Wallet Handlers
// ---------------------------------------------------------------------------

// GetBalance handles GET /api/v1/me/wallet.
func (h *Handlers) GetBalance(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	points, credits, err := h.Service.GetBalance(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// BUG-W-003: include lifetime_points per Section 28 spec
	// ({ points_balance, credits_balance, lifetime_points }).
	lifetimePoints, err := h.Service.GetLifetimePoints(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Ref: japaconnect-engineering/sections/28-wallet-credits.md
	// Returns balances using the canonical Section 28 field names.
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]int64{
			"points_balance":  points,
			"credits_balance": credits,
			"lifetime_points": lifetimePoints,
		},
	})
}

// GetPointsBalance handles GET /api/v1/me/wallet/points.
// Returns the paginated points ledger per Section 28 (BUG-010 — previously
// returned only the balance number). The balance is included as a top-level
// field so clients that only need the number can avoid a second call.
func (h *Handlers) GetPointsBalance(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	points, _, err := h.Service.GetBalance(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	lt := LedgerTypePoints
	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	entries, err := h.Service.ListLedgerEntries(r.Context(), tenantID, user.UserID, &lt, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"balance": points,
			"entries": entries,
		},
	})
}

// GetCreditsBalance handles GET /api/v1/me/wallet/credits.
// Returns the paginated credits ledger per Section 28 (BUG-010 — previously
// returned only the balance number). The balance is included as a top-level
// field so clients that only need the number can avoid a second call.
func (h *Handlers) GetCreditsBalance(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	_, credits, err := h.Service.GetBalance(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	lt := LedgerTypeCredits
	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	entries, err := h.Service.ListLedgerEntries(r.Context(), tenantID, user.UserID, &lt, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"balance": credits,
			"entries": entries,
		},
	})
}

// GetLedger handles GET /api/v1/wallet/ledger.
func (h *Handlers) GetLedger(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Optional ledger_type filter
	var ledgerType *string
	if lt := r.URL.Query().Get("ledger_type"); lt != "" {
		ledgerType = &lt
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	entries, err := h.Service.ListLedgerEntries(r.Context(), tenantID, user.UserID, ledgerType, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": entries})
}

// ConvertPoints handles POST /api/v1/wallet/convert.
func (h *Handlers) ConvertPoints(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		PointsToConvert int64 `json:"points_to_convert"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.PointsToConvert <= 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_amount", "Points to convert must be positive")
		return
	}

	pointsBalance, creditsBalance, err := h.Service.ConvertPointsToCredits(r.Context(), tenantID, user.UserID, ConvertPointsToCreditsInput{
		PointsToConvert: input.PointsToConvert,
	})
	if err != nil {
		slog.Error("points conversion failed", "user_id", user.UserID, "tenant_id", tenantID, "points", input.PointsToConvert, "error", err)
		errpkg.CaptureException(fmt.Errorf("points conversion failed (user=%s, tenant=%s, points=%d): %w", user.UserID, tenantID, input.PointsToConvert, err))
		handleServiceError(w, err)

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]int64{
			"points_balance":  pointsBalance,
			"credits_balance": creditsBalance,
		},
	})
}

// GetSettings handles GET /api/v1/wallet/settings.
func (h *Handlers) GetSettings(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	settings, minConvert, err := h.Service.GetWalletSettingsWithMinConvert(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"settings":           settings,
			"min_convert_points": minConvert,
		},
	})
}

// GetMemberSettings handles GET /api/v1/me/wallet/settings.
//
// Member-facing, read-only view of the conversion rate and minimum convertible
// points so the Wallet settings page can display the effective rate without
// requiring the admin `wallet.admin.settings.manage` node. Reuses the same
// service resolution path as the admin endpoint (per-tenant wallet_settings
// row → global system_settings → compiled defaults), returning the same
// envelope shape (`{ settings, min_convert_points }`) the frontend already
// reads.
//
// Ref: japaconnect-engineering/sections/28-wallet-credits.md (Wallet API Routes).
func (h *Handlers) GetMemberSettings(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	settings, minConvert, err := h.Service.GetWalletSettingsWithMinConvert(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// min_purchase_credits from system_settings (default 100, Section 58).
	// Exposed here so the frontend purchase page can use it instead of
	// hardcoding the minimum (BUG-011, Rule 16).
	minPurchaseCredits := 100
	if h.Service.settings != nil {
		minPurchaseCredits = h.Service.settings.GetIntOr(r.Context(), tenantID, "wallet.min_purchase_credits", 100)
	}

	// min_withdrawal_credits from system_settings (default 1000, Section 58).
	// Exposed here so the frontend withdraw page can use it instead of
	// hardcoding the minimum (Rule 16).
	minWithdrawalCredits := 1000
	if h.Service.settings != nil {
		minWithdrawalCredits = h.Service.settings.GetIntOr(r.Context(), tenantID, "wallet.min_withdrawal_credits", 1000)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"settings":               settings,
			"min_convert_points":     minConvert,
			"min_purchase_credits":   minPurchaseCredits,
			"min_withdrawal_credits": minWithdrawalCredits,
		},
	})
}

// UpdateSettings handles PUT /api/v1/wallet/settings.
func (h *Handlers) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		PointsToCreditsRate int32 `json:"points_to_credits_rate"`
		DailyEarningCap     int32 `json:"daily_earning_cap"`
		EnableEarning       bool  `json:"enable_earning"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.PointsToCreditsRate <= 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_rate", "points_to_credits_rate must be positive")
		return
	}

	settings, err := h.Service.UpdateWalletSettings(r.Context(), tenantID, UpdateWalletSettingsInput{
		PointsToCreditsRate: input.PointsToCreditsRate,
		DailyEarningCap:     input.DailyEarningCap,
		EnableEarning:       input.EnableEarning,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": settings})
}

// ListRules handles GET /api/v1/wallet/rules.
func (h *Handlers) ListRules(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	rules, err := h.Service.ListActivityRules(r.Context(), tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rules})
}

// CreateOrUpdateRule handles POST /api/v1/wallet/rules.
func (h *Handlers) CreateOrUpdateRule(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		ActivityKey string  `json:"activity_key"`
		PointValue  int32   `json:"point_value"`
		IsEnabled   bool    `json:"is_enabled"`
		OneTime     bool    `json:"one_time"`
		CapPerDay   *int32  `json:"cap_per_day"`
		CapPerWeek  *int32  `json:"cap_per_week"`
		Description *string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.ActivityKey == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_activity_key", "activity_key is required")
		return
	}

	rule, err := h.Service.CreateOrUpdateActivityRule(r.Context(), tenantID, CreateOrUpdateActivityRuleInput{
		ActivityKey: input.ActivityKey,
		PointValue:  input.PointValue,
		IsEnabled:   input.IsEnabled,
		OneTime:     input.OneTime,
		CapPerDay:   input.CapPerDay,
		CapPerWeek:  input.CapPerWeek,
		Description: input.Description,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": rule})
}

// UpdateRule handles PATCH /api/v1/admin/wallet/rules/{id}.
// Updates the rule identified by the {id} URL parameter (BUG-005 — previously
// this route called CreateOrUpdateRule which ignored {id} and upserted by
// activity_key, potentially updating the wrong rule).
func (h *Handlers) UpdateRule(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid rule ID")
		return
	}

	var input struct {
		ActivityKey string  `json:"activity_key"`
		PointValue  int32   `json:"point_value"`
		IsEnabled   bool    `json:"is_enabled"`
		OneTime     bool    `json:"one_time"`
		CapPerDay   *int32  `json:"cap_per_day"`
		CapPerWeek  *int32  `json:"cap_per_week"`
		Description *string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	rule, err := h.Service.UpdateActivityRuleByID(r.Context(), id, tenantID, CreateOrUpdateActivityRuleInput{
		ActivityKey: input.ActivityKey,
		PointValue:  input.PointValue,
		IsEnabled:   input.IsEnabled,
		OneTime:     input.OneTime,
		CapPerDay:   input.CapPerDay,
		CapPerWeek:  input.CapPerWeek,
		Description: input.Description,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rule})
}

// ToggleRule handles PATCH /api/v1/wallet/rules/{id}/toggle.
func (h *Handlers) ToggleRule(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid rule ID")
		return
	}

	rule, err := h.Service.ToggleActivityRule(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rule})
}

// DeleteRule handles DELETE /api/v1/admin/wallet/rules/{id}.
func (h *Handlers) DeleteRule(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid rule ID")
		return
	}

	if err := h.Service.DeleteActivityRule(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"deleted": true}})
}

// AdminAdjust handles POST /api/v1/wallet/admin/adjust.
func (h *Handlers) AdminAdjust(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		UserID     string `json:"user_id"`
		LedgerType string `json:"ledger_type"`
		Amount     int64  `json:"amount"`
		Reason     string `json:"reason"`
		Reference  string `json:"reference"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.UserID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_user_id", "user_id is required")
		return
	}

	if input.LedgerType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_ledger_type", "ledger_type is required")
		return
	}

	if input.Amount == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_amount", "Amount cannot be zero")
		return
	}

	if input.Reason == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_reason", "Reason is required for admin adjustments")
		return
	}

	balance, err := h.Service.AdminAdjustBalance(r.Context(), tenantID, user.UserID, AdminAdjustInput{
		UserID:     input.UserID,
		LedgerType: input.LedgerType,
		Amount:     input.Amount,
		Reason:     input.Reason,
		Reference:  input.Reference,
	})
	if err != nil {
		slog.Error("admin wallet adjustment failed", "target_user_id", input.UserID, "tenant_id", tenantID, "amount", input.Amount, "error", err)
		errpkg.CaptureException(fmt.Errorf("admin wallet adjustment failed (target_user=%s, tenant=%s, amount=%d): %w", input.UserID, tenantID, input.Amount, err))
		handleServiceError(w, err)

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]int64{
			"balance": balance,
		},
	})
}

// AdminGetUserBalance handles GET /api/v1/wallet/admin/user/{userId}/balance.
func (h *Handlers) AdminGetUserBalance(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	userID, err := parseUUIDParam(r, "userId")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID")
		return
	}

	username, points, credits, err := h.Service.GetUserBalanceWithInfo(r.Context(), tenantID, userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"user_id":  userID.String(),
			"username": username,
			"points":   points,
			"credits":  credits,
		},
	})
}

// CalculateWithdrawal handles POST /api/v1/wallet/withdraw/calculate.
//
// Ref: japaconnect-engineering/sections/58-credit-currency-system.md.
func (h *Handlers) CalculateWithdrawal(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		CreditAmount int64  `json:"credit_amount"`
		CurrencyCode string `json:"currency_code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	result, err := h.Service.CalculateWithdrawal(r.Context(), tenantID, user.UserID, CalculateWithdrawalInput{
		CreditAmount: input.CreditAmount,
		CurrencyCode: input.CurrencyCode,
	})
	if err != nil {
		slog.Error("withdrawal calculation failed", "user_id", user.UserID, "tenant_id", tenantID, "error", err)
		handleServiceError(w, err)

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// CreateWithdrawal handles POST /api/v1/me/wallet/withdraw.
//
// Ref: japaconnect-engineering/sections/58-credit-currency-system.md.
func (h *Handlers) CreateWithdrawal(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		CreditAmount   int64  `json:"credit_amount"`
		CurrencyCode   string `json:"currency_code"`
		PayoutMethodID string `json:"payout_method_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")

	result, err := h.Service.CreateWithdrawal(r.Context(), tenantID, user.UserID, CreateWithdrawalInput{
		CreditAmount:   input.CreditAmount,
		CurrencyCode:   input.CurrencyCode,
		PayoutMethodID: input.PayoutMethodID,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		slog.Error("withdrawal creation failed", "user_id", user.UserID, "tenant_id", tenantID, "error", err)
		handleServiceError(w, err)

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// ListWithdrawalProviders handles GET /api/v1/wallet/withdraw/providers.
// Returns the supported payout provider keys so the frontend can render the
// provider selector without hardcoding the list (Rule 16).
func (h *Handlers) ListWithdrawalProviders(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	providers := h.Service.ListWithdrawalProviders(r.Context())
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": providers})
}

// CreateWithdrawalMethod handles POST /api/v1/me/wallet/withdrawal-methods.
func (h *Handlers) CreateWithdrawalMethod(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Provider     string `json:"provider"`
		Label        string `json:"label"`
		Account      string `json:"account"`
		CurrencyCode string `json:"currency_code"`
		IsDefault    bool   `json:"is_default"`
		Metadata     any    `json:"metadata"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	method, err := h.Service.CreateWithdrawalMethod(r.Context(), tenantID, user.UserID, CreateWithdrawalMethodInput{
		Provider:     input.Provider,
		Label:        input.Label,
		Account:      input.Account,
		CurrencyCode: input.CurrencyCode,
		IsDefault:    input.IsDefault,
		Metadata:     input.Metadata,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": method})
}

// ListWithdrawalMethods handles GET /api/v1/me/wallet/withdrawal-methods.
func (h *Handlers) ListWithdrawalMethods(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	methods, err := h.Service.ListWithdrawalMethods(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": methods})
}

// AdminListWithdrawals handles GET /api/v1/admin/wallet/withdrawals.
func (h *Handlers) AdminListWithdrawals(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	status := r.URL.Query().Get("status")
	userID := r.URL.Query().Get("user_id")
	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	withdrawals, count, err := h.Service.AdminListWithdrawals(r.Context(), tenantID, status, userID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":  withdrawals,
		"count": count,
	})
}

// ProcessWithdrawal handles POST /api/v1/admin/wallet/withdrawals/{id}/process.
func (h *Handlers) ProcessWithdrawal(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid withdrawal ID")
		return
	}

	var body struct {
		Action    string `json:"action"`
		AdminNote string `json:"admin_note"`
	}

	_ = json.NewDecoder(r.Body).Decode(&body)

	var result WithdrawalResult
	if body.Action == "reject" {
		result, err = h.Service.RejectWithdrawal(r.Context(), tenantID, user.UserID, id, body.AdminNote)
	} else {
		result, err = h.Service.ProcessWithdrawal(r.Context(), tenantID, user.UserID, id)
	}

	if err != nil {
		slog.Error("process withdrawal failed", "withdrawal_id", id, "tenant_id", tenantID, "error", err)
		handleServiceError(w, err)

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// ListWithdrawals handles GET /api/v1/me/wallet/withdrawals.
func (h *Handlers) ListWithdrawals(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	withdrawals, err := h.Service.ListWithdrawals(r.Context(), tenantID, user.UserID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": withdrawals})
}
