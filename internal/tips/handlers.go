// handlers.go — HTTP surface for §125 tip settings, external links, and the
// public tip-config endpoints. Error shape follows
// conventions/api-response-format.md via errpkg.
package tips

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/middleware"
)

// Handlers exposes the tips HTTP surface.
type Handlers struct {
	service *Service
}

// NewHandlers creates Handlers bound to the service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return user, true
}

func writeTipError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrTipsDisabled):
		errpkg.WriteError(w, http.StatusForbidden, "tips_disabled", "Tips are disabled")
	case errors.Is(err, ErrNotEligible):
		errpkg.WriteError(w, http.StatusForbidden, "tips_not_eligible", "Tips are not enabled for your account")
	case errors.Is(err, ErrNotAuthorized):
		errpkg.WriteError(w, http.StatusForbidden, "not_authorized", "Not authorized")
	case errors.Is(err, ErrRecipientNoTips):
		errpkg.WriteError(w, http.StatusForbidden, "recipient_no_tips", "This recipient does not accept tips")
	case errors.Is(err, ErrNotFound):
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Not found")
	case errors.Is(err, ErrMethodNotAccepted):
		errpkg.WriteError(w, http.StatusBadRequest, "method_not_accepted", "Recipient does not accept this tip method")
	case errors.Is(err, ErrInvalidMethod):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_method", "Invalid tip method")
	case errors.Is(err, ErrInvalidRecipient):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_recipient", "Invalid tip recipient")
	case errors.Is(err, ErrTipsNotAllowed):
		errpkg.WriteError(w, http.StatusConflict, "tips_not_allowed", "Tips are not enabled for this space")
	case errors.Is(err, ErrInvalidAmount):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_amount", "Invalid tip amount")
	case errors.Is(err, ErrBelowMinimum):
		errpkg.WriteError(w, http.StatusBadRequest, "below_minimum", "Tip below recipient minimum")
	case errors.Is(err, ErrAboveMaximum):
		errpkg.WriteError(w, http.StatusBadRequest, "above_maximum", "Tip above maximum")
	case errors.Is(err, ErrSelfTip):
		errpkg.WriteError(w, http.StatusBadRequest, "self_tip", "You cannot tip yourself")
	case errors.Is(err, ErrInsufficientBalance):
		errpkg.WriteError(w, http.StatusBadRequest, "insufficient_balance", "Insufficient balance")
	case errors.Is(err, ErrInvalidLink):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_link", "Invalid external link")
	case errors.Is(err, ErrTooManyLinks):
		errpkg.WriteError(w, http.StatusBadRequest, "too_many_links", "Too many external links")
	case errors.Is(err, ErrOCNotConnected):
		errpkg.WriteError(w, http.StatusBadRequest, "oc_not_connected", "Open Collective not connected")
	case errors.Is(err, ErrAlreadySettled):
		errpkg.WriteError(w, http.StatusConflict, "already_settled", "Tip already settled")
	case errors.Is(err, ErrInvalidStatus):
		errpkg.WriteError(w, http.StatusConflict, "invalid_status", "Invalid tip status for this operation")
	case errors.Is(err, ErrBlocked):
		errpkg.WriteError(w, http.StatusForbidden, "user_blocked", "Cannot tip this user")
	case errors.Is(err, ErrMessageTooLong):
		errpkg.WriteError(w, http.StatusBadRequest, "message_too_long", "Tip message is too long")
	case errors.Is(err, ErrOCAccess):
		errpkg.WriteError(w, http.StatusBadRequest, "oc_access_denied", "Token has no access to this collective")
	default:
		errpkg.WriteError(w, http.StatusInternalServerError, "internal", "Internal error")
	}
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
		return pgtype.UUID{}, false
	}
	return id, true
}

func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return 20
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 20
	}
	if n > 100 {
		return 100
	}
	return int32(n) // #nosec G109
}

