// Package rag — handlers.go provides the admin HTTP handlers for the RAG
// knowledge layer management routes.
//
// Routes (Section 69, ADR-069):
//
//	POST   /api/v1/admin/rag/reindex            — queue full corpus re-index
//	GET    /api/v1/admin/rag/status             — index stats (total chunks, last indexed)
//	DELETE /api/v1/admin/rag/entity/:type/:id   — purge embeddings for an entity
//	GET    /api/v1/admin/settings/rag           — get RAG settings
//	PUT    /api/v1/admin/settings/rag           — update RAG settings
//
// Ref: japaconnect-engineering/sections/69-rag-knowledge-layer.md
//
//	japaconnect-engineering/adr/ADR-069-japabot-rag-knowledge-layer.md
package rag

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	errpkg "github.com/jedafe/japa/internal/errors"
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


func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return u, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
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

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "rag_unavailable", "RAG service not configured")
		return
	}

	if err := h.service.QueueReindex(r.Context()); err != nil {
		slog.Error("rag: reindex failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "reindex_failed", "Failed to queue re-index")

		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
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

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "rag_unavailable", "RAG service not configured")
		return
	}

	stats, err := h.service.GetStats(r.Context())
	if err != nil {
		slog.Error("rag: status failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "stats_failed", "Failed to get RAG stats")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": stats,
	})
}

// HandlePurgeEntity handles DELETE /api/v1/admin/rag/entity/:type/:id —
// purge all embeddings for a specific entity. Gated by ai.rag.manage.
//
// The :id parameter accepts either a UUID (purges a single entity) or the
// sentinel "all" (purges every embedding for the given entity_type within
// the tenant). The "all" sentinel supports the admin dashboard's by-type
// aggregate purge, which operates at the entity-type level, not the
// individual-entity level.
func (h *Handlers) HandlePurgeEntity(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	entityType := chi.URLParam(r, "type")
	if entityType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_type", "Entity type is required")
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "rag_unavailable", "RAG service not configured")
		return
	}

	// Sentinel "all" → purge every embedding for this entity_type.
	idParam := chi.URLParam(r, "id")
	if idParam == "all" {
		if err := h.service.PurgeEntityType(r.Context(), entityType); err != nil {
			slog.Error("rag: purge by type failed", "error", err, "entity_type", entityType)
			errpkg.WriteError(w, http.StatusInternalServerError, "purge_failed", "Failed to purge entity type embeddings")

			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"purged": true,
			},
		})

		return
	}

	entityID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.PurgeEntity(r.Context(), entityType, entityID); err != nil {
		slog.Error("rag: purge failed", "error", err, "entity_type", entityType)
		errpkg.WriteError(w, http.StatusInternalServerError, "purge_failed", "Failed to purge entity embeddings")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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


	if h.settings == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "settings_unavailable", "Settings service not configured")
		return
	}

	ctx := r.Context()
	settings := map[string]any{
		"rag.enabled":          h.settings.GetBoolOr(ctx, "rag.enabled", true),
		"rag.embedding_model":  h.settings.GetStringOr(ctx, "rag.embedding_model", "text-embedding-3-small"),
		"rag.retrieval_top_k":  h.settings.GetIntOr(ctx, "rag.retrieval_top_k", 5),
		"rag.chunk_max_tokens": h.settings.GetIntOr(ctx, "rag.chunk_max_tokens", 400),
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": settings,
	})
}

// HandleUpdateSettings — PUT /api/v1/admin/settings/rag
//
// Saves the four RAG system settings (rag.enabled, rag.embedding_model,
// rag.retrieval_top_k, rag.chunk_max_tokens). The request body is a flat
// map of key→value pairs using the short key (without the "rag." prefix),
// matching the pattern of the generic admin settings PUT handler.
// Ref: japaconnect-engineering/sections/69-rag-knowledge-layer.md
//
//	japaconnect-engineering/conventions/system-settings-catalog.md
func (h *Handlers) HandleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.settings == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "settings_unavailable", "Settings service not configured")
		return
	}

	var updates map[string]any
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if len(updates) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "empty_body", "No settings provided")
		return
	}

	// Validate keys against the four allowed RAG settings.
	validKeys := map[string]bool{
		"enabled":          true,
		"embedding_model":  true,
		"retrieval_top_k":  true,
		"chunk_max_tokens": true,
	}

	ctx := r.Context()
	pending := make(map[string][]byte, len(updates))

	for key, value := range updates {
		if !validKeys[key] {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_key", "Unknown setting key: "+key)
			return
		}

		// Validate type + range per setting key (BUG-002 fix).
		switch key {
		case "enabled":
			if _, ok := value.(bool); !ok {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_value",
					"rag.enabled must be a boolean")
				return
			}
		case "embedding_model":
			model, ok := value.(string)
			if !ok || model == "" {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_value",
					"rag.embedding_model must be a non-empty string")
				return
			}
		case "retrieval_top_k":
			topK, ok := value.(float64)
			if !ok {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_value",
					"rag.retrieval_top_k must be an integer")
				return
			}

			if topK < 1 || topK > 50 || topK != float64(int(topK)) {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_value",
					"rag.retrieval_top_k must be an integer between 1 and 50")
				return
			}
		case "chunk_max_tokens":
			chunkMax, ok := value.(float64)
			if !ok {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_value",
					"rag.chunk_max_tokens must be an integer")
				return
			}

			if chunkMax < 50 || chunkMax > 2000 || chunkMax != float64(int(chunkMax)) {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_value",
					"rag.chunk_max_tokens must be an integer between 50 and 2000")
				return
			}
		}

		valueJSON, err := json.Marshal(value)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_value", "Cannot serialize value for key: "+key)
			return
		}

		pending["rag."+key] = valueJSON
	}

	if err := h.settings.UpdateSettingsBatch(ctx, pending); err != nil {
		slog.Error("rag: failed to save settings", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to save settings")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": nil})
}
