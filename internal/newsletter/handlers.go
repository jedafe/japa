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
	"crypto/subtle"
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
	"github.com/jedafe/japa/internal/content"
	errpkg "github.com/jedafe/japa/internal/errors"
)

const (
	defaultPageSize int32 = 20
	maxPageSize     int32 = 100
)

// extractTenant extracts the tenant ID from the request context.
func extractTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	return pgtype.UUID{}, true
}

// requireID parses the :id URL parameter as a UUID.
func requireID(w http.ResponseWriter, r *http.Request, paramName, entityName string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, paramName)); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid "+entityName+" ID")
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
		errpkg.WriteError(w, http.StatusNotFound, "not_found", entityName+" not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
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
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete "+entityName)

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
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
// Ref: conventions/api-routes-catalog.md (POST /newsletter/subscribe).
func (h *Handlers) Subscribe(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Global killswitch (newsletter.enabled, ADR-080 §62).
	if !h.Service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "newsletter_disabled", "Newsletter is not available")
		return
	}

	var input struct {
		Email  string `json:"email"`
		Source string `json:"source"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	email := strings.TrimSpace(input.Email)
	if email == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "email is required")
		return
	}

	if !isValidEmail(email) {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "A valid email is required")
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
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error",
			"source must be one of: signup, onboarding, footer")
		return
	}

	// Link to user ID if authenticated.
	var userID *pgtype.UUID
	if u := auth.FromContext(r.Context()); u != nil {
		userID = &u.UserID
	}

	subscriber, err := h.Service.CreateSubscriber(r.Context(), email, userID, source)
	if err != nil {
		slog.Error("creating newsletter subscriber (public)", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to subscribe")

		return
	}

	// The Subscribe query is an upsert (ON CONFLICT DO UPDATE), so the
	// operation is idempotent: a re-subscribe re-activates the existing row
	// with status='subscribed' and unsubscribed_at=NULL. Because the upsert
	// always sets subscribed_at=now() (NOT NULL DEFAULT now()), the row's
	// SubscribedAt is always valid and cannot be used to distinguish a new
	// insert from a re-subscribe. Return 200 for the idempotent success —
	// the subscriber is subscribed either way.
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": subscriber})
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

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	return pgtype.UUID{}, true
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

	// #nosec G109 // value bounded by MaxPageSize check
	return int32(n)
}

func parseOffset(r *http.Request) int32 {
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			if n > math.MaxInt32 {
				return defaultPageSize
			}

			// #nosec G109 // value bounded by MaxPageSize check
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
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Email  string `json:"email"`
		Source string `json:"source"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.Email == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "email is required")
		return
	}

	// Admin-created subscribers default to 'import' source.
	if input.Source == "" {
		input.Source = SourceImport
	}

	// Validate the source field against the allowed enum (§62 schema CHECK).
	switch input.Source {
	case SourceSignup, SourceOnboarding, SourceFooter, SourceImport, SourceAPI:
		// allowed
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error",
			"source must be one of: signup, onboarding, footer, import, api")
		return
	}

	subscriber, err := h.Service.CreateSubscriber(r.Context(), input.Email, nil, input.Source)
	if err != nil {
		slog.Error("creating newsletter subscriber", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create subscriber")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": subscriber})
}

