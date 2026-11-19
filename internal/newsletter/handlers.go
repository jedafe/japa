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
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/email"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
)

const (
	defaultPageSize int32 = 20
	maxPageSize     int32 = 100
)

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
func getByID[T any](w http.ResponseWriter, r *http.Request, entityName string, fetchFn func(ctx context.Context, id pgtype.UUID) (T, error)) {
	id, ok := requireID(w, r, "id", entityName)
	if !ok {
		return
	}

	result, err := fetchFn(r.Context(), id)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", entityName+" not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// deleteByID is a shared helper for DELETE /{entity}/{id} handlers.
func deleteByID(w http.ResponseWriter, r *http.Request, entityName string, deleteFn func(ctx context.Context, id pgtype.UUID) error) {
	id, ok := requireID(w, r, "id", entityName)
	if !ok {
		return
	}

	if err := deleteFn(r.Context(), id); err != nil {
		slog.Error("deleting newsletter "+entityName, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete "+entityName)

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// Handlers holds dependencies for newsletter admin HTTP handlers.
type Handlers struct {
	Service *Service
	lit     literalResolver // nil disables literal resolution (§14)
}

// literalResolver resolves {{...}} template literals in user-facing text
// fields (§14). Implemented by *literals.Catalog; nil disables resolution.
type literalResolver interface {
	ResolveFields(ctx context.Context, v any, rc literals.RenderContext)
}

// SetLiteralResolver injects the template-literal catalog so user-facing
// titles/descriptions render {{...}} placeholders (§14).
func (h *Handlers) SetLiteralResolver(l literalResolver) { h.lit = l }

// resolveLiterals resolves {{...}} placeholders on a response DTO or slice.
// Best-effort — failures are logged inside the resolver.
func (h *Handlers) resolveLiterals(r *http.Request, v any) {
	if literals.IsRawRequest(r) {
		return
	}

	if h.lit == nil {
		return
	}

	h.lit.ResolveFields(r.Context(), v, literals.NewRequestContext(r, ""))
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

	// Double opt-in (migration 001170): a 'pending' row must not receive any
	// mail until the owner confirms — mint a purpose='confirm' token and
	// enqueue the confirmation email. Failures degrade to logging: the row
	// exists and re-subscribing re-triggers the flow. ReserveConfirmSend
	// caps one confirm email per address per hour so repeated sign-ups for
	// a victim's address can't mail-bomb them; the earlier token stays
	// valid, so a suppressed resend is still confirmable.
	if subscriber.Status == StatusPending && h.Service.ReserveConfirmSend(r.Context(), email) {
		if tok, terr := h.Service.CreateConfirmToken(r.Context(), subscriber.ID); terr == nil {
			if eerr := h.Service.EnqueueConfirmEmail(r.Context(), email, tok); eerr != nil {
				slog.Warn("newsletter: enqueue confirm email failed", "error", eerr)
				h.Service.ReleaseConfirmSend(r.Context(), email)
			}
		} else {
			slog.Warn("newsletter: create confirm token failed", "error", terr)
			h.Service.ReleaseConfirmSend(r.Context(), email)
		}
	}

	// Anti-enumeration: the response is identical whether the address was
	// new, already pending, or already subscribed — reflecting the stored
	// row (status/id/user_id linkage) to an unauthenticated caller would
	// leak who is on the list and whether an email maps to an account.
	// double_opt_in is site configuration, not per-email data: the
	// frontend uses it to pick "check your inbox" vs "subscribed" copy,
	// and callers that need confirmation messaging get the same answer
	// for every address (§62 compliance).
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"status":        "ok",
		"double_opt_in": h.Service.DoubleOptIn(r.Context()),
	}})
}

// HandleConfirm handles GET /newsletter/confirm — double opt-in
// confirmation (migration 001170). Single-use purpose='confirm' token;
// 404 when the token is unknown, wrong-purpose, or the subscriber is no
// longer pending (already confirmed / re-subscribed through another path).
func (h *Handlers) HandleConfirm(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "token is required")
		return
	}

	subscriber, err := h.Service.ConfirmByToken(r.Context(), token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "invalid_token", "Invalid or expired confirmation link")
			return
		}
		slog.Error("newsletter: confirm_failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "The server encountered an error — please try again")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"status":    subscriber.Status,
		"confirmed": true,
	}})
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

	// Direct insert — double opt-in applies to public signups only; admin
	// rows send no confirmation email and would be stuck 'pending'.
	subscriber, err := h.Service.CreateSubscriberDirect(r.Context(), input.Email, nil, input.Source)
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

		if _, err := h.Service.CreateSubscriberDirect(r.Context(), email, nil, SourceImport); err != nil {
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

// ListSubscribers returns all newsletter subscribers for the platform,
// optionally filtered by ?status= (subscribed|unsubscribed|bounced|complained).
// GET /admin/newsletter/subscribers.
func (h *Handlers) ListSubscribers(w http.ResponseWriter, r *http.Request) {
	limit := parsePageSize(r)
	offset := parseOffset(r)
	status := r.URL.Query().Get("status")
	search := strings.TrimSpace(r.URL.Query().Get("search"))

	// Validate the status filter against the allowed enum if provided.
	if status != "" {
		switch status {
		case StatusPending, StatusSubscribed, StatusUnsubscribed, StatusBounced, StatusComplained:
			// allowed
		default:
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error",
				"status must be one of: pending, subscribed, unsubscribed, bounced, complained")
			return
		}
	}

	// Standard paginated envelope {items,total,total_pages} — matches the
	// campaigns endpoint so the admin DataTable can run serverSide
	// pagination/filtering (§22). page_size/offset params → 1-based page.
	page := int(offset/limit) + 1
	subscribers, total, err := h.Service.AdminListSubscribers(r.Context(), status, search, page, int(limit))
	if err != nil {
		slog.Error("listing newsletter subscribers", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list subscribers")

		return
	}

	totalPages := int((total + int64(limit) - 1) / int64(limit))
	if totalPages < 1 {
		totalPages = 1
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"items":       subscribers,
		"total":       total,
		"total_pages": totalPages,
	}})
}

