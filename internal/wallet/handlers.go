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
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/httpx"
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

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
	case errors.Is(err, errpkg.ErrAlreadyExists), errors.Is(err, errpkg.ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, "already_exists", "Resource already exists")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
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

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

// ---------------------------------------------------------------------------
// Wallet Handlers
// ---------------------------------------------------------------------------

// GetBalance handles GET /api/v1/wallet/balance.
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

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]int64{
			"points":  points,
			"credits": credits,
		},
	})
}

// GetPointsBalance handles GET /api/v1/me/wallet/points.
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

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]int64{"points": points}})
}

// GetCreditsBalance handles GET /api/v1/me/wallet/credits.
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

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]int64{"credits": credits}})
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

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": entries})
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
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.PointsToConvert <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_amount", "Points to convert must be positive")
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

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
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

	settings, err := h.Service.GetWalletSettings(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": settings})
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
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.PointsToCreditsRate <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_rate", "points_to_credits_rate must be positive")
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

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": settings})
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

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": rules})
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
		CapPerDay   *int32  `json:"cap_per_day"`
		CapPerWeek  *int32  `json:"cap_per_week"`
		Description *string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.ActivityKey == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_activity_key", "activity_key is required")
		return
	}

	rule, err := h.Service.CreateOrUpdateActivityRule(r.Context(), tenantID, CreateOrUpdateActivityRuleInput{
		ActivityKey: input.ActivityKey,
		PointValue:  input.PointValue,
		IsEnabled:   input.IsEnabled,
		CapPerDay:   input.CapPerDay,
		CapPerWeek:  input.CapPerWeek,
		Description: input.Description,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": rule})
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

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid rule ID")
		return
	}

	rule, err := h.Service.ToggleActivityRule(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": rule})
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

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid rule ID")
		return
	}

	if err := h.Service.DeleteActivityRule(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"deleted": true}})
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
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.UserID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_user_id", "user_id is required")
		return
	}

	if input.LedgerType == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_ledger_type", "ledger_type is required")
		return
	}

	if input.Amount == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_amount", "Amount cannot be zero")
		return
	}

	if input.Reason == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_reason", "Reason is required for admin adjustments")
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

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
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

	userID, err := httpx.ParseUUIDParam(r, "userId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID")
		return
	}

	points, credits, err := h.Service.GetUserBalance(r.Context(), tenantID, userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]int64{
			"points":  points,
			"credits": credits,
		},
	})
}
