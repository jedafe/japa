// Package newsletter — handlers.go provides HTTP handlers for the Newsletter
// & Digest System (Section 62, ADR-080). Covers both legacy admin CRUD and
// subscriber preferences, one-click unsubscribe, and campaign management.
//
// Routes:
//
//	POST   /newsletter/subscribe             — subscribe (public)
//	DELETE /newsletter/unsubscribe           — unsubscribe via token or email
//	GET    /newsletter/preferences           — subscriber preferences
//	PATCH  /newsletter/preferences           — update digest/campaign toggles
//	GET    /admin/newsletter/subscribers     — list subscribers ?status=
//	GET    /admin/newsletter/subscribers/{id} — get subscriber
//	DELETE /admin/newsletter/subscribers/{id} — remove subscriber
//	POST   /admin/newsletter/campaigns       — create campaign
//	GET    /admin/newsletter/campaigns       — list campaigns
//	GET    /admin/newsletter/campaigns/{id} — get campaign
//	PATCH  /admin/newsletter/campaigns/{id} — update campaign
//	POST   /admin/newsletter/campaigns/{id}/send — send campaign
//	GET    /admin/newsletter/stats           — totals, open/click rates
package newsletter

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/tenant"
)

const (
	defaultPageSize int32 = 20
	maxPageSize     int32 = 100
)

// extractTenant extracts the tenant ID from the request context.
func extractTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusBadRequest, "missing_tenant", "Tenant ID required")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireID parses the :id URL parameter as a UUID.
func requireID(w http.ResponseWriter, r *http.Request, paramName, entityName string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, paramName)); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid "+entityName+" ID")
		return pgtype.UUID{}, false
	}

	return id, true
}

