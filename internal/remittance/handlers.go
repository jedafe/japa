// Package remittance — handlers.go provides the HTTP handlers for the
// Remittance Rate Tracker routes (Section 74, ADR-075).
//
// Public routes (no auth):
//
//	GET    /api/v1/remittance/rates        — ?send=GBP&receive=NGN
//	GET    /api/v1/remittance/calculate    — ?send_amount=&send=GBP&receive=NGN
//	GET    /api/v1/remittance/history      — ?send=GBP&receive=NGN&days=30
//	GET    /api/v1/remittance/providers    — active providers
//	GET    /api/v1/remittance/corridors    — supported currency pairs
//
// Authenticated routes:
//
//	GET    /api/v1/remittance/alerts       — user's alerts
//	POST   /api/v1/remittance/alerts       — create alert
//	PATCH  /api/v1/remittance/alerts/:id   — update alert
//	DELETE /api/v1/remittance/alerts/:id   — delete alert
//
// Admin routes:
//
//	GET    /api/v1/admin/remittance/providers              — list all providers
//	POST   /api/v1/admin/remittance/providers              — create provider
//	PATCH  /api/v1/admin/remittance/providers/:id          — update provider
//	POST   /api/v1/admin/remittance/providers/:id/rates    — enter manual rate
//	POST   /api/v1/admin/remittance/providers/:id/fetch    — trigger fetch
//
// Ref: japaconnect-engineering/sections/74-remittance-tracker.md
//
//	japaconnect-engineering/adr/ADR-075-remittance-rate-tracker.md
package remittance

import (
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/crypto"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// safeInt32 converts an int to int32, clamping to math.MaxInt32 if too large.
// Used for pagination parameters to prevent integer overflow (gosec G109/G115).
func safeInt32(n int) int32 {
	if n > math.MaxInt32 {
		return math.MaxInt32
	}

	if n < 0 {
		return 0
	}

	return int32(n)
}

// Handlers exposes the remittance tracker HTTP endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates remittance handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as rag/handlers.go)
// ---------------------------------------------------------------------------

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	return pgtype.UUID{}, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return u, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
		return pgtype.UUID{}, false
	}

	return id, true
}

// ---------------------------------------------------------------------------
// Public routes
// ---------------------------------------------------------------------------

// HandleGetRates handles GET /api/v1/remittance/rates — latest rates for a
// corridor. Query: send, receive.
func (h *Handlers) HandleGetRates(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	send := r.URL.Query().Get("send")
	receive := r.URL.Query().Get("receive")

	if send == "" || receive == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_params", "send and receive query params are required")
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "remittance_unavailable", "Remittance service not configured")
		return
	}

	rates, err := h.service.GetRates(r.Context(), tenantID, send, receive)
	if err != nil {
		slog.Error("remittance: get rates failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "rates_failed", "Failed to get rates")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": rates,
	})
}

// HandleCalculate handles GET /api/v1/remittance/calculate — send-amount
// calculator. Query: send_amount, send, receive.
func (h *Handlers) HandleCalculate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	send := r.URL.Query().Get("send")
	receive := r.URL.Query().Get("receive")
	sendAmountStr := r.URL.Query().Get("send_amount")

	if send == "" || receive == "" || sendAmountStr == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_params", "send_amount, send, and receive query params are required")
		return
	}

	sendAmount, err := strconv.ParseFloat(sendAmountStr, 64)
	if err != nil || sendAmount <= 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_send_amount", "send_amount must be a positive number")
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "remittance_unavailable", "Remittance service not configured")
		return
	}

	result, err := h.service.Calculate(r.Context(), tenantID, sendAmount, send, receive)
	if err != nil {
		slog.Error("remittance: calculate failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "calculate_failed", "Failed to calculate")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": result,
	})
}

// HandleGetHistory handles GET /api/v1/remittance/history — rate history
// time-series. Query: send, receive, days (default 30, max 365).
func (h *Handlers) HandleGetHistory(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	send := r.URL.Query().Get("send")
	receive := r.URL.Query().Get("receive")

	if send == "" || receive == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_params", "send and receive query params are required")
		return
	}

	days := 30

	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "remittance_unavailable", "Remittance service not configured")
		return
	}

	points, err := h.service.GetHistory(r.Context(), tenantID, send, receive, days)
	if err != nil {
		slog.Error("remittance: get history failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "history_failed", "Failed to get history")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": points,
	})
}

// HandleGetProviders handles GET /api/v1/remittance/providers — list active
// providers.
func (h *Handlers) HandleGetProviders(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "remittance_unavailable", "Remittance service not configured")
		return
	}

	providers, err := h.service.GetProviders(r.Context(), tenantID, true)
	if err != nil {
		slog.Error("remittance: get providers failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "providers_failed", "Failed to get providers")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": providers,
	})
}

