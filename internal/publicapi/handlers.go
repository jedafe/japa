package publicapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/httpx"
	"github.com/jedafe/japa/internal/tenant"
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

// siteURL is the base URL for the site, set during initialization.
var siteURL string

// SetSiteURL sets the base URL for the site (e.g. "https://example.com").
func SetSiteURL(url string) {
	siteURL = url
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
	case errors.Is(err, errpkg.ErrAlreadyExists), errors.Is(err, errpkg.ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, "already_exists", "Resource already exists")
	case errors.Is(err, errpkg.ErrRequired):
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, errpkg.ErrInvalid):
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, errpkg.ErrExpired):
		httpx.WriteError(w, http.StatusUnauthorized, "key_expired", err.Error())
	case errors.Is(err, errpkg.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, errpkg.ErrBelongsToAnother):
		httpx.WriteError(w, http.StatusForbidden, "forbidden", err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

func parsePageSize(r *http.Request, defaultSize, maxSize int32) int32 {
	ps := defaultSize

	if v := r.URL.Query().Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > int(maxSize) {
				ps = maxSize
			} else {
				ps = int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
			}
		}
	}

	return ps
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

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": keys})
}

// CreateKey generates a new API key and returns it once.
// POST /api/v1/developer/keys.
func (h *Handlers) CreateKey(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input CreateAPIKeyInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", "name is required")
		return
	}

	resp, err := h.Service.CreateAPIKey(r.Context(), user.UserID, tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": resp})
}

