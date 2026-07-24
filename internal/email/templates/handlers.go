package templates

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
	case errors.Is(err, ErrTemplateNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, errpkg.ErrRequired):
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
	}
}

// ── Admin CRUD ──────────────────────────────────────────────────────────

func (h *Handlers) ListTemplates(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	filterType := r.URL.Query().Get("type")
	limit := parsePageSize(r)
	offset := parsePageOffset(r)
	result, err := h.Service.ListTemplates(r.Context(), tenantID, filterType, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (h *Handlers) GetTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid template ID")
		return
	}
	tmpl, err := h.Service.GetTemplate(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": tmpl})
}

func (h *Handlers) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid template ID")
		return
	}
	var input struct {
		Name          string `json:"name"`
		Subject       string `json:"subject"`
		HtmlBody      string `json:"html_body"`
		PlainTextBody string `json:"plain_text_body"`
		Status        string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}
	tmpl, err := h.Service.UpdateTemplate(r.Context(), id, tenantID, input.Name, input.Subject, input.HtmlBody, input.PlainTextBody, input.Status)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": tmpl})
}

func (h *Handlers) ResetTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid template ID")
		return
	}
	if err := h.Service.ResetTemplate(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "reset"}})
}

func (h *Handlers) TestTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid template ID")
		return
	}
	var input struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}
	if input.Email == "" {
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", "email is required")
		return
	}
	// Test send is a future feature — for now, validate the template exists and
	// return accepted status. The actual email sending will be implemented when
	// the email:send task supports template-based rendering.
	tmpl, err := h.Service.GetTemplate(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	_ = tmpl
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]string{
			"status":  "queued",
			"message": "Test email queued for delivery to " + input.Email,
		},
	})
}