// HandleGetCorridors handles GET /api/v1/remittance/corridors — supported
// currency pairs.
func (h *Handlers) HandleGetCorridors(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "remittance_unavailable", "Remittance service not configured")
		return
	}

	corridors, err := h.service.GetCorridors(r.Context(), tenantID)
	if err != nil {
		slog.Error("remittance: get corridors failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "corridors_failed", "Failed to get corridors")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": corridors,
	})
}

// ---------------------------------------------------------------------------
// Authenticated routes — alerts
// ---------------------------------------------------------------------------

// HandleListAlerts handles GET /api/v1/remittance/alerts — user's alerts.
func (h *Handlers) HandleListAlerts(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "remittance_unavailable", "Remittance service not configured")
		return
	}

	alerts, err := h.service.ListUserAlerts(r.Context(), tenantID, user.UserID)
	if err != nil {
		slog.Error("remittance: list alerts failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "alerts_failed", "Failed to list alerts")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": alerts,
	})
}

// HandleCreateAlert handles POST /api/v1/remittance/alerts — create alert.
// Body: {send, receive, target_rate, direction}.
func (h *Handlers) HandleCreateAlert(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var body struct {
		Send       string  `json:"send"`
		Receive    string  `json:"receive"`
		TargetRate float64 `json:"target_rate"`
		Direction  string  `json:"direction"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if body.Send == "" || body.Receive == "" || body.TargetRate <= 0 || body.Direction == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_params", "send, receive, target_rate, and direction are required")
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "remittance_unavailable", "Remittance service not configured")
		return
	}

	alert, err := h.service.CreateAlert(r.Context(), tenantID, user.UserID, body.Send, body.Receive, body.Direction, body.TargetRate)
	if err != nil {
		slog.Error("remittance: create alert failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_alert_failed", "Failed to create alert")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": alert,
	})
}

// HandleUpdateAlert handles PATCH /api/v1/remittance/alerts/:id — update alert.
// Body: {target_rate?, direction?, is_active?}.
func (h *Handlers) HandleUpdateAlert(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	alertID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		TargetRate *float64 `json:"target_rate"`
		Direction  *string  `json:"direction"`
		IsActive   *bool    `json:"is_active"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "remittance_unavailable", "Remittance service not configured")
		return
	}

	alert, err := h.service.UpdateAlert(r.Context(), tenantID, user.UserID, alertID, body.TargetRate, body.Direction, body.IsActive)
	if err != nil {
		slog.Error("remittance: update alert failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_alert_failed", "Failed to update alert")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": alert,
	})
}

// HandleDeleteAlert handles DELETE /api/v1/remittance/alerts/:id — delete alert.
func (h *Handlers) HandleDeleteAlert(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	alertID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "remittance_unavailable", "Remittance service not configured")
		return
	}

	if err := h.service.DeleteAlert(r.Context(), user.UserID, alertID); err != nil {
		slog.Error("remittance: delete alert failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_alert_failed", "Failed to delete alert")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"deleted": true},
	})
}

// ---------------------------------------------------------------------------
// Admin routes
// ---------------------------------------------------------------------------

// HandleAdminListProviders handles GET /api/v1/admin/remittance/providers —
// paginated list of all providers.
func (h *Handlers) HandleAdminListProviders(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	limit := int32(20)

	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = safeInt32(parsed)
		}
	}

	offset := int32(0)

	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = safeInt32(parsed)
		}
	}

	activeOnly := false
	if a := r.URL.Query().Get("active_only"); a == "true" {
		activeOnly = true
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "remittance_unavailable", "Remittance service not configured")
		return
	}

	providers, total, err := h.service.ListProvidersPaginated(r.Context(), tenantID, activeOnly, limit, offset)
	if err != nil {
		slog.Error("remittance: admin list providers failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "providers_failed", "Failed to list providers")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": providers,
		"pagination": map[string]any{
			"total":  total,
			"limit":  limit,
			"offset": offset,
		},
	})
}