// GetSubscriber returns a single newsletter subscriber by ID.
// GET /admin/newsletter/subscribers/{id}.
func (h *Handlers) GetSubscriber(w http.ResponseWriter, r *http.Request) {
	getByID(w, r, "subscriber", func(ctx context.Context, id pgtype.UUID) (any, error) {
		return h.Service.GetSubscriber(ctx, id)
	})
}

// DeleteSubscriber removes a newsletter subscriber.
// DELETE /admin/newsletter/subscribers/{id}.
func (h *Handlers) DeleteSubscriber(w http.ResponseWriter, r *http.Request) {
	deleteByID(w, r, "subscriber", func(ctx context.Context, id pgtype.UUID) error {
		return h.Service.DeleteSubscriber(ctx, id)
	})
}

// ---------------------------------------------------------------------------
// Admin Handlers — Newsletter Campaigns
// ---------------------------------------------------------------------------

// ListCampaigns returns newsletter campaigns for the platform with pagination.
// GET /admin/newsletter/campaigns?page_size=&offset=&status=
// Returns the standard paginated envelope {items, total, total_pages} so the
// admin DataTable can render serverSide pagination (§22, ADR-080 §3).
func (h *Handlers) ListCampaigns(w http.ResponseWriter, r *http.Request) {
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
	getByID(w, r, "campaign", func(ctx context.Context, id pgtype.UUID) (any, error) {
		return h.Service.GetCampaign(ctx, id)
	})
}

