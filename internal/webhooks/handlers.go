// Package webhooks — handlers.go provides HTTP handlers for webhook admin CRUD.
//
// Routes:
//
//	GET    /admin/webhooks              — list all registered webhooks
//	GET    /admin/webhooks/{id}         — get webhook by ID
//	POST   /admin/webhooks              — register a new webhook endpoint
//	PUT    /admin/webhooks/{id}         — update an existing webhook endpoint
//	DELETE /admin/webhooks/{id}         — delete a webhook endpoint
//	POST   /admin/webhooks/{id}/test    — send a test event
package webhooks

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for webhook admin HTTP handlers.
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

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case contains(err.Error(), "not found"):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
	case contains(err.Error(), "already"):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	default:
		errpkg.HandleServiceError(w, err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, err
	}

	return id, nil
}

// ---------------------------------------------------------------------------
// Admin CRUD Handlers
// ---------------------------------------------------------------------------

// ListWebhooks returns all webhook endpoints for the tenant.
// GET /admin/webhooks.
func (h *Handlers) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	endpoints, err := h.Service.ListEndpoints(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":        endpoints,
		"total_count": len(endpoints),
	})
}

// GetWebhook returns a single webhook endpoint by ID.
// GET /admin/webhooks/{id}.
func (h *Handlers) GetWebhook(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid webhook ID")
		return
	}

	endpoint, err := h.Service.GetEndpoint(r.Context(), tenantID, id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": endpoint})
}

// CreateWebhook registers a new webhook endpoint.
// POST /admin/webhooks.
func (h *Handlers) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input CreateWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.URL == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "url is required")
		return
	}

	if input.Secret == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "secret is required")
		return
	}

	if len(input.EventTypes) == 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "event_types is required and must not be empty")
		return
	}

	endpoint, err := h.Service.CreateEndpoint(r.Context(), tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": endpoint})
}

// UpdateWebhook updates an existing webhook endpoint.
// PUT /admin/webhooks/{id}.
func (h *Handlers) UpdateWebhook(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid webhook ID")
		return
	}

	var input UpdateWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	endpoint, err := h.Service.UpdateEndpoint(r.Context(), tenantID, id, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": endpoint})
}

// DeleteWebhook removes a webhook endpoint.
// DELETE /admin/webhooks/{id}.
func (h *Handlers) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid webhook ID")
		return
	}

	if err := h.Service.DeleteEndpoint(r.Context(), tenantID, id); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// TestWebhook sends a test event to a webhook endpoint.
// POST /admin/webhooks/{id}/test.
func (h *Handlers) TestWebhook(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid webhook ID")
		return
	}

	var input TestWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.EventType == "" {
		input.EventType = "webhook.test"
	}

	deliveryID, err := h.Service.TestEndpoint(r.Context(), tenantID, id, input.EventType)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"delivery_id": deliveryID.String(),
			"status":      "pending",
		},
	})
}

// ListDeliveries handles GET /admin/webhooks/{id}/deliveries.
func (h *Handlers) ListDeliveries(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid webhook ID")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	deliveries, err := h.Service.ListDeliveries(r.Context(), tenantID, id, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": deliveries})
}

// RetryDelivery handles POST /admin/webhooks/{id}/retry/{delivery_id}.
func (h *Handlers) RetryDelivery(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	deliveryID, err := parseUUIDParam(r, "delivery_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid delivery ID")
		return
	}

	if err := h.Service.RetryDelivery(r.Context(), tenantID, deliveryID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{"status": "retried"},
	})
}