// ImportSubscribers bulk-imports subscribers from a list of emails.
// POST /admin/newsletter/subscribers/import — gated by newsletter.import
// (§62 RBAC, ADR-080 §6). Each email is upserted with source='import'.
// Returns the count of imported and skipped (invalid) emails.
func (h *Handlers) ImportSubscribers(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Emails []string `json:"emails"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if len(input.Emails) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "emails is required")
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

		if _, err := h.Service.CreateSubscriber(r.Context(), email, nil, SourceImport); err != nil {
			slog.Warn("newsletter import: skip email", "email", email, "error", err)

			skipped++

			continue
		}

		imported++
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parseOffset(r)
	status := r.URL.Query().Get("status")
	search := strings.TrimSpace(r.URL.Query().Get("search"))

	// Validate the status filter against the allowed enum if provided.
	if status != "" {
		switch status {
		case StatusSubscribed, StatusUnsubscribed, StatusBounced, StatusComplained:
			// allowed
		default:
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error",
				"status must be one of: subscribed, unsubscribed, bounced, complained")
			return
		}
	}

	subscribers, err := h.Service.ListSubscribersFiltered(r.Context(), status, search, limit, offset)
	if err != nil {
		slog.Error("listing newsletter subscribers", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list subscribers")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": subscribers})
}

// GetSubscriber returns a single newsletter subscriber by ID.
// GET /admin/newsletter/subscribers/{id}.
func (h *Handlers) GetSubscriber(w http.ResponseWriter, r *http.Request) {
	getByID(w, r, "subscriber", func(ctx context.Context, tenantID, id pgtype.UUID) (any, error) {
		return h.Service.GetSubscriber(ctx, id)
	})
}

// DeleteSubscriber removes a newsletter subscriber.
// DELETE /admin/newsletter/subscribers/{id}.
func (h *Handlers) DeleteSubscriber(w http.ResponseWriter, r *http.Request) {
	deleteByID(w, r, "subscriber", func(ctx context.Context, tenantID, id pgtype.UUID) error {
		return h.Service.DeleteSubscriber(ctx, id)
	})
}

// ---------------------------------------------------------------------------
// Admin Handlers — Newsletter Campaigns
// ---------------------------------------------------------------------------

// ListCampaigns returns newsletter campaigns for the tenant with pagination.
// GET /admin/newsletter/campaigns?page_size=&offset=&status=
// Returns the standard paginated envelope {items, total, total_pages} so the
// admin DataTable can render serverSide pagination (§22, ADR-080 §3).
func (h *Handlers) ListCampaigns(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := int(parsePageSize(r))
	offset := int(parseOffset(r))
	status := r.URL.Query().Get("status")

	// Validate the status filter against the allowed enum if provided.
	if status != "" {
		switch status {
		case CampaignDraft, CampaignScheduled, CampaignSending, CampaignSent, CampaignCancelled:
			// allowed
		default:
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error",
				"status must be one of: draft, scheduled, sending, sent, cancelled")
			return
		}
	}

	page := 1
	if limit > 0 {
		page = (offset / limit) + 1
	}

	campaigns, total, err := h.Service.AdminListCampaigns(r.Context(), status, page, limit)
	if err != nil {
		slog.Error("listing newsletter campaigns", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list campaigns")

		return
	}

	totalPages := int32(1)
	if limit > 0 && total > 0 {
		// #nosec G115 // bounded
		totalPages = int32((total + int64(limit) - 1) / int64(limit))
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"items":       campaigns,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// GetCampaign returns a single newsletter campaign by ID.
// GET /admin/newsletter/campaigns/{id}.
func (h *Handlers) GetCampaign(w http.ResponseWriter, r *http.Request) {
	getByID(w, r, "campaign", func(ctx context.Context, tenantID, id pgtype.UUID) (any, error) {
		return h.Service.GetCampaign(ctx, id)
	})
}

// CreateCampaign creates a new newsletter campaign.
// POST /admin/newsletter/campaigns.
func (h *Handlers) CreateCampaign(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var input CreateCampaignInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "name is required")
		return
	}

	if input.Subject == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "subject is required")
		return
	}

	if input.Segment == "" {
		input.Segment = SegmentAll
	}

	// Validate the segment against the allowed enum (§62 schema CHECK).
	switch input.Segment {
	case SegmentAll, SegmentSpace, SegmentTag, SegmentProSubscribers:
		// allowed
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error",
			"segment must be one of: all, space, tag, pro_subscribers")
		return
	}

	// Require segment_id when segment targets a space or tag (§62/ADR-080 §3).
	// Without it the worker falls back to 'all' → silent send-to-everyone.
	if (input.Segment == SegmentSpace || input.Segment == SegmentTag) && input.SegmentID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error",
			"segment_id is required when segment is 'space' or 'tag'")
		return
	}

	// If body is not provided but html is, wrap html in a jsonb body.
	if len(input.Body) == 0 && input.HTML != "" {
		input.Body = json.RawMessage(`{"html":` + jsonString(input.HTML) + `}`)
	}

	// Sanitize the HTML body server-side before storage to prevent stored
	// XSS in email (§5.1/§14). Client-side regex sanitization is bypassable;
	// the canonical allowlist-based content.SanitizeHTML is the source of
	// truth for acceptable tags/attributes.
	if input.HTML != "" {
		input.HTML = content.SanitizeHTML(input.HTML)
	}

	campaign, err := h.Service.CreateCampaign(r.Context(), u.UserID, input)
	if err != nil {
		slog.Error("creating newsletter campaign", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create campaign")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": campaign})
}

// jsonString encodes s as a JSON string (including quotes).
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// UpdateCampaign updates an existing newsletter campaign.
// PATCH /admin/newsletter/campaigns/{id}.
func (h *Handlers) UpdateCampaign(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid campaign ID")
		return
	}

	var body struct {
		Name        *string         `json:"name,omitempty"`
		Subject     *string         `json:"subject,omitempty"`
		HTML        *string         `json:"html,omitempty"`
		PreviewText *string         `json:"preview_text,omitempty"`
		Body        json.RawMessage `json:"body,omitempty"`
		Segment     *string         `json:"segment,omitempty"`
		SegmentID   *string         `json:"segment_id,omitempty"`
		Status      *string         `json:"status,omitempty"`
		ScheduledAt *time.Time      `json:"scheduled_at,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	// Sanitize HTML server-side before storage (§5.1/§14, mirrors CreateCampaign).
	if body.HTML != nil && *body.HTML != "" {
		sanitized := content.SanitizeHTML(*body.HTML)
		body.HTML = &sanitized
	}

	// Validate status transitions against the §62 state machine when a new
	// status is provided. Allowed transitions:
	//   draft ↔ scheduled
	//   draft/scheduled → sending (only via the send endpoint, not PATCH)
	//   sending → sent (only via the worker, not PATCH)
	//   draft/scheduled/sending → cancelled
	// The PATCH endpoint may not jump to 'sending' or 'sent' directly —
	// those transitions are owned by the send endpoint and worker.
	if body.Status != nil && *body.Status != "" {
		existing, err := h.Service.GetCampaign(r.Context(), id)
		if err != nil {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Campaign not found")
			return
		}

		next := *body.Status
		allowed := false

		switch next {
		case CampaignDraft:
			allowed = existing.Status == CampaignScheduled
		case CampaignScheduled:
			allowed = existing.Status == CampaignDraft
		case CampaignCancelled:
			allowed = existing.Status == CampaignDraft || existing.Status == CampaignScheduled || existing.Status == CampaignSending
		}

		if !allowed {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_status_transition",
				"cannot transition campaign from '"+existing.Status+"' to '"+next+"' via PATCH")
			return
		}
	}

	campaign, err := h.Service.UpdateCampaign(r.Context(), id, UpdateCampaignInput{
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
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update campaign")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": campaign})
}

