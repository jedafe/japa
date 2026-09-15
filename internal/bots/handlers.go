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
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
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


func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
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

	return int32(ps) //nolint:gosec // G115: bounded by MaxPageSize check above
}

func parseOffset(r *http.Request) int32 {
	if v := r.URL.Query().Get("page_offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			if n > math.MaxInt32 {
				return math.MaxInt32
			}

			// #nosec G109 // value bounded by MaxPageSize check
			return int32(n) //nolint:gosec // G115: bounded by MaxInt32 check above
		}
	}

	return 0
}

// ---------------------------------------------------------------------------
// Admin Handlers — Known Bots CRUD
// ---------------------------------------------------------------------------

// ListKnownBots returns all known bots for the tenant, optionally filtered by bot_type.
// GET /api/v1/admin/bots?bot_type=search_engine.
func (h *Handlers) ListKnownBots(w http.ResponseWriter, r *http.Request) {

	botType := r.URL.Query().Get("bot_type")
	if botType != "" && !IsBotType(botType) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_bot_type", "Invalid bot type")
		return
	}

	result, err := h.Service.ListKnownBotsWithCount(r.Context(), botType, parsePageSize(r), parseOffset(r))
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        result.Data,
		"total":       result.Total,
		"total_pages": result.TotalPages,
	})
}

// GetKnownBot returns a single known bot by ID.
// GET /api/v1/admin/bots/{id}.
func (h *Handlers) GetKnownBot(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid bot ID")
		return
	}

	bot, err := h.Service.GetKnownBotByID(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": bot})
}

// CreateKnownBot creates a new known bot entry.
// POST /api/v1/admin/bots.
func (h *Handlers) CreateKnownBot(w http.ResponseWriter, r *http.Request) {

	var input struct {
		Name             string `json:"name"`
		UserAgentPattern string `json:"user_agent_pattern"`
		BotType          string `json:"bot_type"`
		Policy           string `json:"policy"`
		CrawlDelay       int    `json:"crawl_delay"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	// Validate required fields
	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "name is required")
		return
	}

	if input.UserAgentPattern == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "user_agent_pattern is required")
		return
	}

	// Validate bot_type
	if input.BotType == "" {
		input.BotType = string(BotTypeUnknown)
	}

	if !IsBotType(input.BotType) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_bot_type", "Invalid bot type")
		return
	}

	// Validate policy
	if input.Policy == "" {
		input.Policy = string(PolicyAllow)
	}

	if !IsPolicy(input.Policy) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_policy", "Invalid policy")
		return
	}

	bot, err := h.Service.CreateKnownBot(r.Context(), CreateKnownBotInput{
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

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": bot})
}

// UpdateKnownBot updates an existing known bot entry.
// PUT /api/v1/admin/bots/:id.
func (h *Handlers) UpdateKnownBot(w http.ResponseWriter, r *http.Request) {

	botID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid bot ID")
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "name is required")
		return
	}

	if input.UserAgentPattern == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "user_agent_pattern is required")
		return
	}

	if !IsBotType(input.BotType) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_bot_type", "Invalid bot type")
		return
	}

	if !IsPolicy(input.Policy) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_policy", "Invalid policy")
		return
	}

	bot, err := h.Service.UpdateKnownBot(r.Context(), botID, UpdateKnownBotInput{
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": bot})
}

// DeleteKnownBot removes a known bot entry.
// DELETE /api/v1/admin/bots/:id.
func (h *Handlers) DeleteKnownBot(w http.ResponseWriter, r *http.Request) {

	botID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid bot ID")
		return
	}

	if err := h.Service.DeleteKnownBot(r.Context(), botID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}
