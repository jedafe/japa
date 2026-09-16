package publicapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

const defaultPageSize int32 = 20

// Handlers holds dependencies for Public API HTTP handlers.
type Handlers struct {
	Service *Service
	Queries *gen.Queries
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// SetQueries injects the sqlc queries for public API data access.
func (h *Handlers) SetQueries(q *gen.Queries) {
	h.Queries = q
}

// SetSiteURL is a no-op retained for backward compatibility. The site URL
// is now passed directly to GetPublicOpenAPISpec as a parameter (BUG-011
// fix: the package-level global was dead code — set but never read).
func SetSiteURL(_ string) {}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, errors.New("invalid " + param)
	}

	return id, nil
}

func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
}

func parsePageSize(r *http.Request, defaultSize, maxSize int32) int32 {
	ps := defaultSize

	if v := r.URL.Query().Get("page_size"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil && n > 0 {
			if n > int64(maxSize) {
				ps = maxSize
			} else {
				ps = int32(n)
			}
		}
	}

	return ps
}

func parseOffset(r *http.Request) int32 {
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil && n >= 0 {
			return int32(n)
		}
	}

	return 0
}

// ---------------------------------------------------------------------------
// Developer Handlers — API Key CRUD
// ---------------------------------------------------------------------------

// ListKeys returns all API keys for the authenticated user.
// GET /api/v1/developer/keys.
func (h *Handlers) ListKeys(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	keys, err := h.Service.ListAPIKeys(r.Context(), user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": keys})
}

// CreateKey generates a new API key and returns it once.
// POST /api/v1/developer/keys.
func (h *Handlers) CreateKey(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input CreateAPIKeyInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "name is required")
		return
	}

	resp, err := h.Service.CreateAPIKey(r.Context(), user.UserID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": resp})
}

// RevokeKey soft-deletes an API key.
// DELETE /api/v1/developer/keys/:id.
func (h *Handlers) RevokeKey(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	keyID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	if err := h.Service.RevokeAPIKey(r.Context(), user.UserID, keyID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{"status": "revoked"},
	})
}

// RotateKey rotates an API key: generates a new key with the same configuration,
// sets the old key to expire in 24h (grace period), and returns the new secret.
// POST /api/v1/developer/keys/:id/rotate.
func (h *Handlers) RotateKey(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	keyID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	resp, err := h.Service.RotateAPIKey(r.Context(), user.UserID, keyID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": resp})
}

// ---------------------------------------------------------------------------
// Admin Handlers — API Key Management
// ---------------------------------------------------------------------------

// ListAllKeys returns all API keys across all users (admin view).
// GET /api/v1/admin/api-keys.
func (h *Handlers) ListAllKeys(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r, 50, 200)
	offset := parseOffset(r)

	keys, err := h.Service.ListAllAPIKeys(r.Context(), limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": keys})
}

// SuspendKey suspends an API key (admin action).
// POST /api/v1/admin/api-keys/:id/suspend.
func (h *Handlers) SuspendKey(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	keyID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	if err := h.Service.SuspendAPIKey(r.Context(), keyID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{"status": "suspended"},
	})
}

// UnsuspendKey re-activates a suspended API key (admin action).
// DELETE /api/v1/admin/api-keys/:id/suspend.
func (h *Handlers) UnsuspendKey(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	keyID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	if err := h.Service.UnsuspendAPIKey(r.Context(), keyID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{"status": "active"},
	})
}

// DeleteKeyPermanently permanently deletes an API key (super_admin only).
// DELETE /api/v1/control/api-keys/:id/permanent.
func (h *Handlers) DeleteKeyPermanently(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	keyID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	if err := h.Service.DeleteAPIKey(r.Context(), keyID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{"status": "deleted"},
	})
}