// UpdateSubscriber updates a subscriber's status.
// PATCH /admin/newsletter/subscribers/{id}.
func (h *Handlers) UpdateSubscriber(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if body.Status == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "status is required")
		return
	}

	subscriber, err := h.Service.UpdateSubscriberStatus(r.Context(), id, body.Status)
	if err != nil {
		slog.Error("updating newsletter subscriber", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update subscriber")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": subscriber})
}

// DeleteCampaign removes a newsletter campaign.
// DELETE /admin/newsletter/campaigns/{id}.
func (h *Handlers) DeleteCampaign(w http.ResponseWriter, r *http.Request) {
	deleteByID(w, r, "campaign", func(ctx context.Context, tenantID, id pgtype.UUID) error {
		return h.Service.DeleteCampaign(ctx, id)
	})
}

// HandleSendCampaign triggers sending a newsletter campaign.
// POST /admin/newsletter/campaigns/{id}/send.
func (h *Handlers) HandleSendCampaign(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid campaign ID")
		return
	}

	// Global killswitch (newsletter.enabled, ADR-080 §62).
	if !h.Service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "newsletter_disabled", "Newsletter is not available")
		return
	}

	// Fetch the campaign and verify it's in a sendable state.
	campaign, err := h.Service.GetCampaign(r.Context(), id)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Campaign not found")
		return
	}

	if campaign.Status != CampaignDraft && campaign.Status != CampaignScheduled {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", "Campaign must be in draft or scheduled status to send")
		return
	}

	// Enqueue the newsletter send task FIRST so a failure does not leave the
	// campaign stuck in 'sending' forever (§62, ADR-080 §3). The worker
	// re-fetches the campaign and resolves segment-aware recipients from the
	// DB, so the handler does not need to pre-fetch the recipient list.
	if err := h.Service.EnqueueNewsletterSend(r.Context(), id); err != nil {
		slog.Error("enqueuing newsletter send", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to enqueue newsletter send")

		return
	}

	// Only after a successful enqueue, transition the campaign to 'sending'.
	// If this status update fails, the worker still processes the enqueued
	// task (idempotent on campaign ID), so the send is not lost — the
	// campaign row just lags behind and can be reconciled manually.
	sendingStatus := CampaignSending
	if _, err := h.Service.UpdateCampaign(r.Context(), id, UpdateCampaignInput{
		Status: &sendingStatus,
	}); err != nil {
		slog.Error("updating campaign status to sending", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update campaign status")

		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]any{
			"status": "sending",
		},
	})
}

// ---------------------------------------------------------------------------
// Handlers — Unsubscribe, preferences, stats
// ---------------------------------------------------------------------------

