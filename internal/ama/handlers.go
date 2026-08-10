// Package ama — handlers.go provides the HTTP handlers for the AMA
// thread format routes (§88, ADR-096).
//
// Routes:
//
//	GET    /api/v1/forum/ama        — list active AMA sessions (?dest=&profession=&page_size=&cursor_created_at=)
//	GET    /api/v1/forum/ama/:id    — single AMA thread
//	POST   /api/v1/forum/ama        — create an AMA session (forum.ama.create)
//	PATCH  /api/v1/forum/ama/:id    — close AMA (host or forum.ama.moderate)
//
// AMA creation is owned by this package because the generic forum
// CreateForumThread sqlc query does not carry the ama_config JSONB column.
// The forum.ama.create permission is enforced via route middleware; the
// one-active-per-host and cooldown rate limits are enforced in the service.
//
// Ref: japaconnect-engineering/sections/88-ask-an-expat.md
//      japaconnect-engineering/adr/ADR-096-ask-an-expat.md
package ama

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes the AMA endpoints.
type Handlers struct {
	service *Service
}

// NewHandlers creates AMA handlers.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("ama: encode response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return u, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
		return pgtype.UUID{}, false
	}
	return id, true
}

// parsePageSize reads the page_size query param (default 20, max 100), matching
// the forum list pagination convention (cursor-based, not offset-based).
func parsePageSize(r *http.Request) int32 {
	limit := int32(20)
	if v := r.URL.Query().Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n)
		}
	}
	return limit
}

// parseCursor reads the cursor_created_at query param as a timestamptz.
func parseCursor(r *http.Request) pgtype.Timestamptz {
	v := r.URL.Query().Get("cursor_created_at")
	if v == "" {
		return pgtype.Timestamptz{}
	}
	var ts pgtype.Timestamptz
	if err := ts.Scan(v); err != nil {
		return pgtype.Timestamptz{}
	}
	return ts
}

// HandleList handles GET /forum/ama — list active AMA sessions.
// Supports the geo-aware ?dest= and ?profession= filters (§88 / §§79–101)
// and cursor-based pagination (?page_size=, ?cursor_created_at=).
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	dest := r.URL.Query().Get("dest")
	profession := r.URL.Query().Get("profession")
	pageSize := parsePageSize(r)
	cursor := parseCursor(r)

	dtos, err := h.service.ListActive(r.Context(), tenantID, dest, profession, pageSize, cursor)
	if err != nil {
		slog.Error("ama: list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "list_failed", "Failed to list AMA sessions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": dtos})
}

// HandleGet handles GET /forum/ama/:id — single AMA thread.
func (h *Handlers) HandleGet(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	threadID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	thread, err := h.service.GetByID(r.Context(), threadID, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "ama_not_found", "AMA thread not found")
			return
		}
		slog.Error("ama: get failed", "error", err)
		writeError(w, http.StatusInternalServerError, "get_failed", "Failed to get AMA thread")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": thread})
}

// HandleCreate handles POST /forum/ama — create an AMA session.
// The forum.ama.create permission is enforced by route middleware; the
// one-active-per-host and cooldown rate limits are enforced by the service.
func (h *Handlers) HandleCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Title                    string  `json:"title"`
		Slug                     string  `json:"slug"`
		Body                     []byte  `json:"body"`
		SpaceID                  *string `json:"space_id"`
		IsAnonymous              bool    `json:"is_anonymous"`
		HostProfession           string  `json:"host_profession"`
		HostDestCountryID        string  `json:"host_dest_country_id"`
		HostYearsAbroad          *int    `json:"host_years_abroad"`
		OpenDurationHours        *int    `json:"open_duration_hours"`
		QuestionApprovalRequired *bool   `json:"question_approval_required"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}
	if input.Slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	var spaceID pgtype.UUID
	if input.SpaceID != nil {
		if err := spaceID.Scan(*input.SpaceID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id format")
			return
		}
	}

	dto, err := h.service.CreateAMA(r.Context(), tenantID, user.UserID, CreateAMAInput{
		Title:                    input.Title,
		Slug:                     input.Slug,
		Body:                     input.Body,
		SpaceID:                  spaceID,
		IsAnonymous:              input.IsAnonymous,
		HostProfession:           input.HostProfession,
		HostDestCountryID:        input.HostDestCountryID,
		HostYearsAbroad:          input.HostYearsAbroad,
		OpenDurationHours:        input.OpenDurationHours,
		QuestionApprovalRequired: input.QuestionApprovalRequired,
	})
	if err != nil {
		handleCreateError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": dto})
}

// handleCreateError maps service errors to the standard error envelope.
func handleCreateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrActiveAMAExists):
		writeError(w, http.StatusConflict, "ama_active_exists", "You already have an active AMA session.")
	case errors.Is(err, ErrAMACooldown):
		writeError(w, http.StatusTooManyRequests, "ama_cooldown", "The AMA cooldown period has not elapsed.")
	case errors.Is(err, ErrCloseForbidden):
		writeError(w, http.StatusForbidden, "close_forbidden", "Only the host or a moderator may close this AMA.")
	default:
		slog.Error("ama: create failed", "error", err)
		writeError(w, http.StatusInternalServerError, "create_failed", "Failed to create AMA session")
	}
}

// HandleClose handles PATCH /forum/ama/:id — close an AMA session.
// The host (thread author) or a moderator with forum.ama.moderate may close
// (§88). Authorization is resolved in the service so the host can close
// without holding the moderator permission.
func (h *Handlers) HandleClose(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	threadID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	err := h.service.CloseAMA(r.Context(), threadID, tenantID, user.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "ama_not_found", "AMA thread not found")
			return
		}
		if errors.Is(err, ErrCloseForbidden) {
			writeError(w, http.StatusForbidden, "close_forbidden", "Only the host or a moderator may close this AMA.")
			return
		}
		slog.Error("ama: close failed", "error", err)
		writeError(w, http.StatusInternalServerError, "close_failed", "Failed to close AMA session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"closed": true}})
}