// ListWebhookEvents handles GET /webhook-events — returns available webhook event types.
func (h *Handlers) ListWebhookEvents(w http.ResponseWriter, r *http.Request) {
	events := []map[string]string{
		// Content
		{ "key": "content.published", "label": "Content Published", "description": "When content is published" },
		{ "key": "content.updated", "label": "Content Updated", "description": "When content is updated" },
		{ "key": "content.deleted", "label": "Content Deleted", "description": "When content is soft-deleted" },
		{ "key": "content.liked", "label": "Content Liked", "description": "When a user likes content" },
		{ "key": "content.bookmarked", "label": "Content Bookmarked", "description": "When a user bookmarks content" },
		{ "key": "content.reviewed", "label": "Content Reviewed", "description": "When content receives a review" },
		// Claims
		{ "key": "claim.approved", "label": "Claim Approved", "description": "When an ownership claim is approved" },
		{ "key": "claim.rejected", "label": "Claim Rejected", "description": "When an ownership claim is rejected" },
		{ "key": "claim.submitted", "label": "Claim Submitted", "description": "When an ownership claim is submitted" },
		// Reviews
		{ "key": "review.created", "label": "Review Created", "description": "When a new review is created" },
		{ "key": "review.updated", "label": "Review Updated", "description": "When a review is updated" },
		{ "key": "review.deleted", "label": "Review Deleted", "description": "When a review is deleted" },
		// Organizations
		{ "key": "organization.verified", "label": "Organization Verified", "description": "When an organization is verified" },
		{ "key": "organization.created", "label": "Organization Created", "description": "When a new organization is created" },
		{ "key": "organization.updated", "label": "Organization Updated", "description": "When an organization profile is updated" },
		// Payments
		{ "key": "payment.succeeded", "label": "Payment Succeeded", "description": "When a payment succeeds" },
		{ "key": "payment.failed", "label": "Payment Failed", "description": "When a payment fails" },
		{ "key": "payment.refunded", "label": "Payment Refunded", "description": "When a payment is refunded" },
		{ "key": "payment.disputed", "label": "Payment Disputed", "description": "When a payment dispute is opened" },
		// Users
		{ "key": "user.registered", "label": "User Registered", "description": "When a new user account is created" },
		{ "key": "user.verified", "label": "User Verified", "description": "When a user account is verified" },
		{ "key": "user.deleted", "label": "User Deleted", "description": "When a user account is deleted (GDPR erasure)" },
		{ "key": "user.banned", "label": "User Banned", "description": "When a user is banned" },
		{ "key": "user.unbanned", "label": "User Unbanned", "description": "When a user ban is lifted" },
		{ "key": "user.profile_updated", "label": "User Profile Updated", "description": "When a user updates their profile" },
		// Comments
		{ "key": "comment.created", "label": "Comment Created", "description": "When a new comment is posted" },
		{ "key": "comment.updated", "label": "Comment Updated", "description": "When a comment is edited" },
		{ "key": "comment.deleted", "label": "Comment Deleted", "description": "When a comment is deleted" },
		{ "key": "comment.liked", "label": "Comment Liked", "description": "When a comment receives a like" },
		// Forum
		{ "key": "forum.thread.created", "label": "Forum Thread Created", "description": "When a new forum thread is created" },
		{ "key": "forum.thread.updated", "label": "Forum Thread Updated", "description": "When a forum thread is edited" },
		{ "key": "forum.thread.deleted", "label": "Forum Thread Deleted", "description": "When a forum thread is deleted" },
		{ "key": "forum.thread.solved", "label": "Forum Thread Solved", "description": "When a forum thread is marked as solved" },
		{ "key": "forum.reply.created", "label": "Forum Reply Created", "description": "When a new reply is posted in a forum thread" },
		{ "key": "forum.reply.updated", "label": "Forum Reply Updated", "description": "When a forum reply is edited" },
		{ "key": "forum.reply.deleted", "label": "Forum Reply Deleted", "description": "When a forum reply is deleted" },
		{ "key": "forum.vote.cast", "label": "Forum Vote Cast", "description": "When a user votes on a forum thread or reply" },
		// Spaces
		{ "key": "space.created", "label": "Space Created", "description": "When a new space is created" },
		{ "key": "space.updated", "label": "Space Updated", "description": "When a space profile is updated" },
		{ "key": "space.deleted", "label": "Space Deleted", "description": "When a space is deleted" },
		{ "key": "space.joined", "label": "Space Joined", "description": "When a user joins a space" },
		{ "key": "space.left", "label": "Space Left", "description": "When a user leaves a space" },
		// Wallet
		{ "key": "wallet.points.earned", "label": "Points Earned", "description": "When a user earns wallet points" },
		{ "key": "wallet.points.spent", "label": "Points Spent", "description": "When a user spends wallet points" },
		{ "key": "wallet.points.adjusted", "label": "Points Adjusted", "description": "When an admin adjusts a user's wallet points" },
		{ "key": "wallet.converted", "label": "Wallet Converted", "description": "When a user converts between wallet currencies" },
		// Moderation
		{ "key": "moderation.report.created", "label": "Report Created", "description": "When a moderation report is filed" },
		{ "key": "moderation.report.resolved", "label": "Report Resolved", "description": "When a moderation report is resolved" },
		{ "key": "moderation.content.removed", "label": "Content Removed", "description": "When content is removed by moderation" },
		{ "key": "moderation.user.suspended", "label": "User Suspended", "description": "When a user is suspended by moderation" },
		// Subscriptions
		{ "key": "subscription.created", "label": "Subscription Created", "description": "When a new subscription is created" },
		{ "key": "subscription.cancelled", "label": "Subscription Cancelled", "description": "When a subscription is cancelled" },
		{ "key": "subscription.renewed", "label": "Subscription Renewed", "description": "When a subscription is renewed" },
		{ "key": "subscription.expired", "label": "Subscription Expired", "description": "When a subscription expires" },
		// Newsletter
		{ "key": "newsletter.subscribed", "label": "Newsletter Subscribed", "description": "When a user subscribes to the newsletter" },
		{ "key": "newsletter.unsubscribed", "label": "Newsletter Unsubscribed", "description": "When a user unsubscribes from the newsletter" },
		// Affiliate
		{ "key": "affiliate.referral.created", "label": "Referral Created", "description": "When a new affiliate referral is created" },
		{ "key": "affiliate.commission.earned", "label": "Commission Earned", "description": "When an affiliate commission is earned" },
		{ "key": "affiliate.payout.requested", "label": "Payout Requested", "description": "When an affiliate payout is requested" },
		// AI
		{ "key": "ai.content.generated", "label": "AI Content Generated", "description": "When AI generates content" },
		{ "key": "ai.moderation.flagged", "label": "AI Moderation Flagged", "description": "When AI flags content for moderation" },
		// DM
		{ "key": "dm.message.sent", "label": "DM Sent", "description": "When a direct message is sent" },
		{ "key": "dm.conversation.created", "label": "DM Conversation Created", "description": "When a new DM conversation is created" },
		// Ads
		{ "key": "ad.campaign.created", "label": "Ad Campaign Created", "description": "When a new ad campaign is created" },
		{ "key": "ad.campaign.started", "label": "Ad Campaign Started", "description": "When an ad campaign goes live" },
		{ "key": "ad.campaign.ended", "label": "Ad Campaign Ended", "description": "When an ad campaign ends" },
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": events})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func parsePageSize(r *http.Request) int32 {
	if v := r.URL.Query().Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			return int32(n)
		}
	}
	return 20
}

func parsePageSizeOffset(r *http.Request) int32 {
	if v := r.URL.Query().Get("page_token"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return int32(n)
		}
	}
	return 0
}