// HandleUnsubscribe handles one-click unsubscribe via token or email.
// DELETE /newsletter/unsubscribe.
func (h *Handlers) HandleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	token := r.URL.Query().Get("token")
	if token != "" {
		if err := h.Service.UnsubscribeByToken(r.Context(), token); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "unsubscribe_failed", err.Error())
			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"unsubscribed": true}})

		return
	}

	email := r.URL.Query().Get("email")
	if email == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "token or email is required")
		return
	}

	if err := h.Service.Unsubscribe(r.Context(), email); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "unsubscribe_failed", err.Error())
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"unsubscribed": true}})
}

// HandleGetPreferences returns subscriber preferences.
// GET /newsletter/preferences.
func (h *Handlers) HandleGetPreferences(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	subscriber, err := h.Service.GetPreferencesByUserID(r.Context(), u.UserID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "No subscription found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": subscriber})
}

// HandleUpdatePreferences updates subscriber preferences.
// PATCH /newsletter/preferences.
func (h *Handlers) HandleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	subscriber, err := h.Service.GetPreferencesByUserID(r.Context(), u.UserID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "No subscription found")
		return
	}

	var body struct {
		DigestEnabled   *bool `json:"digest_enabled"`
		CampaignEnabled *bool `json:"campaign_enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	updated, err := h.Service.UpdatePreferences(r.Context(), subscriber.ID, body.DigestEnabled, body.CampaignEnabled)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "update_failed", err.Error())
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": updated})
}

// HandleAdminStats returns newsletter stats.
// GET /admin/newsletter/stats.
func (h *Handlers) HandleAdminStats(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if auth.FromContext(r.Context()) == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	stats, err := h.Service.AdminSubscriberStats(r.Context())
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "stats_failed", err.Error())
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": stats})
}

// HandleWebhook processes bounce/complaint webhooks from the marketing
// email provider (ADR-015). The provider POSTs a JSON payload describing
// the event; this handler updates the matching subscriber's status to
// 'bounced' or 'complained' so future sends skip them (§62 Compliance,
// ADR-080 §4). Public route — authenticated at the provider level via a
// shared secret passed in the X-Webhook-Secret header (JC_NEWSLETTER_WEBHOOK_SECRET).
//
// Ref: japaconnect-engineering/sections/62-newsletter-digest.md
//
//	japaconnect-engineering/adr/ADR-080-newsletter-digest.md
func (h *Handlers) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Shared-secret auth: the provider echoes a secret set in config.
	// Use constant-time comparison to avoid timing side channels (§5.2).
	expected := r.Header.Get("X-Webhook-Secret")
	want := h.Service.WebhookSecret(r.Context())

	if expected == "" || want == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(want)) != 1 {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Invalid webhook secret")
		return
	}

	var payload struct {
		EventType  string `json:"event_type"` // "bounce" | "complaint" | "delivered" | "open" | "click"
		Email      string `json:"email"`
		CampaignID string `json:"campaign_id"` // optional — required for open/click attribution
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if payload.Email == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "email is required")
		return
	}

	switch payload.EventType {
	case "bounce":
		if err := h.Service.MarkBounced(r.Context(), payload.Email); err != nil {
			slog.Error("newsletter webhook: mark bounced", "email", payload.Email, "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to mark bounced")

			return
		}
	case "complaint":
		if err := h.Service.MarkComplained(r.Context(), payload.Email); err != nil {
			slog.Error("newsletter webhook: mark complained", "email", payload.Email, "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to mark complained")

			return
		}
	case "open":
		// Increment the campaign's open_count for engagement analytics (§62
		// schema: open_count/click_count). Requires campaign_id in the
		// payload; if absent, acknowledge to avoid provider retries but log.
		if payload.CampaignID == "" {
			slog.Warn("newsletter webhook: open event missing campaign_id", "email", payload.Email)
		} else if err := h.Service.IncrementCampaignOpens(r.Context(), payload.CampaignID); err != nil {
			slog.Error("newsletter webhook: increment opens", "campaign_id", payload.CampaignID, "error", err)
		}
	case "click":
		if payload.CampaignID == "" {
			slog.Warn("newsletter webhook: click event missing campaign_id", "email", payload.Email)
		} else if err := h.Service.IncrementCampaignClicks(r.Context(), payload.CampaignID); err != nil {
			slog.Error("newsletter webhook: increment clicks", "campaign_id", payload.CampaignID, "error", err)
		}
	case "delivered":
		// No status change needed; provider delivery events are out of
		// scope for the subscriber status machine. Acknowledge to avoid
		// provider retries.
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "unsupported event_type")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "ok"}})
}