// RevokeKey soft-deletes an API key.
// DELETE /api/v1/developer/keys/:id.
func (h *Handlers) RevokeKey(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	keyID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	if err := h.Service.RevokeAPIKey(r.Context(), user.UserID, keyID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
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

	keyID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	resp, err := h.Service.RotateAPIKey(r.Context(), user.UserID, keyID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": resp})
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
	offset := httpx.ParseOffset(r)

	keys, err := h.Service.ListAllAPIKeys(r.Context(), limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": keys})
}

// SuspendKey suspends an API key (admin action).
// POST /api/v1/admin/api-keys/:id/suspend.
func (h *Handlers) SuspendKey(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	keyID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	if err := h.Service.SuspendAPIKey(r.Context(), keyID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
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

	keyID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	if err := h.Service.UnsuspendAPIKey(r.Context(), keyID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
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

	keyID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	if err := h.Service.DeleteAPIKey(r.Context(), keyID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
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

	keyID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	var input struct {
		Name          string `json:"name"`
		Status        string `json:"status"`
		RateLimitTier string `json:"rate_limit_tier"`
		Notes         string `json:"notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	// Validate status if provided.
	if input.Status != "" {
		validStatuses := map[string]bool{"active": true, "suspended": true, "revoked": true}
		if !validStatuses[input.Status] {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_status", "Status must be active, suspended, or revoked")
			return
		}
	}

	// Build dynamic update query.
	updates := []string{}
	args := []any{keyID}
	argIdx := 2

	if input.Name != "" {
		updates = append(updates, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, input.Name)
		argIdx++
	}

	if input.Status != "" {
		updates = append(updates, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, input.Status)
		argIdx++
	}

	if input.RateLimitTier != "" {
		updates = append(updates, fmt.Sprintf("rate_limit_tier = $%d", argIdx))
		args = append(args, input.RateLimitTier)
		argIdx++
	}

	if input.Notes != "" {
		updates = append(updates, fmt.Sprintf("notes = $%d", argIdx))
		args = append(args, input.Notes)
		argIdx++
	}

	if len(updates) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", "No fields to update")
		return
	}

	updates = append(updates, "updated_at = now()")

	query := fmt.Sprintf("UPDATE api_keys SET %s WHERE id = $1", strings.Join(updates, ", "))

	tag, err := h.Service.store.dbPool.Exec(r.Context(), query, args...)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if tag.RowsAffected() == 0 {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "API key not found")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
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
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if len(input.IDs) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", "ids is required")
		return
	}

	if len(input.IDs) > 100 {
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", "Maximum 100 keys per batch")
		return
	}

	var keyIDs []pgtype.UUID
	for _, idStr := range input.IDs {
		var id pgtype.UUID
		if err := id.Scan(idStr); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID: "+idStr)
			return
		}
		keyIDs = append(keyIDs, id)
	}

	if err := h.Service.DeleteAPIKeys(r.Context(), keyIDs); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
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

	keyID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	// Verify the key belongs to the user.
	var ownerID pgtype.UUID

	err = h.Service.store.dbPool.QueryRow(r.Context(), `
		SELECT user_id FROM api_keys WHERE id = $1
	`, keyID).Scan(&ownerID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if ownerID != user.UserID {
		httpx.WriteError(w, http.StatusForbidden, "forbidden", "API key does not belong to user")
		return
	}

	entries, err := h.Service.ListAllowedIPs(r.Context(), keyID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": entries})
}

// AddAllowedIP adds an IP or CIDR range to an API key's allowlist.
// POST /api/v1/developer/keys/:id/ip-allowlist.
func (h *Handlers) AddAllowedIP(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	keyID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	// Verify the key belongs to the user.
	var ownerID pgtype.UUID

	err = h.Service.store.dbPool.QueryRow(r.Context(), `
		SELECT user_id FROM api_keys WHERE id = $1
	`, keyID).Scan(&ownerID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if ownerID != user.UserID {
		httpx.WriteError(w, http.StatusForbidden, "forbidden", "API key does not belong to user")
		return
	}

	var input struct {
		CIDR        string `json:"cidr"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.CIDR == "" {
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", "cidr is required")
		return
	}

	if err := h.Service.AddAllowedIP(r.Context(), keyID, input.CIDR, input.Description); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
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

	keyID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	ipID, err := httpx.ParseUUIDParam(r, "ipId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_ip_id", "Invalid IP allowlist entry ID")
		return
	}

	// Verify the key belongs to the user.
	var ownerID pgtype.UUID

	err = h.Service.store.dbPool.QueryRow(r.Context(), `
		SELECT user_id FROM api_keys WHERE id = $1
	`, keyID).Scan(&ownerID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if ownerID != user.UserID {
		httpx.WriteError(w, http.StatusForbidden, "forbidden", "API key does not belong to user")
		return
	}

	if err := h.Service.RemoveAllowedIP(r.Context(), keyID, ipID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
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
	keyID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}
	var key APIKey
	err = h.Service.store.dbPool.QueryRow(r.Context(),
		`SELECT id, name, key_prefix, scopes, status, created_at, last_used_at
		 FROM api_keys WHERE id = $1 AND user_id = $2`,
		keyID, user.UserID).
		Scan(&key.ID, &key.Name, &key.KeyPrefix, &key.Scopes, &key.Status, &key.CreatedAt, &key.LastUsedAt)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "API key not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": key})
}

// UpdateAPIKey handles PATCH /me/api-keys/:id — updates an API key (P5.6).
func (h *Handlers) UpdateAPIKey(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	keyID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}
	var input struct {
		Name   string `json:"name"`
		Scopes string `json:"scopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	_, err = h.Service.store.dbPool.Exec(r.Context(),
		`UPDATE api_keys SET name = COALESCE(NULLIF($3,''), name),
		 scopes = COALESCE(NULLIF($4,''), scopes), updated_at = now()
		 WHERE id = $1 AND user_id = $2`, keyID, user.UserID, input.Name, input.Scopes)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update API key")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"id": keyID.String()}})
}

// ---------------------------------------------------------------------------
// Public API Resource Handlers (Section 47 — api.japaconnect.com)
// ---------------------------------------------------------------------------
// These are read-only endpoints exposed to external integrations via API key auth.
// They proxy to the same underlying data as the tenant-scoped internal API but
// return a simplified, stable response shape suitable for third-party consumers.

// GetContentByID handles GET /api/v1/content/:id — returns a single content item.
func (h *Handlers) GetContentByID(w http.ResponseWriter, r *http.Request) {
	if h.Queries == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid content ID")
		return
	}

	content, err := h.Queries.GetContentByID(r.Context(), gen.GetContentByIDParams{
		ID:       id,
		TenantID: keyCtx.TenantID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": content})
}

// ListSpaces handles GET /api/v1/spaces — returns a paginated list of spaces.
func (h *Handlers) ListSpaces(w http.ResponseWriter, r *http.Request) {
	if h.Queries == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	spaces, err := h.Queries.ListSpacesByTenant(r.Context(), keyCtx.TenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": spaces})
}

// GetSpaceBySlug handles GET /api/v1/spaces/:slug — returns a single space.
func (h *Handlers) GetSpaceBySlug(w http.ResponseWriter, r *http.Request) {
	if h.Queries == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	space, err := h.Queries.GetSpaceBySlug(r.Context(), gen.GetSpaceBySlugParams{
		Slug:     slug,
		TenantID: keyCtx.TenantID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": space})
}

// ListTags handles GET /api/v1/tags — returns a paginated list of tags.
func (h *Handlers) ListTags(w http.ResponseWriter, r *http.Request) {
	if h.Queries == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	tags, err := h.Queries.ListTagsByTenant(r.Context(), keyCtx.TenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": tags})
}

// GetUserByID handles GET /api/v1/users/:id — returns a single user profile.
func (h *Handlers) GetUserByID(w http.ResponseWriter, r *http.Request) {
	if h.Queries == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	user, err := h.Queries.GetUserByID(r.Context(), gen.GetUserByIDParams{
		UserID:   id,
		TenantID: keyCtx.TenantID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": user})
}

// SearchContent handles GET /api/v1/search — searches content by title.
func (h *Handlers) SearchContent(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	q := r.URL.Query().Get("q")
	if q == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_query", "q parameter is required")
		return
	}

	limit := parsePageSize(r, defaultPageSize, 100)
	offset := httpx.ParseOffset(r)

	// Search content by title using PostgreSQL ILIKE (simple substring match).
	rows, err := h.Service.store.dbPool.Query(r.Context(), `
		SELECT id, title, slug, content_type, status, created_at
		FROM contents
		WHERE tenant_id = $1 AND status = 'published' AND title ILIKE '%' || $2 || '%'
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`, keyCtx.TenantID, q, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var id pgtype.UUID
		var title, slug, contentType, status string
		var createdAt pgtype.Timestamptz
		if err := rows.Scan(&id, &title, &slug, &contentType, &status, &createdAt); err != nil {
			continue
		}
		results = append(results, map[string]any{
			"id":           id.String(),
			"title":        title,
			"slug":         slug,
			"content_type": contentType,
			"status":       status,
			"created_at":   createdAt,
		})
	}

	if results == nil {
		results = []map[string]any{}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": results})
}

// GetMyProfile handles GET /api/v1/me — returns the authenticated user's profile.
// Requires api.read.me scope on the API key.
func (h *Handlers) GetMyProfile(w http.ResponseWriter, r *http.Request) {
	if h.Queries == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	user, err := h.Queries.GetUserByID(r.Context(), gen.GetUserByIDParams{
		UserID:   keyCtx.UserID,
		TenantID: keyCtx.TenantID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": user})
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

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": analytics})
}