// parsePage resolves the 1-based page number. `page` is the cataloged
// contract; `page_offset` is honored as a legacy fallback for older clients.
func parsePage(r *http.Request, limit int32) int32 {
	if s := r.URL.Query().Get("page"); s != "" {
		// Cap before int32 conversion — a huge value wraps negative and
		// yields a negative SQL OFFSET (500 instead of empty page).
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 1_000_000 {
			return int32(n) // #nosec G109 G115
		}
		return 1
	}
	if s := r.URL.Query().Get("page_offset"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 100_000_000 && limit > 0 {
			return int32(n)/limit + 1 // #nosec G109 G115
		}
	}
	return 1
}

// --- My settings --------------------------------------------------------------

// GetMyTipSettings handles GET /api/v1/me/tip-settings.
func (h *Handlers) GetMyTipSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	st, err := h.service.GetSettings(r.Context(), UserOwner(user.UserID))
	if err != nil {
		writeTipError(w, err)
		return
	}
	links, err := h.service.ListLinks(r.Context(), UserOwner(user.UserID))
	if err != nil {
		writeTipError(w, err)
		return
	}
	eligible, _ := h.service.CanReceive(r.Context(), user.UserID)
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"settings":       st,
		"external_links": links,
		"eligible":       eligible,
	}})
}

// UpdateMyTipSettings handles PUT /api/v1/me/tip-settings.
func (h *Handlers) UpdateMyTipSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var in UpdateSettingsInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	st, err := h.service.UpdateSettingsFor(r.Context(), UserOwner(user.UserID), user.UserID, in)
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": st})
}

// --- My external links --------------------------------------------------------

// CreateMyTipLink handles POST /api/v1/me/tip-settings/links.
func (h *Handlers) CreateMyTipLink(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var in LinkInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	link, err := h.service.CreateLink(r.Context(), UserOwner(user.UserID), user.UserID, in)
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": link})
}

// UpdateMyTipLink handles PATCH /api/v1/me/tip-settings/links/{id}.
func (h *Handlers) UpdateMyTipLink(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	linkID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var in LinkInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	link, err := h.service.UpdateLink(r.Context(), UserOwner(user.UserID), user.UserID, linkID, in)
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": link})
}

// DeleteMyTipLink handles DELETE /api/v1/me/tip-settings/links/{id}.
func (h *Handlers) DeleteMyTipLink(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	linkID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteLink(r.Context(), UserOwner(user.UserID), user.UserID, linkID); err != nil {
		writeTipError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Organization settings ----------------------------------------------------

// GetOrgTipSettings handles GET /api/v1/organizations/{id}/tip-settings.
func (h *Handlers) GetOrgTipSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	orgID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	owner := OrgOwner(orgID)
	if err := h.service.ensureWritable(r.Context(), owner, user.UserID); err != nil {
		writeTipError(w, err)
		return
	}
	st, err := h.service.GetSettings(r.Context(), owner)
	if err != nil {
		writeTipError(w, err)
		return
	}
	links, err := h.service.ListLinks(r.Context(), owner)
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"settings":       st,
		"external_links": links,
	}})
}

// UpdateOrgTipSettings handles PUT /api/v1/organizations/{id}/tip-settings.
func (h *Handlers) UpdateOrgTipSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	orgID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var in UpdateSettingsInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	st, err := h.service.UpdateSettingsFor(r.Context(), OrgOwner(orgID), user.UserID, in)
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": st})
}

// CreateOrgTipLink handles POST /api/v1/organizations/{id}/tip-settings/links.
func (h *Handlers) CreateOrgTipLink(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	orgID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var in LinkInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	link, err := h.service.CreateLink(r.Context(), OrgOwner(orgID), user.UserID, in)
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": link})
}

// UpdateOrgTipLink handles PATCH /api/v1/organizations/{id}/tip-settings/links/{lid}.
func (h *Handlers) UpdateOrgTipLink(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	orgID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	linkID, ok := scanUUIDParam(w, r, "lid")
	if !ok {
		return
	}
	var in LinkInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	link, err := h.service.UpdateLink(r.Context(), OrgOwner(orgID), user.UserID, linkID, in)
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": link})
}

