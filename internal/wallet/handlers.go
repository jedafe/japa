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
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/auth"
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
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
	case errors.Is(err, errpkg.ErrAlreadyExists), errors.Is(err, errpkg.ErrDuplicate):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

// writeJSON serializes the given value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

// writeError writes a JSON error response following the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
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

	writeJSON(w, http.StatusOK, map[string]any{
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

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]int64{"points": points}})
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

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]int64{"credits": credits}})
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

	writeJSON(w, http.StatusOK, map[string]any{"data": entries})
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.PointsToConvert <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_amount", "Points to convert must be positive")
		return
	}

	pointsBalance, creditsBalance, err := h.Service.ConvertPointsToCredits(r.Context(), tenantID, user.UserID, ConvertPointsToCreditsInput{
		PointsToConvert: input.PointsToConvert,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
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

	writeJSON(w, http.StatusOK, map[string]any{"data": settings})
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
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.PointsToCreditsRate <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_rate", "points_to_credits_rate must be positive")
		return
	}

	settings, err := h.Service.UpdateWalletSettings(r.Context(), tenantID, UpdateWalletSettingsInput{
		PointsToCreditsRate: input.PointsToCreditsRate,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": settings})
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

	writeJSON(w, http.StatusOK, map[string]any{"data": rules})
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.ActivityKey == "" {
		writeError(w, http.StatusBadRequest, "missing_activity_key", "activity_key is required")
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

	writeJSON(w, http.StatusCreated, map[string]any{"data": rule})
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
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid rule ID")
		return
	}

	rule, err := h.Service.ToggleActivityRule(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": rule})
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
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid rule ID")
		return
	}

	if err := h.Service.DeleteActivityRule(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"deleted": true}})
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.UserID == "" {
		writeError(w, http.StatusBadRequest, "missing_user_id", "user_id is required")
		return
	}

	if input.LedgerType == "" {
		writeError(w, http.StatusBadRequest, "missing_ledger_type", "ledger_type is required")
		return
	}

	if input.Amount == 0 {
		writeError(w, http.StatusBadRequest, "invalid_amount", "Amount cannot be zero")
		return
	}

	if input.Reason == "" {
		writeError(w, http.StatusBadRequest, "missing_reason", "Reason is required for admin adjustments")
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
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
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
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID")
		return
	}

	points, credits, err := h.Service.GetUserBalance(r.Context(), tenantID, userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]int64{
			"points":  points,
			"credits": credits,
		},
	})
}
