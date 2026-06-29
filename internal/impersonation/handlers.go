package impersonation

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
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
	case strings.Contains(err.Error(), "required"):
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
	case strings.Contains(err.Error(), "invalid"):
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
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
		"error": map[string]string{"code": code, "message": message},
	})
}

// ---------------------------------------------------------------------------
// Impersonation Handlers
// ---------------------------------------------------------------------------

// StartImpersonation begins impersonating a target user.
// POST /api/v1/admin/impersonate — body: {"user_id": "uuid"}
func (h *Handlers) StartImpersonation(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.UserID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "user_id is required")
		return
	}

	var targetUserID pgtype.UUID
	if err := targetUserID.Scan(input.UserID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id format")
		return
	}

	user, _ := h.requireUser(w, r)

	if err := h.Service.StartImpersonation(r.Context(), user.UserID, targetUserID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{
			"status":  "impersonation_started",
			"target_id": input.UserID,
		},
	})
}

// EndImpersonation stops impersonating the current target.
// DELETE /api/v1/admin/impersonate
func (h *Handlers) EndImpersonation(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	if err := h.Service.EndImpersonation(r.Context(), user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{
			"status": "impersonation_ended",
		},
	})
}

// GetImpersonationStatus returns the current impersonation state.
// GET /api/v1/admin/impersonate/status
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

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"active":    active,
			"target_id": targetID,
		},
	})
}
