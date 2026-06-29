// Package bots — HTTP handlers for Bot & Crawler Management (Section 43).
//
// Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package bots

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for Bot management HTTP handlers.
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

func parsePageSize(r *http.Request) int32 {
	ps := DefaultPageSize
	if v := r.URL.Query().Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > MaxPageSize {
				ps = MaxPageSize
			} else {
				ps = n
			}
		}
	}
	return int32(ps)
}

func parseOffset(r *http.Request) int32 {
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return int32(n)
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// Admin Handlers — Known Bots CRUD
// ---------------------------------------------------------------------------

// ListKnownBots returns all known bots for the tenant, optionally filtered by bot_type.
// GET /api/v1/admin/bots?bot_type=search_engine
func (h *Handlers) ListKnownBots(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	botType := r.URL.Query().Get("bot_type")
	if botType != "" && !IsBotType(botType) {
		writeError(w, http.StatusBadRequest, "invalid_bot_type", "Invalid bot type")
		return
	}

	bots, err := h.Service.ListKnownBots(r.Context(), tenantID, botType)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": bots})
}

// CreateKnownBot creates a new known bot entry.
// POST /api/v1/admin/bots
func (h *Handlers) CreateKnownBot(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Name             string `json:"name"`
		UserAgentPattern string `json:"user_agent_pattern"`
		BotType          string `json:"bot_type"`
		Policy           string `json:"policy"`
		CrawlDelay       int    `json:"crawl_delay"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	// Validate required fields
	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "name is required")
		return
	}
	if input.UserAgentPattern == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "user_agent_pattern is required")
		return
	}

	// Validate bot_type
	if input.BotType == "" {
		input.BotType = string(BotTypeUnknown)
	}
	if !IsBotType(input.BotType) {
		writeError(w, http.StatusBadRequest, "invalid_bot_type", "Invalid bot type")
		return
	}

	// Validate policy
	if input.Policy == "" {
		input.Policy = string(PolicyAllow)
	}
	if !IsPolicy(input.Policy) {
		writeError(w, http.StatusBadRequest, "invalid_policy", "Invalid policy")
		return
	}

	bot, err := h.Service.CreateKnownBot(r.Context(), tenantID, CreateKnownBotInput{
		Name:             input.Name,
		UserAgentPattern: input.UserAgentPattern,
		BotType:          BotType(input.BotType),
		Policy:           BotPolicy(input.Policy),
		CrawlDelay:       input.CrawlDelay,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": bot})
}

// UpdateKnownBot updates an existing known bot entry.
// PUT /api/v1/admin/bots/:id
func (h *Handlers) UpdateKnownBot(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	botID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid bot ID")
		return
	}

	var input struct {
		Name             string `json:"name"`
		UserAgentPattern string `json:"user_agent_pattern"`
		BotType          string `json:"bot_type"`
		Policy           string `json:"policy"`
		CrawlDelay       int    `json:"crawl_delay"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "name is required")
		return
	}
	if input.UserAgentPattern == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "user_agent_pattern is required")
		return
	}
	if !IsBotType(input.BotType) {
		writeError(w, http.StatusBadRequest, "invalid_bot_type", "Invalid bot type")
		return
	}
	if !IsPolicy(input.Policy) {
		writeError(w, http.StatusBadRequest, "invalid_policy", "Invalid policy")
		return
	}

	bot, err := h.Service.UpdateKnownBot(r.Context(), tenantID, botID, UpdateKnownBotInput{
		Name:             input.Name,
		UserAgentPattern: input.UserAgentPattern,
		BotType:          BotType(input.BotType),
		Policy:           BotPolicy(input.Policy),
		CrawlDelay:       input.CrawlDelay,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": bot})
}

// DeleteKnownBot removes a known bot entry.
// DELETE /api/v1/admin/bots/:id
func (h *Handlers) DeleteKnownBot(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	botID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid bot ID")
		return
	}

	if err := h.Service.DeleteKnownBot(r.Context(), tenantID, botID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}