// getByID is a shared helper for GET /{entity}/{id} handlers.
func getByID[T any](w http.ResponseWriter, r *http.Request, entityName string, fetchFn func(ctx context.Context, tenantID, id pgtype.UUID) (T, error)) {
	tenantID, ok := extractTenant(w, r)
	if !ok {
		return
	}

	id, ok := requireID(w, r, "id", entityName)
	if !ok {
		return
	}

	result, err := fetchFn(r.Context(), tenantID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", entityName+" not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

// deleteByID is a shared helper for DELETE /{entity}/{id} handlers.
func deleteByID(w http.ResponseWriter, r *http.Request, entityName string, deleteFn func(ctx context.Context, tenantID, id pgtype.UUID) error) {
	tenantID, ok := extractTenant(w, r)
	if !ok {
		return
	}

	id, ok := requireID(w, r, "id", entityName)
	if !ok {
		return
	}

	if err := deleteFn(r.Context(), tenantID, id); err != nil {
		slog.Error("deleting newsletter "+entityName, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete "+entityName)

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// Handlers holds dependencies for newsletter admin HTTP handlers.
type Handlers struct {
	Service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// ---------------------------------------------------------------------------
// Public Handlers — Newsletter Subscribe
// ---------------------------------------------------------------------------

// Subscribe handles POST /api/v1/newsletter/subscribe — public, no auth
// required. Allows guests and authenticated users to subscribe to the
// newsletter. Gated by feature.newsletter at the route level.
// Ref: conventions/api-routes-catalog.md (POST /newsletter/subscribe)
func (h *Handlers) Subscribe(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Global killswitch (newsletter.enabled, ADR-080 §62).
	if !h.Service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "newsletter_disabled", "Newsletter is not available")
		return
	}

	var input struct {
		Email  string `json:"email"`
		Source string `json:"source"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	email := strings.TrimSpace(input.Email)
	if email == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "email is required")
		return
	}

	if !isValidEmail(email) {
		writeError(w, http.StatusBadRequest, "validation_error", "A valid email is required")
		return
	}

	// Link to user ID if authenticated.
	var userID *pgtype.UUID
	if u := auth.FromContext(r.Context()); u != nil {
		userID = &u.UserID
	}

	subscriber, err := h.Service.CreateSubscriber(r.Context(), tenantID, email, userID, input.Source)
	if err != nil {
		// Duplicate email on the unique (tenant_id, email) index → already subscribed.
		if isDuplicateSubscriberErr(err) {
			writeJSON(w, http.StatusOK, map[string]any{
				"data": map[string]string{"status": "already_subscribed"},
			})
			return
		}
		slog.Error("creating newsletter subscriber (public)", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to subscribe")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": subscriber})
}

// isValidEmail performs a basic RFC-5321 email format check. The canonical
// validation is the DB constraint on newsletter_subscribers.email; this guard
// rejects obviously malformed input before hitting the store.
func isValidEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	if at <= 0 || at == len(s)-1 {
		return false
	}
	dot := strings.IndexByte(s[at+1:], '.')
	return dot > 0
}

// isDuplicateSubscriberErr reports whether err is a unique-violation from the
// (tenant_id, email) index on newsletter_subscribers.
func isDuplicateSubscriberErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "23505")
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

// CreateSubscriber adds a new newsletter subscriber.
// POST /admin/newsletter/subscribers.
func (h *Handlers) CreateSubscriber(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Email  string `json:"email"`
		Source string `json:"source"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.Email == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "email is required")
		return
	}

	// Admin-created subscribers default to 'import' source.
	if input.Source == "" {
		input.Source = SourceImport
	}

	subscriber, err := h.Service.CreateSubscriber(r.Context(), tenantID, input.Email, nil, input.Source)
	if err != nil {
		slog.Error("creating newsletter subscriber", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create subscriber")

		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": subscriber})
}

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
	getByID(w, r, "subscriber", func(ctx context.Context, tenantID, id pgtype.UUID) (any, error) {
		return h.Service.GetSubscriber(ctx, tenantID, id)
	})
}

// DeleteSubscriber removes a newsletter subscriber.
// DELETE /admin/newsletter/subscribers/{id}.
func (h *Handlers) DeleteSubscriber(w http.ResponseWriter, r *http.Request) {
	deleteByID(w, r, "subscriber", func(ctx context.Context, tenantID, id pgtype.UUID) error {
		return h.Service.DeleteSubscriber(ctx, tenantID, id)
	})
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
	getByID(w, r, "campaign", func(ctx context.Context, tenantID, id pgtype.UUID) (any, error) {
		return h.Service.GetCampaign(ctx, tenantID, id)
	})
}

// CreateCampaign creates a new newsletter campaign.
// POST /admin/newsletter/campaigns.
func (h *Handlers) CreateCampaign(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	u := auth.FromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var input CreateCampaignInput
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

	if input.Segment == "" {
		input.Segment = SegmentAll
	}

	campaign, err := h.Service.CreateCampaign(r.Context(), tenantID, u.UserID, input)
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

// DeleteCampaign removes a newsletter campaign.
// DELETE /admin/newsletter/campaigns/{id}.
func (h *Handlers) DeleteCampaign(w http.ResponseWriter, r *http.Request) {
	deleteByID(w, r, "campaign", func(ctx context.Context, tenantID, id pgtype.UUID) error {
		return h.Service.DeleteCampaign(ctx, tenantID, id)
	})
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

// ---------------------------------------------------------------------------
// Handlers — Unsubscribe, preferences, stats
// ---------------------------------------------------------------------------

// HandleUnsubscribe handles one-click unsubscribe via token or email.
// DELETE /newsletter/unsubscribe
func (h *Handlers) HandleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	token := r.URL.Query().Get("token")
	if token != "" {
		if err := h.Service.UnsubscribeByToken(r.Context(), tenantID, token); err != nil {
			writeError(w, http.StatusBadRequest, "unsubscribe_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"unsubscribed": true}})
		return
	}

	email := r.URL.Query().Get("email")
	if email == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "token or email is required")
		return
	}

	if err := h.Service.Unsubscribe(r.Context(), tenantID, email); err != nil {
		writeError(w, http.StatusBadRequest, "unsubscribe_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"unsubscribed": true}})
}

// HandleGetPreferences returns subscriber preferences.
// GET /newsletter/preferences
func (h *Handlers) HandleGetPreferences(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	u := auth.FromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	subscriber, err := h.Service.GetPreferencesByUserID(r.Context(), tenantID, u.UserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "No subscription found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": subscriber})
}

// HandleUpdatePreferences updates subscriber preferences.
// PATCH /newsletter/preferences
func (h *Handlers) HandleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	u := auth.FromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	subscriber, err := h.Service.GetPreferencesByUserID(r.Context(), tenantID, u.UserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "No subscription found")
		return
	}

	var body struct {
		DigestEnabled   *bool `json:"digest_enabled"`
		CampaignEnabled *bool `json:"campaign_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	updated, err := h.Service.UpdatePreferences(r.Context(), tenantID, subscriber.ID, body.DigestEnabled, body.CampaignEnabled)
	if err != nil {
		writeError(w, http.StatusBadRequest, "update_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

// HandleAdminStats returns newsletter stats.
// GET /admin/newsletter/stats
func (h *Handlers) HandleAdminStats(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	if auth.FromContext(r.Context()) == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	stats, err := h.Service.AdminSubscriberStats(r.Context(), tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "stats_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"stats": stats}})
}

// HandleAdminSendCampaign sends a campaign immediately.
// POST /admin/newsletter/campaigns/{id}/send-v2
func (h *Handlers) HandleAdminSendCampaign(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	if auth.FromContext(r.Context()) == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid campaign ID")
		return
	}

	if err := h.Service.SendCampaign(r.Context(), tenantID, id); err != nil {
		writeError(w, http.StatusBadRequest, "send_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"data": map[string]bool{"sent": true}})
}

// HandleAdminUpdateCampaign updates a campaign.
// PATCH /admin/newsletter/campaigns/{id}/v2
func (h *Handlers) HandleAdminUpdateCampaign(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	if auth.FromContext(r.Context()) == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	campaignID, ok := requireID(w, r, "id", "campaign")
	if !ok {
		return
	}

	var body struct {
		Name        string          `json:"name"`
		Subject     string          `json:"subject"`
		PreviewText string          `json:"preview_text"`
		Body        json.RawMessage `json:"body"`
		Segment     string          `json:"segment"`
		SegmentID   string          `json:"segment_id"`
		Status      string          `json:"status"`
		ScheduledAt *time.Time      `json:"scheduled_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	var name, subject, previewText, segment, status *string
	if body.Name != "" {
		name = &body.Name
	}
	if body.Subject != "" {
		subject = &body.Subject
	}
	if body.PreviewText != "" {
		previewText = &body.PreviewText
	}
	if body.Segment != "" {
		segment = &body.Segment
	}
	if body.Status != "" {
		status = &body.Status
	}

	var segmentID pgtype.UUID
	if body.SegmentID != "" {
		_ = segmentID.Scan(body.SegmentID)
	}

	var scheduledAt pgtype.Timestamptz
	if body.ScheduledAt != nil {
		scheduledAt = pgtype.Timestamptz{Time: *body.ScheduledAt, Valid: true}
	}

	campaign, err := h.Service.queries.UpdateCampaign(r.Context(), gen.UpdateCampaignParams{
		TenantID:    tenantID,
		ID:          campaignID,
		Name:        name,
		Subject:     subject,
		PreviewText: previewText,
		Body:        body.Body,
		Segment:     segment,
		SegmentID:   segmentID,
		Status:      status,
		ScheduledAt: scheduledAt,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "update_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": campaign})
}