// DeleteOrgTipLink handles DELETE /api/v1/organizations/{id}/tip-settings/links/{lid}.
func (h *Handlers) DeleteOrgTipLink(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	orgID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	linkID, ok := scanUUIDParam(w, r, "lid")
	if !ok {
		return
	}
	if err := h.service.DeleteLink(r.Context(), OrgOwner(orgID), user.UserID, linkID); err != nil {
		writeTipError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Open Collective connect ---------------------------------------------------

type ocConnectRequest struct {
	CollectiveSlug string `json:"collective_slug"`
	APIToken       string `json:"api_token"`
	// RotateSecret regenerates the webhook secret (compromised/lost URL).
	RotateSecret bool `json:"rotate_secret,omitempty"`
}

// ConnectMyOC handles PUT /api/v1/me/tip-settings/opencollective.
func (h *Handlers) ConnectMyOC(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var in ocConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	st, err := h.service.ConnectOC(r.Context(), UserOwner(user.UserID), user.UserID, in.CollectiveSlug, in.APIToken, in.RotateSecret)
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": st})
}

// DisconnectMyOC handles DELETE /api/v1/me/tip-settings/opencollective.
func (h *Handlers) DisconnectMyOC(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	if err := h.service.DisconnectOC(r.Context(), UserOwner(user.UserID), user.UserID); err != nil {
		writeTipError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ConnectOrgOC handles PUT /api/v1/organizations/{id}/tip-settings/opencollective.
func (h *Handlers) ConnectOrgOC(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	orgID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var in ocConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	st, err := h.service.ConnectOC(r.Context(), OrgOwner(orgID), user.UserID, in.CollectiveSlug, in.APIToken, in.RotateSecret)
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": st})
}

// DisconnectOrgOC handles DELETE /api/v1/organizations/{id}/tip-settings/opencollective.
func (h *Handlers) DisconnectOrgOC(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	orgID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.service.DisconnectOC(r.Context(), OrgOwner(orgID), user.UserID); err != nil {
		writeTipError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Tip creation ---------------------------------------------------------------

// createTipRequest is the POST /api/v1/tips body. `amount` maps onto the
// per-method column: credits→credits_amount, fiat→fiat_amount.
type createTipRequest struct {
	RecipientType string `json:"recipient_type"`
	RecipientID   string `json:"recipient_id"`
	ContentID     string `json:"content_id"`
	Method        string `json:"method"`
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
	Message       string `json:"message"`
	IsAnonymous   bool   `json:"is_anonymous"`
}

// CreateTip handles POST /api/v1/tips — Rule 9 Idempotency-Key required.
func (h *Handlers) CreateTip(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	idemKey := middleware.IdempotencyKeyFromContext(r.Context())
	if idemKey == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_idempotency_key", "Idempotency-Key header is required")
		return
	}
	var in createTipRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if !IsValidMethod(in.Method) || in.Method == MethodOpenCollective {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_method", "Invalid tip method")
		return
	}

	var input CreateTipInput
	input.Method = in.Method
	input.RecipientType = in.RecipientType
	input.Message = in.Message
	input.IsAnonymous = in.IsAnonymous
	input.FiatCurrency = in.Currency
	if in.Method == MethodCredits {
		input.CreditsAmount = in.Amount
	} else {
		input.FiatAmount = in.Amount
	}
	if in.RecipientID != "" {
		var rid pgtype.UUID
		if err := rid.Scan(in.RecipientID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_recipient", "Invalid recipient_id")
			return
		}
		if in.RecipientType == RecipientOrganization {
			input.RecipientOrgID = rid
		} else if in.RecipientType == RecipientSpace {
			input.RecipientSpaceID = rid
		} else {
			input.RecipientUserID = rid
		}
	}
	if in.ContentID != "" {
		if err := input.ContentID.Scan(in.ContentID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_recipient", "Invalid content_id")
			return
		}
	}

	res, err := h.service.CreateTip(r.Context(), user.UserID, input, idemKey)
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": res})
}

// --- Admin moderation -----------------------------------------------------------

// AdminRefundTip handles POST /api/v1/admin/tips/{id}/refund — reverses the
// recipient credit grant and marks the tip refunded (tips.moderate-gated).
func (h *Handlers) AdminRefundTip(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tipID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.service.AdminRefundTip(r.Context(), tipID, user.UserID); err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": "refunded"}})
}

// --- Inbound Open Collective webhook --------------------------------------------

// ocWebhookPayload is the inbound contribution event. OC's webhook envelope
// carries a contribution object; we only need its id/amount/fromAccount.
type ocWebhookPayload struct {
	Type string `json:"type"`
	Data struct {
		Order struct {
			ID          string `json:"id"`
			TotalAmount struct {
				ValueInCents int64  `json:"valueInCents"`
				Currency     string `json:"currency"`
			} `json:"totalAmount"`
			FromAccount struct {
				Name string `json:"name"`
				Slug string `json:"slug"`
			} `json:"fromAccount"`
		} `json:"order"`
	} `json:"data"`
}

// HandleOCWebhook handles POST /api/v1/tips/webhook/opencollective?s=<secret>
// — per-collective secret in the query identifies the owner (OC has no
// signature header on webhooks).
func (h *Handlers) HandleOCWebhook(w http.ResponseWriter, r *http.Request) {
	// Path-param secret (preferred — keeps it out of query-string logs) with
	// the spec's ?s= form kept for webhooks already configured that way.
	secret := chi.URLParam(r, "secret")
	if secret == "" {
		secret = r.URL.Query().Get("s")
	}
	if secret == "" {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Missing webhook secret")
		return
	}
	var payload ocWebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if payload.Data.Order.ID == "" {
		w.WriteHeader(http.StatusNoContent) // non-contribution event — nothing to import
		return
	}
	if err := h.service.ImportOCWebhook(r.Context(), secret, OCContribution{
		ID:          payload.Data.Order.ID,
		AmountCents: payload.Data.Order.TotalAmount.ValueInCents,
		Currency:    payload.Data.Order.TotalAmount.Currency,
		FromName:    payload.Data.Order.FromAccount.Name,
		FromSlug:    payload.Data.Order.FromAccount.Slug,
	}); err != nil {
		if errors.Is(err, ErrNotFound) {
			errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Invalid webhook secret")
			return
		}
		writeTipError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Tip history ----------------------------------------------------------------

// ListMyTipsSent handles GET /api/v1/me/tips/sent.
func (h *Handlers) ListMyTipsSent(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	limit := parsePageSize(r)
	tips, err := h.service.ListTipsSent(r.Context(), user.UserID, parsePage(r, limit), limit)
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tips})
}

// ListMyTipsReceived handles GET /api/v1/me/tips/received.
func (h *Handlers) ListMyTipsReceived(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	limit := parsePageSize(r)
	tips, err := h.service.ListTipsReceived(r.Context(), user.UserID, parsePage(r, limit), limit)
	if err != nil {
		writeTipError(w, err)
		return
	}
	total, err := h.service.CountTipsReceived(r.Context(), user.UserID)
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tips, "meta": map[string]any{"total_received": total}})
}

// ContentTipSummary handles GET /api/v1/content/{id}/tip-summary — public.
func (h *Handlers) ContentTipSummary(w http.ResponseWriter, r *http.Request) {
	contentID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	summary, err := h.service.ContentTipSummary(r.Context(), contentID)
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": summary})
}

// --- Admin tips surface ---------------------------------------------------------

// AdminListTips handles GET /api/v1/admin/tips.
func (h *Handlers) AdminListTips(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	method := r.URL.Query().Get("method")
	recipientType := r.URL.Query().Get("recipient_type")
	q := r.URL.Query().Get("q")
	limit := parsePageSize(r)
	tips, total, err := h.service.AdminListTips(r.Context(), status, method, recipientType, q, parsePage(r, limit), limit)
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tips, "meta": map[string]any{"total": total}})
}

// AdminGetTip handles GET /api/v1/admin/tips/{id} — single hydrated record.
func (h *Handlers) AdminGetTip(w http.ResponseWriter, r *http.Request) {
	tipID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	tip, err := h.service.GetTip(r.Context(), tipID)
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tip})
}

