// Package newsletter — handlers.go provides HTTP handlers for newsletter admin CRUD.
//
// Routes:
//
//	GET    /admin/newsletter/subscribers    — list subscribers
//	GET    /admin/newsletter/subscribers/{id} — get subscriber
//	DELETE /admin/newsletter/subscribers/{id} — remove subscriber
//	POST   /admin/newsletter/campaigns      — create campaign
//	GET    /admin/newsletter/campaigns      — list campaigns
//	GET    /admin/newsletter/campaigns/{id} — get campaign
//	PATCH  /admin/newsletter/campaigns/{id} — update campaign
package newsletter

import (
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/tenant"
)

const (
	defaultPageSize int32 = 20
	maxPageSize     int32 = 100
)

// Handlers holds dependencies for newsletter admin HTTP handlers.
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > int(maxPageSize) {
		return maxPageSize
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

func parseOffset(r *http.Request) int32 {
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			if n > math.MaxInt32 {
				return defaultPageSize
			}

			return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
		}
	}

	return 0
}

// ---------------------------------------------------------------------------
// Admin Handlers — Newsletter Subscribers
// ---------------------------------------------------------------------------

// ListSubscribers returns all newsletter subscribers for the tenant.
// GET /admin/newsletter/subscribers.
func (h *Handlers) ListSubscribers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parseOffset(r)

	subscribers, err := h.Service.ListSubscribers(r.Context(), tenantID, limit, offset)
	if err != nil {
		slog.Error("listing newsletter subscribers", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list subscribers")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": subscribers})
}

// GetSubscriber returns a single newsletter subscriber by ID.
// GET /admin/newsletter/subscribers/{id}.
func (h *Handlers) GetSubscriber(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid subscriber ID")
		return
	}

	subscriber, err := h.Service.GetSubscriber(r.Context(), tenantID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Subscriber not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": subscriber})
}

// DeleteSubscriber removes a newsletter subscriber.
// DELETE /admin/newsletter/subscribers/{id}.
func (h *Handlers) DeleteSubscriber(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid subscriber ID")
		return
	}

	if err := h.Service.DeleteSubscriber(r.Context(), tenantID, id); err != nil {
		slog.Error("deleting newsletter subscriber", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete subscriber")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// Admin Handlers — Newsletter Campaigns
// ---------------------------------------------------------------------------

// ListCampaigns returns all newsletter campaigns for the tenant.
// GET /admin/newsletter/campaigns.
func (h *Handlers) ListCampaigns(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parseOffset(r)

	campaigns, err := h.Service.ListCampaigns(r.Context(), tenantID, limit, offset)
	if err != nil {
		slog.Error("listing newsletter campaigns", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list campaigns")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": campaigns})
}

// GetCampaign returns a single newsletter campaign by ID.
// GET /admin/newsletter/campaigns/{id}.
func (h *Handlers) GetCampaign(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid campaign ID")
		return
	}

	campaign, err := h.Service.GetCampaign(r.Context(), tenantID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Campaign not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": campaign})
}

// CreateCampaign creates a new newsletter campaign.
// POST /admin/newsletter/campaigns.
func (h *Handlers) CreateCampaign(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Name    string `json:"name"`
		Subject string `json:"subject"`
		HTML    string `json:"html"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "name is required")
		return
	}

	if input.Subject == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "subject is required")
		return
	}

	campaign, err := h.Service.CreateCampaign(r.Context(), tenantID, input.Name, input.Subject, input.HTML)
	if err != nil {
		slog.Error("creating newsletter campaign", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create campaign")

		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": campaign})
}

// UpdateCampaign updates an existing newsletter campaign.
// PATCH /admin/newsletter/campaigns/{id}.
func (h *Handlers) UpdateCampaign(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid campaign ID")
		return
	}

	var input struct {
		Name    *string `json:"name,omitempty"`
		Subject *string `json:"subject,omitempty"`
		HTML    *string `json:"html,omitempty"`
		Status  *string `json:"status,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	campaign, err := h.Service.UpdateCampaign(r.Context(), tenantID, id, input.Name, input.Subject, input.HTML, input.Status)
	if err != nil {
		slog.Error("updating newsletter campaign", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update campaign")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": campaign})
}

// HandleSendCampaign triggers sending a newsletter campaign.
// POST /admin/newsletter/campaigns/{id}/send.
func (h *Handlers) HandleSendCampaign(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid campaign ID")
		return
	}

	// Fetch the campaign and verify it's in a sendable state.
	campaign, err := h.Service.GetCampaign(r.Context(), tenantID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Campaign not found")
		return
	}

	if campaign.Status != "draft" && campaign.Status != "ready" {
		writeError(w, http.StatusBadRequest, "invalid_status", "Campaign must be in draft or ready status to send")
		return
	}

	// Update campaign status to 'sending'.
	sendingStatus := "sending"
	if _, err := h.Service.UpdateCampaign(r.Context(), tenantID, id, nil, nil, nil, &sendingStatus); err != nil {
		slog.Error("updating campaign status to sending", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update campaign status")

		return
	}

	// Get all active subscriber emails.
	emails, err := h.Service.GetAllSubscriberEmails(r.Context(), tenantID)
	if err != nil {
		slog.Error("getting subscriber emails", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get subscribers")

		return
	}

	if len(emails) == 0 {
		writeError(w, http.StatusBadRequest, "no_subscribers", "No active subscribers to send to")
		return
	}

	// Enqueue the newsletter send task.
	if err := h.Service.EnqueueNewsletterSend(r.Context(), id, tenantID, campaign.Subject, campaign.HTML, emails); err != nil {
		slog.Error("enqueuing newsletter send", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to enqueue newsletter send")

		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]any{
			"status":     "sending",
			"recipients": len(emails),
		},
	})
}