// UpdateAPIKeyStatus updates the status of an API key (super_admin only).
// PATCH /api/v1/control/api-keys/:id.
func (h *Handlers) UpdateAPIKeyStatus(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	keyID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	var input struct {
		Name          string `json:"name"`
		Status        string `json:"status"`
		RateLimitTier string `json:"rate_limit_tier"`
		Notes         string `json:"notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	// Validate status if provided.
	if input.Status != "" {
		validStatuses := map[string]bool{"active": true, "suspended": true, "revoked": true}
		if !validStatuses[input.Status] {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", "Status must be active, suspended, or revoked")
			return
		}
	}

	// Delegate to the service method which uses a strict column allowlist
	// (SEC-002 / LAYER-003) — no hand-concatenated SQL in the handler.
	rowsAffected, err := h.Service.UpdateAPIKeyFields(r.Context(), keyID, APIKeyUpdateFields{
		Name:          input.Name,
		Status:        input.Status,
		RateLimitTier: input.RateLimitTier,
		Notes:         input.Notes,
	})
	if err != nil {
		if errors.Is(err, ErrNoFieldsToUpdate) {
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "No fields to update")
			return
		}

		handleServiceError(w, err)

		return
	}

	if rowsAffected == 0 {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "API key not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{"id": keyID.String()},
	})
}

// DeleteKeysPermanently permanently deletes multiple API keys (super_admin only).
// POST /api/v1/control/api-keys/batch-delete.
func (h *Handlers) DeleteKeysPermanently(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		IDs []string `json:"ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if len(input.IDs) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "ids is required")
		return
	}

	if len(input.IDs) > 100 {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "Maximum 100 keys per batch")
		return
	}

	var keyIDs []pgtype.UUID

	for _, idStr := range input.IDs {
		var id pgtype.UUID
		if err := id.Scan(idStr); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID: "+idStr)
			return
		}

		keyIDs = append(keyIDs, id)
	}

	if err := h.Service.DeleteAPIKeys(r.Context(), keyIDs); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"deleted": len(keyIDs),
		},
	})
}

// ---------------------------------------------------------------------------
// IP Allowlist Handlers
// ---------------------------------------------------------------------------