// CreateCampaign creates a new newsletter campaign.
// POST /admin/newsletter/campaigns.
func (h *Handlers) CreateCampaign(w http.ResponseWriter, r *http.Request) {
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
	//   sending → draft — reset a stuck/abandoned send; the send_id lease
	//     fences the old task, which self-terminates at the next page check
	//   draft/scheduled/sending → cancelled
	// The PATCH endpoint may not jump to 'sending' or 'sent' directly —
	// those transitions are owned by the send endpoint and worker.
	// Fetch once — needed for both the transition table and the field-edit
	// guard below.
	existing, err := h.Service.GetCampaign(r.Context(), id)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Campaign not found")
		return
	}

	if body.Status != nil && *body.Status != "" {
		next := *body.Status
		allowed := false

		switch next {
		case CampaignDraft:
			allowed = existing.Status == CampaignScheduled || existing.Status == CampaignSending
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

	// Field-edit guard: content/targeting fields are only editable in
	// draft or scheduled. Rewriting subject/body/segment on a 'sending'
	// campaign has no effect on the in-flight send (the worker already
	// fetched it) but corrupts the record; editing a 'sent' or
	// 'cancelled' campaign falsifies history/analytics. The guard checks
	// the RESULTING status so a combined {status:'draft', subject:...}
	// reset-and-edit stays legal.
	resulting := existing.Status
	if body.Status != nil && *body.Status != "" {
		resulting = *body.Status
	}

	if resulting != CampaignDraft && resulting != CampaignScheduled {
		if body.Name != nil || body.Subject != nil || body.HTML != nil || body.PreviewText != nil ||
			body.Body != nil || body.Segment != nil || body.SegmentID != nil || body.ScheduledAt != nil {
			errpkg.WriteError(w, http.StatusConflict, "campaign_not_editable",
				"Campaign content is only editable while draft or scheduled")
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

	switch body.Status {
	case StatusPending, StatusSubscribed, StatusUnsubscribed, StatusBounced, StatusComplained:
		// allowed
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error",
			"status must be one of: pending, subscribed, unsubscribed, bounced, complained")
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
	deleteByID(w, r, "campaign", func(ctx context.Context, id pgtype.UUID) error {
		return h.Service.DeleteCampaign(ctx, id)
	})
}

// HandleSendCampaign triggers sending a newsletter campaign.
// POST /admin/newsletter/campaigns/{id}/send.
func (h *Handlers) HandleSendCampaign(w http.ResponseWriter, r *http.Request) {
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

	// Claim the send atomically BEFORE enqueueing (§62, ADR-080 §3): the
	// claim flips draft|scheduled→sending in one UPDATE, so a losing claim
	// means another send is in flight — a 409, not a second queued task
	// that could double-send. The winning claim also guarantees the queued
	// task finds 'sending' (no enqueue-then-flip race for the worker).
	// The generated sendID is the send's lease identity — stamped by the
	// claim and carried in the task payload, so a stale task can't mail
	// for a send that was reset or re-claimed.
	sendID, err := GenerateUnsubscribeToken()
	if err != nil {
		slog.Error("generating send id", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to initiate send")
		return
	}

	claimed, prevStatus, err := h.Service.ClaimSend(r.Context(), id, sendID)
	if err != nil {
		slog.Error("claiming newsletter send", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to claim newsletter send")
		return
	}

	if !claimed {
		errpkg.WriteError(w, http.StatusConflict, "already_sending", "Campaign is already sending or has been sent")
		return
	}

	if err := h.Service.EnqueueNewsletterSend(r.Context(), id, sendID); err != nil {
		slog.Error("enqueuing newsletter send", "error", err)
		// Undo the claim so the campaign is not stuck in 'sending' with no
		// task behind it — the admin can retry once the queue recovers.
		// prevStatus (not the earlier fetch) restores exactly what the
		// claim displaced.
		h.Service.RevertSendClaim(r.Context(), id, sendID, prevStatus)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to enqueue newsletter send")

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
	token := r.URL.Query().Get("token")
	// RFC 8058 one-click: email clients POST List-Unsubscribe=One-Click with
	// the token in the query or body — treat it identically to the token path.
	if token == "" && r.Method == http.MethodPost {
		_ = r.ParseForm()
		token = r.PostForm.Get("token")
	}

	if token != "" {
		if err := h.Service.UnsubscribeByToken(r.Context(), token); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				errpkg.WriteError(w, http.StatusNotFound, "invalid_token", "Invalid or already-used unsubscribe token")
				return
			}
			slog.Error("newsletter: unsubscribe_failed", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "The server encountered an error — please try again")
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

	// Tokenless unsubscribe is only allowed for the signed-in owner of the
	// subscription — anything else lets a third party unsubscribe arbitrary
	// emails (§62: public one-click flow is token-based). Ownership is
	// verified against the subscriber row linked to the caller's user_id.
	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "A valid unsubscribe token is required")
		return
	}

	sub, err := h.Service.GetPreferencesByUserID(r.Context(), u.UserID)
	if err != nil || !strings.EqualFold(sub.Email, email) {
		errpkg.WriteError(w, http.StatusForbidden, "forbidden", "You can only unsubscribe your own address without a token")
		return
	}

	if err := h.Service.Unsubscribe(r.Context(), email); err != nil {
		slog.Error("newsletter: unsubscribe_failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "The server encountered an error — please try again")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"unsubscribed": true}})
}

// HandleGetPreferences returns subscriber preferences.
// GET /newsletter/preferences.
func (h *Handlers) HandleGetPreferences(w http.ResponseWriter, r *http.Request) {
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

	h.resolveLiterals(r, &subscriber)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": subscriber})
}

// HandleUpdatePreferences updates subscriber preferences.
// PATCH /newsletter/preferences.
func (h *Handlers) HandleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
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
		slog.Error("newsletter: update_failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "The server encountered an error — please try again")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": updated})
}

// HandleAdminStats returns newsletter stats.
// GET /admin/newsletter/stats.
func (h *Handlers) HandleAdminStats(w http.ResponseWriter, r *http.Request) {
	if auth.FromContext(r.Context()) == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	stats, err := h.Service.AdminSubscriberStats(r.Context())
	if err != nil {
		slog.Error("newsletter: stats_failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "stats_failed", "The server encountered an error — please try again")
		return
	}

	// §62 stats contract: surface explicit totals so consumers don't sum
	// suppressed statuses (unsubscribed/bounced/complained) into the
	// "subscribers" figure.
	var total, subscribed int64
	for _, s := range stats {
		total += s.Count
		if s.Status == StatusSubscribed {
			subscribed = s.Count
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"status_counts": stats,
		"total":         total,
		"subscribed":    subscribed,
	}})
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
	// Shared-secret auth: the provider echoes a secret set in config.
	// Use constant-time comparison to avoid timing side channels (§5.2).
	expected := r.Header.Get("X-Webhook-Secret")
	want := h.Service.WebhookSecret(r.Context())

	if expected == "" || want == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(want)) != 1 {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Invalid webhook secret")
		return
	}

	// Idempotency (§62 Compliance): providers retry webhook deliveries, and a
	// compromised/duplicated event must not inflate open/click counters or
	// re-flip subscriber status. Dedupe exact-duplicate payloads for 24h via
	// jc:newsletter_webhook_evt:{sha256(body)} — a SET NX that fails means
	// "already processed" → acknowledge 200 so the provider stops retrying.
	// The claim is released on processing failure — otherwise a transient
	// error would swallow the event (the retry would read as "duplicate").
	rawBody, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	dedupeKey := ""
	releaseDedupe := func() {
		if dedupeKey != "" {
			if vk := h.Service.WebhookDedupe(); vk != nil {
				_ = vk.Del(r.Context(), dedupeKey).Err()
			}
		}
	}

	if vk := h.Service.WebhookDedupe(); vk != nil {
		sum := sha256.Sum256(rawBody)
		dedupeKey = "jc:newsletter_webhook_evt:" + hex.EncodeToString(sum[:])
		set, derr := vk.SetNX(r.Context(), dedupeKey, 1, 24*time.Hour).Result()
		if derr == nil && !set {
			errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "duplicate"}})
			return
		}
	}

	var payload struct {
		EventType  string `json:"event_type"` // "bounce" | "complaint" | "delivered" | "open" | "click"
		Email      string `json:"email"`
		CampaignID string `json:"campaign_id"` // optional — required for open/click attribution
	}

	if err := json.Unmarshal(rawBody, &payload); err != nil {
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
			releaseDedupe()
			slog.Error("newsletter webhook: mark bounced", "email", payload.Email, "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to mark bounced")

			return
		}
	case "complaint":
		if err := h.Service.MarkComplained(r.Context(), payload.Email); err != nil {
			releaseDedupe()
			slog.Error("newsletter webhook: mark complained", "email", payload.Email, "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to mark complained")

			return
		}
	case "open", "click":
		// Increment the campaign's open/click counters for engagement
		// analytics (§62 schema). campaign_id must reference an existing
		// campaign — arbitrary IDs would poison counters.
		if payload.CampaignID == "" {
			slog.Warn("newsletter webhook: "+payload.EventType+" event missing campaign_id", "email", payload.Email)
			break
		}
		var cid pgtype.UUID
		if err := cid.Scan(payload.CampaignID); err != nil {
			slog.Warn("newsletter webhook: invalid campaign_id", "campaign_id", payload.CampaignID)
			break
		}
		if _, err := h.Service.GetCampaign(r.Context(), cid); err != nil {
			slog.Warn("newsletter webhook: unknown campaign_id", "campaign_id", payload.CampaignID)
			break
		}
		var err error
		if payload.EventType == "open" {
			err = h.Service.IncrementCampaignOpens(r.Context(), payload.CampaignID)
		} else {
			err = h.Service.IncrementCampaignClicks(r.Context(), payload.CampaignID)
		}
		if err != nil {
			releaseDedupe()
			slog.Error("newsletter webhook: increment "+payload.EventType+"s", "campaign_id", payload.CampaignID, "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to record "+payload.EventType)

			return
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

// trackingPixel is the standard 1x1 transparent GIF served by the open
// tracking endpoint (§62 self-hosted transport).
var trackingPixel = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00,
	0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x21,
	0xf9, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00, 0x2c, 0x00, 0x00,
	0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02, 0x44,
	0x01, 0x00, 0x3b,
}

// HandleTrackOpen serves the 1px open-tracking pixel appended to campaign
// bodies (§62 self-hosted MTA transport — provider transports get opens
// via HandleWebhook instead). GET /newsletter/track/open?c={campaign}&e={recipient}&s={sig}.
// The HMAC signature binds scope+campaign+recipient, verified against
// JC_NEWSLETTER_TRACKING_SECRET; a valid first hit inside the dedupe
// window increments the campaign's open_count. The pixel is served even
// for invalid signatures so a broken/forged URL never renders a missing
// image in the recipient's client — it just doesn't count.
//
// Ref: japaconnect-engineering/sections/62-newsletter-digest.md
func (h *Handlers) HandleTrackOpen(w http.ResponseWriter, r *http.Request) {
	campaignID := r.URL.Query().Get("c")
	recipient := r.URL.Query().Get("e")
	sig := r.URL.Query().Get("s")

	// The e param is the opaque per-recipient tag minted at send time
	// (hex HMAC over the normalized address — §62), never the raw email.
	// Scanner/proxy User-Agents are served the pixel but not counted, so
	// mailbox-provider prefetch doesn't inflate open rates.
	if campaignID != "" && recipient != "" &&
		!IsTrackingScanner(r.UserAgent()) &&
		email.VerifyTracking(h.Service.TrackingSecret(), sig, TrackScopeOpen, campaignID, recipient) {
		var cid pgtype.UUID
		if err := cid.Scan(campaignID); err == nil {
			if _, gerr := h.Service.GetCampaign(r.Context(), cid); gerr == nil {
				if err := h.Service.RecordTrackEvent(r.Context(), TrackScopeOpen, campaignID, recipient, ""); err != nil {
					slog.Warn("newsletter track: open increment failed", "campaign_id", campaignID, "error", err)
				}
			}
		}
	}

	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(trackingPixel)
}

// HandleTrackClick redirects a wrapped campaign link to its destination
// and counts the click (§62 self-hosted MTA transport). GET
// /newsletter/track/click?c={campaign}&e={recipient}&u={url}&s={sig}.
// The HMAC signature binds scope+campaign+recipient+destination, so an
// attacker cannot turn the endpoint into an open redirect without the
// secret — invalid signatures and non-http(s) destinations are rejected
// with 400 instead of redirecting.
//
// Ref: japaconnect-engineering/sections/62-newsletter-digest.md
func (h *Handlers) HandleTrackClick(w http.ResponseWriter, r *http.Request) {
	campaignID := r.URL.Query().Get("c")
	recipient := r.URL.Query().Get("e")
	dest := r.URL.Query().Get("u")
	sig := r.URL.Query().Get("s")

	if campaignID == "" || recipient == "" || dest == "" ||
		!email.VerifyTracking(h.Service.TrackingSecret(), sig, TrackScopeClick, campaignID, recipient, dest) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_token", "Invalid or expired tracking link")
		return
	}

	u, err := url.Parse(dest)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_url", "Unsupported redirect destination")
		return
	}

	var cid pgtype.UUID
	if err := cid.Scan(campaignID); err == nil && !IsTrackingScanner(r.UserAgent()) {
		if _, gerr := h.Service.GetCampaign(r.Context(), cid); gerr == nil {
			if err := h.Service.RecordTrackEvent(r.Context(), TrackScopeClick, campaignID, recipient, dest); err != nil {
				slog.Warn("newsletter track: click increment failed", "campaign_id", campaignID, "error", err)
			}
		}
	}

	http.Redirect(w, r, dest, http.StatusFound)
}

// HandleUnsubscribeRedirect 302s to the human-facing unsubscribe page.
// The per-recipient List-Unsubscribe header URI targets THIS api route —
// mail clients doing an RFC 8058 one-click POST hit HandleUnsubscribe on
// the same path, while clients/users that GET the URI are sent to the
// frontend page (which offers the confirm UX). GET never mutates state.
//
// Ref: japaconnect-engineering/sections/62-newsletter-digest.md
func (h *Handlers) HandleUnsubscribeRedirect(w http.ResponseWriter, r *http.Request) {
	target := "/unsubscribe"
	if token := r.URL.Query().Get("token"); token != "" {
		target += "?token=" + url.QueryEscape(token)
	}

	if base := h.Service.siteBaseURL(r.Context()); base != "" {
		target = strings.TrimRight(base, "/") + target
	}

	http.Redirect(w, r, target, http.StatusFound)
}
