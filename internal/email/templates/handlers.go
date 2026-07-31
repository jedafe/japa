package templates

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

type Handlers struct {
	Service     *Service
	AsynqClient *asynq.Client
}

func NewHandlers(service *Service, asynqClient *asynq.Client) *Handlers {
	return &Handlers{Service: service, AsynqClient: asynqClient}
}

// testEmailRateLimiter is a simple in-memory fixed-window rate limiter keyed
// by user ID. It limits test-email requests to 3 per minute per user.
type testEmailRateLimiter struct {
	mu      sync.Mutex
	windows map[string]*rateLimitWindow
}

type rateLimitWindow struct {
	count     int
	expiresAt time.Time
}

func (r *testEmailRateLimiter) Allow(key string, max int, window time.Duration) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	w, ok := r.windows[key]
	if !ok || now.After(w.expiresAt) {
		r.windows[key] = &rateLimitWindow{count: 1, expiresAt: now.Add(window)}
		return true
	}
	w.count++
	return w.count <= max
}

var testEmailLimiter = &testEmailRateLimiter{windows: make(map[string]*rateLimitWindow)}

// ── Helpers ─────────────────────────────────────────────────────────────

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "Tenant context not available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

func parseUUIDParam(r *http.Request, name string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		return pgtype.UUID{}, err
	}
	return id, nil
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		json.NewEncoder(w).Encode(v)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrTemplateNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, errpkg.ErrRequired):
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
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
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (h *Handlers) GetTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid template ID")
		return
	}
	tmpl, err := h.Service.GetTemplate(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": tmpl})
}

func (h *Handlers) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid template ID")
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
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}
	tmpl, err := h.Service.UpdateTemplate(r.Context(), id, tenantID, input.Name, input.Subject, input.HtmlBody, input.PlainTextBody, input.Status)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": tmpl})
}

func (h *Handlers) ResetTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid template ID")
		return
	}
	if err := h.Service.ResetTemplate(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "reset"}})
}

func (h *Handlers) TestTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Rate limit: 3 test emails per minute per user.
	user := auth.FromContext(r.Context())
	if user != nil {
		key := "test_email:" + user.UserID.String()
		if !testEmailLimiter.Allow(key, 3, 1*time.Minute) {
			writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "Too many test email requests. Please try again later.")
			return
		}
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid template ID")
		return
	}
	var input struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}
	if input.Email == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "email is required")
		return
	}
	tmpl, err := h.Service.GetTemplate(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Enqueue an email:send task using the template's slug for DB lookup.
	if h.AsynqClient != nil {
		payload := map[string]interface{}{
			"to":            input.Email,
			"template_slug": tmpl.Type,
			"tenant_id":     tenantID.String(),
			"template_data": map[string]string{},
		}
		body, _ := json.Marshal(payload)
		task := asynq.NewTask("email:send", body)
		if _, err := h.AsynqClient.Enqueue(task, asynq.Queue("default"), asynq.MaxRetry(3)); err != nil {
			slog.Error("test_template: failed to enqueue email", "error", err)
			writeError(w, http.StatusInternalServerError, "enqueue_failed", "Failed to queue test email")
			return
		}
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]string{
			"status":  "queued",
			"message": "Test email queued for delivery to " + input.Email,
		},
	})
}