// AdminTipStats handles GET /api/v1/admin/tips/stats — 30-day window by
// default, override with ?days=N.
func (h *Handlers) AdminTipStats(w http.ResponseWriter, r *http.Request) {
	days, err := strconv.Atoi(r.URL.Query().Get("days"))
	if err != nil || days <= 0 || days > 365 {
		days = 30
	}
	var since pgtype.Timestamptz
	_ = since.Scan(time.Now().AddDate(0, 0, -days))
	stats, byCur, err := h.service.AdminTipStats(r.Context(), since)
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"total": stats.Total, "credits_settled": stats.CreditsSettled,
		"fiat_minor_settled": stats.FiatMinorSettled, "oc_count": stats.OcCount,
		"fiat_by_currency": byCur,
	}})
}

// --- Public config ------------------------------------------------------------

// PublicUserTipConfig handles GET /api/v1/users/{username}/tip-config.
func (h *Handlers) PublicUserTipConfig(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	userID, err := h.service.lookupUserIDByUsername(r.Context(), username)
	if err != nil {
		writeTipError(w, err)
		return
	}
	cfg, err := h.service.PublicConfig(r.Context(), UserOwner(userID))
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": cfg})
}

// PublicOrgTipConfig handles GET /api/v1/organizations/{id}/donation-config.
func (h *Handlers) PublicOrgTipConfig(w http.ResponseWriter, r *http.Request) {
	orgID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	cfg, err := h.service.PublicConfig(r.Context(), OrgOwner(orgID))
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": cfg})
}

