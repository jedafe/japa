package impersonation

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/httpx"
	"github.com/jedafe/japa/internal/tenant"
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
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
	case errors.Is(err, errpkg.ErrAlreadyExists):
		httpx.WriteError(w, http.StatusConflict, "already_exists", "Resource already exists")
	case errors.Is(err, errpkg.ErrRequired):
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, errpkg.ErrInvalid):
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

// ---------------------------------------------------------------------------
// Impersonation Handlers
// ---------------------------------------------------------------------------

// StartImpersonation begins impersonating a target user.
// POST /api/v1/admin/impersonate — body: {"user_id": "uuid"}.
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
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.UserID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", "user_id is required")
		return
	}

	var targetUserID pgtype.UUID
	if err := targetUserID.Scan(input.UserID); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id format")
		return
	}

	if err := h.Service.StartImpersonation(r.Context(), tenantID, user.UserID, targetUserID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{
			"status":    "impersonation_started",
			"target_id": input.UserID,
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

	if err := h.Service.EndImpersonation(r.Context(), tenantID, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{
			"status": "impersonation_ended",
		},
	})
}

// GetImpersonationStatus returns the current impersonation state.
// GET /api/v1/admin/impersonate/status.
func (h *Handlers) GetImpersonationStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	targetID, active, err := h.Service.GetImpersonationStatus(r.Context(), user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"active":    active,
			"target_id": targetID,
		},
	})
}
