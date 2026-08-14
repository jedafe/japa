package impersonation

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/valkey"
)

// Handlers holds dependencies for impersonation HTTP handlers.
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

func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrTargetIsAdmin):
		errpkg.WriteError(w, http.StatusForbidden, "cannot_impersonate_admin", "Cannot impersonate an admin-level user")
	case errors.Is(err, ErrTargetNotFound):
		errpkg.WriteError(w, http.StatusNotFound, "target_user_not_found", "Target user not found")
	case errors.Is(err, ErrNoActiveSession):
		errpkg.WriteError(w, http.StatusNotFound, "no_active_session", "No active impersonation session")
	default:
		errpkg.HandleServiceError(w, err)
	}
}

// ---------------------------------------------------------------------------
// Impersonation Handlers
// ---------------------------------------------------------------------------

// StartImpersonation begins impersonating a target user.
// POST /api/v1/admin/impersonate — body: {"user_id": "uuid"}.
// Response per Section 46: {"session_key":"jc:impersonate:{admin_id}","expires_at":"ISO8601","target":{id,username,email}}.
func (h *Handlers) StartImpersonation(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		UserID string `json:"user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.UserID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "user_id is required")
		return
	}

	var targetUserID pgtype.UUID
	if err := targetUserID.Scan(input.UserID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id format")
		return
	}

	ipAddress := clientip.FromRequest(r)

	if err := h.Service.StartImpersonation(r.Context(), tenantID, user.UserID, targetUserID, ipAddress); err != nil {
		handleServiceError(w, err)
		return
	}

	// Fetch target profile for the response (Section 46 contract).
	target, err := h.Service.GetTargetProfile(r.Context(), tenantID, targetUserID)
	if err != nil {
		// Impersonation succeeded but profile fetch failed — return minimal response.
		slog.Warn("impersonation: failed to fetch target profile for response", "error", err)
		target = &TargetProfile{ID: input.UserID}
	}

	expiresAt := time.Now().Add(DefaultImpersonationTTL).UTC().Format(time.RFC3339)
	sessionKey := valkey.MustKey("impersonate:%s", user.UserID.String())

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"session_key": sessionKey,
			"expires_at":  expiresAt,
			"target":      target,
		},
	})
}

// EndImpersonation stops impersonating the current target.
// DELETE /api/v1/admin/impersonate.
func (h *Handlers) EndImpersonation(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	ipAddress := clientip.FromRequest(r)

	targetID, err := h.Service.EndImpersonation(r.Context(), tenantID, user.UserID, ipAddress)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"status":    "impersonation_ended",
			"target_id": targetID,
		},
	})
}

// GetImpersonationStatus returns the current impersonation state.
// GET /api/v1/admin/impersonate/status.
// Response per Section 46: {"active":bool,"target_user_id":"uuid|null","expires_at":"ISO8601|null","target":{id,username,email}|null}.
// The target profile is included so the UI banner can display the
// username instead of a raw UUID (Section 46 "UI Banner").
func (h *Handlers) GetImpersonationStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	targetID, active, ttlSeconds, target, err := h.Service.GetImpersonationStatusWithTarget(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	var expiresAt any
	if active && ttlSeconds > 0 {
		expiresAt = time.Now().Add(time.Duration(ttlSeconds) * time.Second).UTC().Format(time.RFC3339)
	} else {
		expiresAt = nil
	}

	var targetUserID any
	if active {
		targetUserID = targetID
	} else {
		targetUserID = nil
	}

	var targetProfile any
	if active && target != nil {
		targetProfile = target
	} else {
		targetProfile = nil
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"active":         active,
			"target_user_id": targetUserID,
			"expires_at":     expiresAt,
			"target":         targetProfile,
		},
	})
}
