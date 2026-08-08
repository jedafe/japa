// Package ama — handlers.go provides the HTTP handlers for the AMA
// thread format routes (§88, ADR-096).
//
// Routes:
//
//	GET    /api/v1/forum/ama        — list active AMA sessions
//	GET    /api/v1/forum/ama/:id    — single AMA thread
//	PATCH  /api/v1/forum/ama/:id    — close AMA (host or moderator)
//
// AMA thread creation goes through the existing POST /forum/threads endpoint
// with thread_type=ama and ama_config in the payload. The forum.ama.create
// permission is checked there.
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
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes the AMA endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates AMA handlers.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
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

func parsePageParams(r *http.Request) (page, limit int32) {
	page = 1
	limit = 20
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = int32(n)
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n)
		}
	}
	return page, limit
}

// HandleList handles GET /forum/ama — list active AMA sessions.
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	page, limit := parsePageParams(r)
	threads, total, err := h.service.ListActive(r.Context(), tenantID, page, limit)
	if err != nil {
		slog.Error("ama: list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "list_failed", "Failed to list AMA sessions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": threads,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
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

// HandleClose handles PATCH /forum/ama/:id — close an AMA session.
// The host (thread author) or a moderator with forum.ama.moderate can close.
func (h *Handlers) HandleClose(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
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

	if err := h.service.CloseAMA(r.Context(), threadID, tenantID); err != nil {
		slog.Error("ama: close failed", "error", err)
		writeError(w, http.StatusInternalServerError, "close_failed", "Failed to close AMA session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"closed": true}})
}
