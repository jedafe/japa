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

	// Validate the source field. The public endpoint may only use the
	// public source values ('signup', 'onboarding', 'footer'); the admin-
	// only sources ('import', 'api') are rejected to prevent spoofing
	// admin/api-originated subscriptions. Defaults to 'signup' when empty.
	// Ref: japaconnect-engineering/sections/62-newsletter-digest.md (source CHECK)
	source := input.Source
	if source == "" {
		source = SourceSignup
	}
	switch source {
	case SourceSignup, SourceOnboarding, SourceFooter:
		// allowed
	default:
		writeError(w, http.StatusBadRequest, "validation_error",
			"source must be one of: signup, onboarding, footer")
		return
	}

	// Link to user ID if authenticated.
	var userID *pgtype.UUID
	if u := auth.FromContext(r.Context()); u != nil {
		userID = &u.UserID
	}

	subscriber, err := h.Service.CreateSubscriber(r.Context(), tenantID, email, userID, source)
	if err != nil {
		slog.Error("creating newsletter subscriber (public)", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to subscribe")
		return
	}

	// The Subscribe query is an upsert (ON CONFLICT DO UPDATE), so a
	// re-subscribe for an existing email returns the re-activated row with
	// status='subscribed' rather than a duplicate error. Return 200 for
	// re-subscribes (existing row) and 201 for new subscribers.
	status := http.StatusCreated
	if subscriber.UnsubscribedAt.Valid || subscriber.SubscribedAt.Valid {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"data": subscriber})
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

// ImportSubscribers bulk-imports subscribers from a list of emails.
// POST /admin/newsletter/subscribers/import — gated by newsletter.import
// (§62 RBAC, ADR-080 §6). Each email is upserted with source='import'.
// Returns the count of imported and skipped (invalid) emails.
func (h *Handlers) ImportSubscribers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Emails []string `json:"emails"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if len(input.Emails) == 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "emails is required")
		return
	}

	imported := 0
	skipped := 0
	for _, raw := range input.Emails {
		email := strings.TrimSpace(raw)
		if email == "" || !isValidEmail(email) {
			skipped++
			continue
		}
		if _, err := h.Service.CreateSubscriber(r.Context(), tenantID, email, nil, SourceImport); err != nil {
			slog.Warn("newsletter import: skip email", "email", email, "error", err)
			skipped++
			continue
		}
		imported++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]int{
			"imported": imported,
			"skipped":  skipped,
		},
	})
}

// ListSubscribers returns all newsletter subscribers for the tenant,
// optionally filtered by ?status= (subscribed|unsubscribed|bounced|complained).
// GET /admin/newsletter/subscribers.
func (h *Handlers) ListSubscribers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parseOffset(r)
	status := r.URL.Query().Get("status")

	// Validate the status filter against the allowed enum if provided.
	if status != "" {
		switch status {
		case StatusSubscribed, StatusUnsubscribed, StatusBounced, StatusComplained:
			// allowed
		default:
			writeError(w, http.StatusBadRequest, "validation_error",
				"status must be one of: subscribed, unsubscribed, bounced, complained")
			return
		}
	}

	subscribers, err := h.Service.ListSubscribersFiltered(r.Context(), tenantID, status, limit, offset)
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

	// If body is not provided but html is, wrap html in a jsonb body.
	if len(input.Body) == 0 && input.HTML != "" {
		input.Body = json.RawMessage(`{"html":` + jsonString(input.HTML) + `}`)
	}

	campaign, err := h.Service.CreateCampaign(r.Context(), tenantID, u.UserID, input)
	if err != nil {
		slog.Error("creating newsletter campaign", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create campaign")

		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": campaign})
}

// jsonString encodes s as a JSON string (including quotes).
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
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

	var body struct {
		Name        *string          `json:"name,omitempty"`
		Subject     *string          `json:"subject,omitempty"`
		HTML        *string          `json:"html,omitempty"`
		PreviewText *string          `json:"preview_text,omitempty"`
		Body        json.RawMessage  `json:"body,omitempty"`
		Segment     *string          `json:"segment,omitempty"`
		SegmentID   *string          `json:"segment_id,omitempty"`
		Status      *string          `json:"status,omitempty"`
		ScheduledAt *time.Time       `json:"scheduled_at,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	campaign, err := h.Service.UpdateCampaign(r.Context(), tenantID, id, UpdateCampaignInput{
		Name:        body.Name,
		Subject:     body.Subject,
		HTML:        body.HTML,
		PreviewText: body.PreviewText,
		Body:        body.Body,
		Segment:     body.Segment,
		SegmentID:   body.SegmentID,
		Status:      body.Status,
		ScheduledAt: body.ScheduledAt,
	})
	if err != nil {
		slog.Error("updating newsletter campaign", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update campaign")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": campaign})
}