// HandleAdminCreateProvider handles POST /api/v1/admin/remittance/providers —
// create provider.
// Body: {name, slug, website_url?, data_source, api_config?, is_active, display_order}.
func (h *Handlers) HandleAdminCreateProvider(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var body struct {
		Name         string          `json:"name"`
		Slug         string          `json:"slug"`
		WebsiteURL   *string         `json:"website_url"`
		DataSource   string          `json:"data_source"`
		APIConfig    json.RawMessage `json:"api_config"`
		IsActive     bool            `json:"is_active"`
		DisplayOrder int32           `json:"display_order"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if body.Name == "" || body.Slug == "" || body.DataSource == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_params", "name, slug, and data_source are required")
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "remittance_unavailable", "Remittance service not configured")
		return
	}

	var apiConfig []byte

	if len(body.APIConfig) > 0 {
		enc, err := crypto.EncryptJSON(body.APIConfig)
		if err != nil {
			slog.Error("remittance: failed to encrypt api_config", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "encryption_failed", "Failed to encrypt provider credentials")

			return
		}

		apiConfig = enc
	}

	provider, err := h.service.CreateProvider(r.Context(), gen.CreateRemittanceProviderParams{

		Name:         body.Name,
		Slug:         body.Slug,
		WebsiteUrl:   body.WebsiteURL,
		DataSource:   body.DataSource,
		ApiConfig:    apiConfig,
		IsActive:     body.IsActive,
		DisplayOrder: body.DisplayOrder,
	})
	if err != nil {
		slog.Error("remittance: admin create provider failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_provider_failed", "Failed to create provider")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": provider,
	})
}

// HandleAdminUpdateProvider handles PATCH /api/v1/admin/remittance/providers/:id
// — update provider config.
// Body: {name?, website_url?, api_config?, is_active?, display_order?}.
func (h *Handlers) HandleAdminUpdateProvider(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	providerID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Name         *string         `json:"name"`
		WebsiteURL   *string         `json:"website_url"`
		APIConfig    json.RawMessage `json:"api_config"`
		IsActive     *bool           `json:"is_active"`
		DisplayOrder *int32          `json:"display_order"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "remittance_unavailable", "Remittance service not configured")
		return
	}

	var apiConfig []byte

	if len(body.APIConfig) > 0 {
		enc, err := crypto.EncryptJSON(body.APIConfig)
		if err != nil {
			slog.Error("remittance: failed to encrypt api_config", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "encryption_failed", "Failed to encrypt provider credentials")

			return
		}

		apiConfig = enc
	}

	provider, err := h.service.UpdateProvider(r.Context(), gen.UpdateRemittanceProviderParams{

		ID:           providerID,
		Name:         body.Name,
		WebsiteUrl:   body.WebsiteURL,
		ApiConfig:    apiConfig,
		IsActive:     body.IsActive,
		DisplayOrder: body.DisplayOrder,
	})
	if err != nil {
		slog.Error("remittance: admin update provider failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_provider_failed", "Failed to update provider")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": provider,
	})
}

// HandleAdminInsertManualRate handles POST /api/v1/admin/remittance/providers/:id/rates
// — enter a manual rate.
// Body: {send_currency, receive_currency, rate, fee_flat?, fee_pct?, transfer_time?, source_url?}.
func (h *Handlers) HandleAdminInsertManualRate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	providerID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		SendCurrency    string  `json:"send_currency"`
		ReceiveCurrency string  `json:"receive_currency"`
		Rate            float64 `json:"rate"`
		FeeFlat         float64 `json:"fee_flat"`
		FeePct          float64 `json:"fee_pct"`
		TransferTime    *string `json:"transfer_time"`
		SourceURL       *string `json:"source_url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if body.SendCurrency == "" || body.ReceiveCurrency == "" || body.Rate <= 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_params", "send_currency, receive_currency, and rate are required")
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "remittance_unavailable", "Remittance service not configured")
		return
	}

	rate, err := h.service.InsertManualRate(r.Context(), tenantID, providerID, body.SendCurrency, body.ReceiveCurrency, body.Rate, body.FeeFlat, body.FeePct, body.TransferTime, body.SourceURL)
	if err != nil {
		slog.Error("remittance: admin insert manual rate failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "insert_rate_failed", "Failed to insert manual rate")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"id":               rate.ID.String(),
			"provider_id":      rate.ProviderID.String(),
			"send_currency":    rate.SendCurrency,
			"receive_currency": rate.ReceiveCurrency,
			"rate":             numericToFloat64(rate.Rate),
			"fee_flat":         numericToFloat64(rate.FeeFlat),
			"fee_pct":          numericToFloat64(rate.FeePct),
			"transfer_time":    ptrString(rate.TransferTime),
			"fetched_at":       rfc3339(rate.FetchedAt),
			"is_manual":        rate.IsManual,
			"source_url":       ptrString(rate.SourceUrl),
		},
	})
}

// HandleAdminTriggerFetch handles POST /api/v1/admin/remittance/providers/:id/fetch
// — trigger a manual rate fetch for a provider. Enqueues a
// remittance:fetch_rates task (fire-and-forget per Rule 8).
func (h *Handlers) HandleAdminTriggerFetch(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	providerID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "remittance_unavailable", "Remittance service not configured")
		return
	}

	if err := h.service.TriggerFetch(r.Context(), tenantID, providerID); err != nil {
		slog.Error("remittance: admin trigger fetch failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "fetch_failed", "Failed to trigger fetch")

		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]any{
			"status": "queued",
		},
	})
}