// ListAllowedIPs returns all IP allowlist entries for an API key.
// GET /api/v1/developer/keys/:id/ip-allowlist.
func (h *Handlers) ListAllowedIPs(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	keyID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	entries, err := h.Service.ListAllowedIPsForUser(r.Context(), user.UserID, keyID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": entries})
}

// AddAllowedIP adds an IP or CIDR range to an API key's allowlist.
// POST /api/v1/developer/keys/:id/ip-allowlist.
func (h *Handlers) AddAllowedIP(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	keyID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	var input struct {
		CIDR        string `json:"cidr"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.CIDR == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "cidr is required")
		return
	}

	if err := h.Service.AddAllowedIPForUser(r.Context(), user.UserID, keyID, input.CIDR, input.Description); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]string{"status": "added"},
	})
}

// RemoveAllowedIP removes an IP allowlist entry from an API key.
// DELETE /api/v1/developer/keys/:id/ip-allowlist/:ipId.
func (h *Handlers) RemoveAllowedIP(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	keyID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	ipID, err := parseUUIDParam(r, "ipId")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_ip_id", "Invalid IP allowlist entry ID")
		return
	}

	if err := h.Service.RemoveAllowedIPForUser(r.Context(), user.UserID, keyID, ipID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{"status": "removed"},
	})
}

// ─── P5.6 Me API Key Extras ──────────────────────────────────────────────

// GetAPIKey handles GET /me/api-keys/:id — returns a single API key (P5.6).
func (h *Handlers) GetAPIKey(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	keyID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	key, err := h.Service.GetOwnAPIKey(r.Context(), user.UserID, keyID)
	if err != nil {
		handleServiceError(w, err) // pgx.ErrNoRows → 404, other → 500 (PUB-013)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": key})
}

// UpdateAPIKey handles PATCH /me/api-keys/:id — updates an API key (P5.6).
//
// Self-service update is restricted to the key's display name only.
// Scope changes require admin approval (Section 47 line 182: "Write scopes
// are approved by admin on a per-key basis") and are handled via the
// control-panel PATCH /control/api-keys/{id} route, not this self-service
// endpoint. This closes the scope self-escalation vector (PUB-001).
func (h *Handlers) UpdateAPIKey(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	keyID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	var input struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "name is required")
		return
	}

	rowsAffected, err := h.Service.UpdateOwnAPIKeyName(r.Context(), user.UserID, keyID, input.Name)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if rowsAffected == 0 {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "API key not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"id": keyID.String()}})
}

// ---------------------------------------------------------------------------
// Public API Resource Handlers (Section 47 — api.japaconnect.com)
// ---------------------------------------------------------------------------
// These are read-only endpoints exposed to external integrations via API key auth.
// They proxy to the same underlying data as the platform-scoped internal API but
// return a simplified, stable response shape suitable for third-party consumers.

// GetContentByID handles GET /api/v1/content/:id — returns a single content item.
// Only published, public, non-deleted content is exposed to external API
// consumers (Section 47 — BUG-001 fix: GetContentByID exposed drafts/private).
func (h *Handlers) GetContentByID(w http.ResponseWriter, r *http.Request) {
	if h.Queries == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid content ID")
		return
	}

	content, err := h.Queries.GetPublishedContentByID(r.Context(),
		id,
	)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": content})
}

// ListSpaces handles GET /api/v1/spaces — returns a paginated list of spaces.
func (h *Handlers) ListSpaces(w http.ResponseWriter, r *http.Request) {
	if h.Queries == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	spaces, err := h.Queries.ListSpaces(r.Context(),

		pgtype.UUID{}, // public API key has no user context → NULL → public spaces only
	)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": spaces})
}

// GetSpaceBySlug handles GET /api/v1/spaces/:slug — returns a single space.
// Only public, active, non-deleted spaces are exposed to external API
// consumers (Section 47 — BUG-004 fix: GetSpaceBySlug exposed private/inactive).
func (h *Handlers) GetSpaceBySlug(w http.ResponseWriter, r *http.Request) {
	if h.Queries == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	space, err := h.Queries.GetPublicSpaceBySlug(r.Context(),
		slug,
	)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": space})
}

// ListTags handles GET /api/v1/tags — returns a paginated list of tags.
// Uses ListTagsPaginated (BUG-010 fix: ListTags returned ALL tags
// with no pagination — unbounded response for large platforms).
func (h *Handlers) ListTags(w http.ResponseWriter, r *http.Request) {
	if h.Queries == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	limit := parsePageSize(r, defaultPageSize, 100)
	offset := parseOffset(r)
	q := r.URL.Query().Get("q")
	sort := r.URL.Query().Get("sort")

	tags, err := h.Queries.ListTagsPaginated(r.Context(), gen.ListTagsPaginatedParams{

		Q:          q,
		Sort:       sort,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	total, err := h.Queries.CountTagsPaginated(r.Context(), q)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":   tags,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetUserByID handles GET /api/v1/users/:id — returns a single user profile.
// PII-safe: no email, status, last_login, or group membership (Section 47 —
// BUG-002 fix: GetUserByID leaked email + status fields).
// Only active users who have not opted out of search indexability.
func (h *Handlers) GetUserByID(w http.ResponseWriter, r *http.Request) {
	if h.Queries == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	user, err := h.Queries.GetPublicUserByID(r.Context(),
		id,
	)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": user})
}

// SearchContent handles GET /api/v1/search — searches content by title.
func (h *Handlers) SearchContent(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	q := r.URL.Query().Get("q")
	if q == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_query", "q parameter is required")
		return
	}

	limit := parsePageSize(r, defaultPageSize, 100)
	offset := parseOffset(r)

	results, err := h.Service.SearchContent(r.Context(), q, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": results})
}

// GetMyProfile handles GET /api/v1/me — returns the authenticated user's profile.
// Requires api.read.me scope on the API key.
func (h *Handlers) GetMyProfile(w http.ResponseWriter, r *http.Request) {
	if h.Queries == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	user, err := h.Queries.GetUserByID(r.Context(),
		keyCtx.UserID,
	)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": user})
}

// ListContent handles GET /api/v1/public/content — returns a paginated list
// of published, public content within the API key's platform.
// Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md.
func (h *Handlers) ListContent(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	limit := parsePageSize(r, defaultPageSize, 100)
	offset := parseOffset(r)

	results, err := h.Service.ListPublishedContent(r.Context(), limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":   results,
		"limit":  limit,
		"offset": offset,
	})
}

// ListContentComments handles GET /api/v1/public/content/{id}/comments —
// returns top-level published comments for a content item, but ONLY if the
// parent content itself is published and public (Section 47 — BUG-005 fix:
// comments on drafts/private content were exposed).
// Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md.
func (h *Handlers) ListContentComments(w http.ResponseWriter, r *http.Request) {
	if h.Queries == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid content ID")
		return
	}

	limit := parsePageSize(r, defaultPageSize, 100)
	offset := parseOffset(r)

	comments, err := h.Queries.ListPublicCommentsByContent(r.Context(), gen.ListPublicCommentsByContentParams{
		ContentID: contentID,

		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":   comments,
		"limit":  limit,
		"offset": offset,
	})
}

// ListUsers handles GET /api/v1/public/users — returns a paginated list of
// public user profiles. PII-safe: no email, status, last_login, or groups.
// Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md.
func (h *Handlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	if h.Queries == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	search := r.URL.Query().Get("q")
	profileType := r.URL.Query().Get("type")
	limit := parsePageSize(r, defaultPageSize, 100)
	offset := parseOffset(r)

	users, err := h.Queries.ListPublicUsers(r.Context(), gen.ListPublicUsersParams{

		Search:      search,
		ProfileType: profileType,
		PageOffset:  offset,
		PageLimit:   limit,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":   users,
		"limit":  limit,
		"offset": offset,
	})
}

// GetTagBySlug handles GET /api/v1/public/tags/{slug} — returns a single
// tag by its URL slug.
// Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md.
func (h *Handlers) GetTagBySlug(w http.ResponseWriter, r *http.Request) {
	if h.Queries == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	tag, err := h.Queries.GetTagBySlug(r.Context(),
		slug,
	)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tag})
}

// ---------------------------------------------------------------------------
// Trending endpoints (Section 47 — public read-only trending snapshots)
// ---------------------------------------------------------------------------

// parseTrendingWindow validates the ?window= query param. Accepts 24h, 7d,
// 30d; defaults to 24h (matching internal/trending/handlers.go).
func parseTrendingWindow(r *http.Request) string {
	if v := r.URL.Query().Get("window"); v != "" {
		switch v {
		case "24h", "7d", "30d":
			return v
		}
	}

	return "24h"
}

// GetTrendingContentHandler handles GET /api/v1/trending/content — returns
// trending content snapshots for the API key's platform.
// Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md.
func (h *Handlers) GetTrendingContentHandler(w http.ResponseWriter, r *http.Request) {
	if h.Queries == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	window := parseTrendingWindow(r)
	geoCode := r.URL.Query().Get("geo")
	limit := parsePageSize(r, 10, 50)

	rows, err := h.Queries.GetTrendingContent(r.Context(), gen.GetTrendingContentParams{

		Window: window, Column2: geoCode, Limit: limit,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if rows == nil {
		rows = []gen.GetTrendingContentRow{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// GetTrendingTagsHandler handles GET /api/v1/trending/tags.
func (h *Handlers) GetTrendingTagsHandler(w http.ResponseWriter, r *http.Request) {
	if h.Queries == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	window := parseTrendingWindow(r)
	geoCode := r.URL.Query().Get("geo")
	limit := parsePageSize(r, 10, 50)

	rows, err := h.Queries.GetTrendingTags(r.Context(), gen.GetTrendingTagsParams{

		Window: window, Column2: geoCode, Limit: limit,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if rows == nil {
		rows = []gen.GetTrendingTagsRow{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// GetTrendingSpacesHandler handles GET /api/v1/trending/spaces.
func (h *Handlers) GetTrendingSpacesHandler(w http.ResponseWriter, r *http.Request) {
	if h.Queries == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	window := parseTrendingWindow(r)
	geoCode := r.URL.Query().Get("geo")
	limit := parsePageSize(r, 10, 50)

	rows, err := h.Queries.GetTrendingSpaces(r.Context(), gen.GetTrendingSpacesParams{

		Window: window, Column2: geoCode, Limit: limit,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if rows == nil {
		rows = []gen.GetTrendingSpacesRow{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// GetTrendingUsersHandler handles GET /api/v1/trending/users.
func (h *Handlers) GetTrendingUsersHandler(w http.ResponseWriter, r *http.Request) {
	if h.Queries == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	window := parseTrendingWindow(r)
	geoCode := r.URL.Query().Get("geo")
	limit := parsePageSize(r, 10, 50)

	rows, err := h.Queries.GetTrendingUsers(r.Context(), gen.GetTrendingUsersParams{

		Window: window, Column2: geoCode, Limit: limit,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if rows == nil {
		rows = []gen.GetTrendingUsersRow{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// GetAPIKeyAnalytics returns aggregate analytics for all API keys (admin view).
// GET /api/v1/control/api-keys/analytics.
func (h *Handlers) GetAPIKeyAnalytics(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	analytics, err := h.Service.GetAnalytics(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": analytics})
}