// UpdateSubscriber updates a subscriber's status.
// PATCH /admin/newsletter/subscribers/{id}.
func (h *Handlers) UpdateSubscriber(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, ok := requireID(w, r, "id", "subscriber")
	if !ok {
		return
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if body.Status == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "status is required")
		return
	}

	subscriber, err := h.Service.UpdateSubscriberStatus(r.Context(), tenantID, id, body.Status)
	if err != nil {
		slog.Error("updating newsletter subscriber", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update subscriber")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": subscriber})
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

	if campaign.Status != CampaignDraft && campaign.Status != CampaignScheduled {
		writeError(w, http.StatusBadRequest, "invalid_status", "Campaign must be in draft or scheduled status to send")
		return
	}

	// Update campaign status to 'sending'.
	sendingStatus := CampaignSending
	if _, err := h.Service.UpdateCampaign(r.Context(), tenantID, id, UpdateCampaignInput{
		Status: &sendingStatus,
	}); err != nil {
		slog.Error("updating campaign status to sending", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update campaign status")

		return
	}

	// Enqueue the newsletter send task. The worker re-fetches the campaign
	// and resolves segment-aware recipients from the DB (§62, ADR-080 §3),
	// so the handler does not need to pre-fetch the recipient list.
	if err := h.Service.EnqueueNewsletterSend(r.Context(), id, tenantID); err != nil {
		slog.Error("enqueuing newsletter send", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to enqueue newsletter send")

		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]any{
			"status": "sending",
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
	writeJSON(w, http.StatusOK, map[string]any{"data": stats})
}

// HandleWebhook processes bounce/complaint webhooks from the marketing
// email provider (ADR-015). The provider POSTs a JSON payload describing
// the event; this handler updates the matching subscriber's status to
// 'bounced' or 'complained' so future sends skip them (§62 Compliance,
// ADR-080 §4). Public route — authenticated at the provider level via a
// shared secret passed in the X-Webhook-Secret header (JC_NEWSLETTER_WEBHOOK_SECRET).
//
// Ref: japaconnect-engineering/sections/62-newsletter-digest.md
//      japaconnect-engineering/adr/ADR-080-newsletter-digest.md
func (h *Handlers) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Shared-secret auth: the provider echoes a secret set in config.
	expected := r.Header.Get("X-Webhook-Secret")
	if expected == "" || expected != h.Service.WebhookSecret(r.Context()) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid webhook secret")
		return
	}

	var payload struct {
		EventType string `json:"event_type"` // "bounce" | "complaint" | "delivered" | "open" | "click"
		Email     string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if payload.Email == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "email is required")
		return
	}

	switch payload.EventType {
	case "bounce":
		if err := h.Service.MarkBounced(r.Context(), tenantID, payload.Email); err != nil {
			slog.Error("newsletter webhook: mark bounced", "email", payload.Email, "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to mark bounced")
			return
		}
	case "complaint":
		if err := h.Service.MarkComplained(r.Context(), tenantID, payload.Email); err != nil {
			slog.Error("newsletter webhook: mark complained", "email", payload.Email, "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to mark complained")
			return
		}
	case "delivered", "open", "click":
		// No status change needed; provider engagement events are out of
		// scope for the subscriber status machine. Acknowledge to avoid
		// provider retries.
	default:
		writeError(w, http.StatusBadRequest, "validation_error", "unsupported event_type")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "ok"}})
}