// PublicDonate handles GET /api/v1/donate — the platform donation config.
func (h *Handlers) PublicDonate(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.service.PublicConfig(r.Context(), PlatformOwner())
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": cfg})
}

// --- Admin platform settings --------------------------------------------------

// AdminGetDonationSettings handles GET /api/v1/admin/donation-settings.
func (h *Handlers) AdminGetDonationSettings(w http.ResponseWriter, r *http.Request) {
	st, err := h.service.GetSettings(r.Context(), PlatformOwner())
	if err != nil {
		writeTipError(w, err)
		return
	}
	links, err := h.service.ListLinks(r.Context(), PlatformOwner())
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"settings":       st,
		"external_links": links,
	}})
}

// AdminUpdateDonationSettings handles PUT /api/v1/admin/donation-settings.
func (h *Handlers) AdminUpdateDonationSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var in UpdateSettingsInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	st, err := h.service.UpdateSettingsFor(r.Context(), PlatformOwner(), user.UserID, in)
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": st})
}

// --- Admin platform donation links -------------------------------------------

// AdminCreateDonationLink handles POST /api/v1/admin/donation-settings/links.
func (h *Handlers) AdminCreateDonationLink(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var in LinkInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	link, err := h.service.CreateLink(r.Context(), PlatformOwner(), user.UserID, in)
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": link})
}

// AdminUpdateDonationLink handles PATCH /api/v1/admin/donation-settings/links/{id}.
func (h *Handlers) AdminUpdateDonationLink(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	linkID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var in LinkInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	link, err := h.service.UpdateLink(r.Context(), PlatformOwner(), user.UserID, linkID, in)
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": link})
}

// AdminDeleteDonationLink handles DELETE /api/v1/admin/donation-settings/links/{id}.
func (h *Handlers) AdminDeleteDonationLink(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	linkID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteLink(r.Context(), PlatformOwner(), user.UserID, linkID); err != nil {
		writeTipError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// spaceBySlug resolves a space by slug for the space tip endpoints.
func (h *Handlers) spaceBySlug(w http.ResponseWriter, r *http.Request) (gen.GetSpaceBySlugRow, bool) {
	var space gen.GetSpaceBySlugRow
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Space slug is required")
		return space, false
	}
	space, err := h.service.store.getSpaceBySlug(r.Context(), slug)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Space not found")
		return space, false
	}
	return space, true
}

