package publicapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

const defaultPageSize int32 = 20

// Handlers holds dependencies for Public API HTTP handlers.
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

func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, errors.New("invalid " + param)
	}

	return id, nil
}

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
	case strings.Contains(err.Error(), "already_exists"):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	case strings.Contains(err.Error(), "duplicate"):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	case strings.Contains(err.Error(), "required"):
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
	case strings.Contains(err.Error(), "invalid"):
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
	case strings.Contains(err.Error(), "expired"):
		writeError(w, http.StatusUnauthorized, "key_expired", err.Error())
	case strings.Contains(err.Error(), "not found"):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case strings.Contains(err.Error(), "does not belong"):
		writeError(w, http.StatusForbidden, "forbidden", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if v != nil {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			slog.Warn("json encode error", "error", err)
		}
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
			"status":  status,
			"docs":    "https://api.japaconnect.com/docs#section/Errors",
		},
	})
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

	writeJSON(w, http.StatusOK, map[string]any{"data": keys})
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
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "name is required")
		return
	}

	resp, err := h.Service.CreateAPIKey(r.Context(), user.UserID, tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": resp})
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
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	if err := h.Service.RevokeAPIKey(r.Context(), user.UserID, keyID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
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
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	resp, err := h.Service.RotateAPIKey(r.Context(), user.UserID, keyID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": resp})
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

	writeJSON(w, http.StatusOK, map[string]any{"data": keys})
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
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	if err := h.Service.SuspendAPIKey(r.Context(), keyID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{"status": "suspended"},
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
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
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
		writeError(w, http.StatusForbidden, "forbidden", "API key does not belong to user")
		return
	}

	entries, err := h.Service.ListAllowedIPs(r.Context(), keyID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": entries})
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
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
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
		writeError(w, http.StatusForbidden, "forbidden", "API key does not belong to user")
		return
	}

	var input struct {
		CIDR        string `json:"cidr"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.CIDR == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "cidr is required")
		return
	}

	if err := h.Service.AddAllowedIP(r.Context(), keyID, input.CIDR, input.Description); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
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
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	ipID, err := parseUUIDParam(r, "ipId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_ip_id", "Invalid IP allowlist entry ID")
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
		writeError(w, http.StatusForbidden, "forbidden", "API key does not belong to user")
		return
	}

	if err := h.Service.RemoveAllowedIP(r.Context(), keyID, ipID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{"status": "removed"},
	})
}
