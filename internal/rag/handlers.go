// Package rag — handlers.go provides the admin HTTP handlers for the RAG
// knowledge layer management routes.
//
// Routes (Section 69, ADR-069):
//
//	POST   /api/v1/admin/rag/reindex            — queue full corpus re-index
//	GET    /api/v1/admin/rag/status             — index stats (total chunks, last indexed)
//	DELETE /api/v1/admin/rag/entity/:type/:id   — purge embeddings for an entity
//	GET    /api/v1/admin/settings/rag           — get RAG settings
//
// Ref: japaconnect-engineering/sections/69-rag-knowledge-layer.md
//      japaconnect-engineering/adr/ADR-069-japabot-rag-knowledge-layer.md
package rag

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes the admin RAG management endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates RAG handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as university/handlers.go)
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("rag: encode response", "error", err)
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

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// HandleReindex handles POST /api/v1/admin/rag/reindex — queue a full corpus
// re-index. Gated by ai.rag.reindex permission (checked by RBAC middleware
// at the route registration site).
func (h *Handlers) HandleReindex(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "rag_unavailable", "RAG service not configured")
		return
	}

	if err := h.service.QueueReindex(r.Context(), tenantID); err != nil {
		slog.Error("rag: reindex failed", "error", err)
		writeError(w, http.StatusInternalServerError, "reindex_failed", "Failed to queue re-index")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]any{
			"status": "queued",
		},
	})
}

// HandleStatus handles GET /api/v1/admin/rag/status — index statistics.
// Gated by ai.rag.manage permission.
func (h *Handlers) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "rag_unavailable", "RAG service not configured")
		return
	}

	stats, err := h.service.GetStats(r.Context(), tenantID)
	if err != nil {
		slog.Error("rag: status failed", "error", err)
		writeError(w, http.StatusInternalServerError, "stats_failed", "Failed to get RAG stats")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": stats,
	})
}

// HandlePurgeEntity handles DELETE /api/v1/admin/rag/entity/:type/:id —
// purge all embeddings for a specific entity. Gated by ai.rag.manage.
func (h *Handlers) HandlePurgeEntity(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	entityType := chi.URLParam(r, "type")
	if entityType == "" {
		writeError(w, http.StatusBadRequest, "missing_type", "Entity type is required")
		return
	}

	entityID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "rag_unavailable", "RAG service not configured")
		return
	}

	if err := h.service.PurgeEntity(r.Context(), tenantID, entityType, entityID); err != nil {
		slog.Error("rag: purge failed", "error", err, "entity_type", entityType)
		writeError(w, http.StatusInternalServerError, "purge_failed", "Failed to purge entity embeddings")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"purged": true,
		},
	})
}

// HandleGetSettings handles GET /api/v1/admin/settings/rag — get RAG settings.
// Returns the 4 rag.* system_settings keys with their current values.
// Gated by ai.rag.manage permission.
func (h *Handlers) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "settings_unavailable", "Settings service not configured")
		return
	}

	ctx := r.Context()
	settings := map[string]any{
		"rag.enabled":         h.settings.GetBoolOr(ctx, tenantID, "rag.enabled", true),
		"rag.embedding_model": h.settings.GetStringOr(ctx, tenantID, "rag.embedding_model", "text-embedding-3-small"),
		"rag.retrieval_top_k": h.settings.GetIntOr(ctx, tenantID, "rag.retrieval_top_k", 5),
		"rag.chunk_max_tokens": h.settings.GetIntOr(ctx, tenantID, "rag.chunk_max_tokens", 400),
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": settings,
	})
}