// canViewSpaceTips enforces the same visibility contract as the spaces
// endpoints: non-public spaces hide their tip config and ledger from
// non-members; non-active spaces are manager-only (owner/mod/admin).
func (h *Handlers) canViewSpaceTips(r *http.Request, spaceID pgtype.UUID, visibility, status string) bool {
	if visibility == "public" && status == "active" {
		return true
	}
	user := auth.FromContext(r.Context())
	if user == nil || !user.UserID.Valid {
		return false
	}
	ctx := r.Context()
	// Managers (owner, assigned moderator, spaces.moderate/admin node) can
	// always see the space's tip state — mirrors canManageRestrictedSpace.
	isManager := false
	if ok, err := h.service.store.checkSpaceOwnership(ctx, spaceID, user.UserID); err == nil && ok {
		isManager = true
	} else if ok, err := h.service.store.getSpaceModerator(ctx, spaceID, user.UserID); err == nil && ok {
		isManager = true
	} else if h.service.perms != nil &&
		(h.service.perms.Can(ctx, user.UserID, "spaces.moderate") || h.service.perms.Can(ctx, user.UserID, "spaces.admin")) {
		isManager = true
	}
	if status != "active" {
		return isManager
	}
	if isManager {
		return true
	}
	if ok, err := h.service.store.checkSpaceMembership(ctx, spaceID, user.UserID); err == nil && ok {
		return true
	}
	return false
}

func spaceTipReadGate(h *Handlers, w http.ResponseWriter, r *http.Request, space gen.GetSpaceBySlugRow) bool {
	if !h.canViewSpaceTips(r, space.ID, space.Visibility, space.Status) {
		errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Space not found")
		return false
	}
	return true
}

// GetSpaceTipSettings handles GET /api/v1/spaces/{slug}/tip-settings — the
// public button state for a tip-eligible space.
func (h *Handlers) GetSpaceTipSettings(w http.ResponseWriter, r *http.Request) {
	space, ok := h.spaceBySlug(w, r)
	if !ok || !spaceTipReadGate(h, w, r, space) {
		return
	}
	cfg, err := h.service.PublicConfig(r.Context(), SpaceOwner(space.ID))
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": cfg})
}

// UpdateSpaceTipSettings handles PATCH /api/v1/spaces/{slug}/tip-settings —
// owner/admin only; the service re-checks ownership and eligibility.
func (h *Handlers) UpdateSpaceTipSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	space, ok := h.spaceBySlug(w, r)
	if !ok {
		return
	}
	var in UpdateSettingsInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	st, err := h.service.UpdateSettingsFor(r.Context(), SpaceOwner(space.ID), user.UserID, in)
	if err != nil {
		writeTipError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": st})
}

// ListSpaceTips handles GET /api/v1/spaces/{slug}/tips — paginated tips the
// space has received.
func (h *Handlers) ListSpaceTips(w http.ResponseWriter, r *http.Request) {
	space, ok := h.spaceBySlug(w, r)
	if !ok || !spaceTipReadGate(h, w, r, space) {
		return
	}
	limit := parsePageSize(r)
	offset := parsePage(r, limit)
	rows, err := h.service.store.listTipsBySpace(r.Context(), gen.ListTipsBySpaceParams{
		SpaceID: space.ID, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list tips")
		return
	}

	// Anonymous tips hide sender identity for everyone except the recipient
	// owner and tips moderators — same masking as the other tip lists.
	mask := true
	if user := auth.FromContext(r.Context()); user != nil && user.UserID.Valid {
		if ownerID, oerr := h.service.store.getSpaceTipRecipient(r.Context(), space.ID); oerr == nil && ownerID == user.UserID {
			mask = false
		}
		if mask && h.service.perms != nil && h.service.perms.Can(r.Context(), user.UserID, "tips.moderate") {
			mask = false
		}
	}
	items := h.service.hydrateTips(r.Context(), rows, mask)

	total, _ := h.service.store.countTipsBySpace(r.Context(), space.ID)
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": items,
		"meta": map[string]any{"total": total},
	})
}
