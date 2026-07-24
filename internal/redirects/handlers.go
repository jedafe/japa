package redirects

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/httpx"
	"github.com/jedafe/japa/internal/tenant"
)

type Handlers struct {
	Service *Service
}

func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// ── Helpers ─────────────────────────────────────────────────────────────

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "Tenant context not available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

func parsePageSize(r *http.Request) int32 {
	v := r.URL.Query().Get("page_size")
	if v == "" {
		return DefaultPageSize
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return DefaultPageSize
	}
	if n > int(MaxPageSize) {
		return MaxPageSize
	}
	return int32(n)
}

func parsePageOffset(r *http.Request) int32 {
	v := r.URL.Query().Get("page_offset")
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return int32(n)
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrRedirectNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrInvalidHTTPStatus):
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, ErrDuplicateSourcePath):
		httpx.WriteError(w, http.StatusConflict, "already_exists", err.Error())
	case errors.Is(err, errpkg.ErrRequired):
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
	}
}

// ── Admin CRUD ──────────────────────────────────────────────────────────

func (h *Handlers) ListRedirects(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	var isActive *bool
	if v := r.URL.Query().Get("status"); v != "" {
		b := v == "active"
		isActive = &b
	}
	search := r.URL.Query().Get("q")
	limit := parsePageSize(r)
	offset := parsePageOffset(r)
	result, err := h.Service.ListRedirects(r.Context(), tenantID, isActive, search, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (h *Handlers) GetRedirect(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid redirect ID")
		return
	}
	redirect, err := h.Service.GetRedirect(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": redirect})
}

func (h *Handlers) CreateRedirect(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	var input struct {
		SourcePath string  `json:"source_path"`
		TargetURL  string  `json:"target_url"`
		HTTPStatus int16   `json:"http_status"`
		Notes      *string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}
	if input.SourcePath == "" || input.TargetURL == "" {
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", "source_path and target_url are required")
		return
	}
	if input.HTTPStatus == 0 {
		input.HTTPStatus = 301
	}
	redirect, err := h.Service.CreateRedirect(r.Context(), tenantID, input.SourcePath, input.TargetURL, input.HTTPStatus, input.Notes)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": redirect})
}

func (h *Handlers) UpdateRedirect(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid redirect ID")
		return
	}
	var input struct {
		SourcePath string  `json:"source_path"`
		TargetURL  string  `json:"target_url"`
		HTTPStatus int16   `json:"http_status"`
		IsActive   bool    `json:"is_active"`
		Notes      *string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}
	redirect, err := h.Service.UpdateRedirect(r.Context(), id, tenantID, input.SourcePath, input.TargetURL, input.HTTPStatus, input.IsActive, input.Notes)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": redirect})
}

func (h *Handlers) DeleteRedirect(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid redirect ID")
		return
	}
	if err := h.Service.DeleteRedirect(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

func (h *Handlers) BulkAction(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	var input BulkActionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}
	if input.Action == "" || len(input.Ids) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", "action and ids are required")
		return
	}
	if err := h.Service.BulkAction(r.Context(), tenantID, input); err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "ok"}})
}
