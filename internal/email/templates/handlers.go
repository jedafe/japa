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
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// Handlers is an exported type.
type Handlers struct {
	Service     *Service
	AsynqClient *asynq.Client
	redis       valkeycompat.Cmdable
}

// NewHandlers is an exported function.
func NewHandlers(service *Service, asynqClient *asynq.Client) *Handlers {
	return &Handlers{Service: service, AsynqClient: asynqClient}
}

// SetRedisClient injects the Valkey client for template caching and rate
// limiting. Optional — when nil, in-memory rate limiting is used.
func (h *Handlers) SetRedisClient(c valkeycompat.Cmdable) {
	h.redis = c
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

	return int32(n) // #nosec
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

	return int32(n) // #nosec
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrTemplateNotFound):
		errpkg.WriteError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, errpkg.ErrRequired):
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
	default:
		errpkg.HandleServiceError(w, err)
	}
}

// ── Admin CRUD ──────────────────────────────────────────────────────────

// ListTemplates is an exported method.
func (h *Handlers) ListTemplates(w http.ResponseWriter, r *http.Request) {

	filterType := r.URL.Query().Get("type")
	filterIsActive := r.URL.Query().Get("is_active")
	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	result, err := h.Service.ListTemplates(r.Context(), filterType, filterIsActive, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// GetTemplate is an exported method.
func (h *Handlers) GetTemplate(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid template ID")
		return
	}

	tmpl, err := h.Service.GetTemplate(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tmpl})
}

// UpdateTemplate is an exported method.
func (h *Handlers) UpdateTemplate(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid template ID")
		return
	}

	var input struct {
		Subject       string `json:"subject"`
		HtmlBody      string `json:"html_body"`
		PlainTextBody string `json:"plain_text_body"`
		IsActive      bool   `json:"is_active"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	tmpl, err := h.Service.UpdateTemplate(r.Context(), id, input.Subject, input.HtmlBody, input.PlainTextBody, input.IsActive)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tmpl})
}

// ResetTemplate is an exported method.
func (h *Handlers) ResetTemplate(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid template ID")
		return
	}

	if err := h.Service.ResetTemplate(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "reset"}})
}

func (h *Handlers) TestTemplate(w http.ResponseWriter, r *http.Request) {

	// Rate limit: 3 test emails per minute per user.
	// Uses Valkey distributed rate limiting when available, falls back to
	// in-memory for single-instance deployments.
	user := auth.FromContext(r.Context())
	if user != nil {
		key := "test_email:" + user.UserID.String()
		allowed := true

		if h.redis != nil {
			// Distributed fixed-window via INCR + EXPIRE.
			ctx := r.Context()
			count, err := h.redis.Incr(ctx, "jc:ratelimit:"+key).Result()

			if err == nil && count == 1 {
				h.redis.Expire(ctx, "jc:ratelimit:"+key, 1*time.Minute)
			}

			allowed = err == nil && count <= 3
		} else {
			allowed = testEmailLimiter.Allow(key, 3, 1*time.Minute)
		}

		if !allowed {
			errpkg.WriteError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "Too many test email requests. Please try again later.")
			return
		}
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid template ID")
		return
	}

	var input struct {
		Email string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if input.Email == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "email is required")
		return
	}

	tmpl, err := h.Service.GetTemplate(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Enqueue an email:send task using the template's slug for DB lookup.
	if h.AsynqClient != nil {
		payload := map[string]interface{}{
			"to":            input.Email,
			"template_slug": tmpl.Slug,

			"template_data": map[string]string{},
		}
		body, _ := json.Marshal(payload)

		task := asynq.NewTask("email:send", body)
		if _, err := h.AsynqClient.Enqueue(task, asynq.Queue("default"), asynq.MaxRetry(3)); err != nil {
			slog.Error("test_template: failed to enqueue email", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "enqueue_failed", "Failed to queue test email")

			return
		}
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]string{
			"status":  "queued",
			"message": "Test email queued for delivery to " + input.Email,
		},
	})
}
