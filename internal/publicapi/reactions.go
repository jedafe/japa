package publicapi

// Key-scoped reaction reads/writes — parity with the session reaction API
// (§14). The session-side semantics are reused through the adapter:
// content visibility, enabled reaction-type validation, the §89 achievement
// curated set, and engagement-count enqueue.

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// PublicReaction is the wire shape returned by reaction writes.
type PublicReaction struct {
	EntityType  string `json:"entity_type"`
	EntityID    string `json:"entity_id"`
	ReactionKey string `json:"reaction_key"`
	CreatedAt   string `json:"created_at"`
}

// PublicReactionCount is one row of the per-key aggregate response.
type PublicReactionCount struct {
	ReactionKey string `json:"reaction_key"`
	Count       int64  `json:"count"`
}

// ReactionService is the narrow seam into the session-side reaction domain,
// satisfied by an adapter in internal/server wrapping content.Service.
// Implementations must return errpkg.ErrNotFound for content that is absent
// or invisible to the key owner, and errpkg.ErrInvalid for unknown or
// disabled reaction keys.
type ReactionService interface {
	ListReactions(ctx context.Context, contentID pgtype.UUID) ([]PublicReactionCount, error)
	AddReaction(ctx context.Context, userID, contentID pgtype.UUID, reactionKey string) (PublicReaction, error)
	RemoveReaction(ctx context.Context, userID, contentID pgtype.UUID, reactionKey string) error
}

// SetReactionService injects the reaction adapter.
func (h *Handlers) SetReactionService(svc ReactionService) {
	h.reactions = svc
}

// parseUUID is shared with the rest of the publicapi handlers.
func reactionContentID(r *http.Request) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := id.Scan(chi.URLParam(r, "id"))
	return id, err
}

// ListPublicReactions handles GET /api/v1/public/content/{id}/reactions.
// Scope: api.read.content.
func (h *Handlers) ListPublicReactions(w http.ResponseWriter, r *http.Request) {
	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}
	if h.reactions == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "reactions_unavailable", "Reaction service unavailable")
		return
	}

	contentID, err := reactionContentID(r)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid content ID")
		return
	}

	counts, err := h.reactions.ListReactions(r.Context(), contentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if counts == nil {
		counts = []PublicReactionCount{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": counts})
}

// CreatePublicReaction handles POST /api/v1/public/content/{id}/reactions.
// Scope: api.write.reactions.
func (h *Handlers) CreatePublicReaction(w http.ResponseWriter, r *http.Request) {
	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}
	if h.reactions == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "reactions_unavailable", "Reaction service unavailable")
		return
	}

	contentID, err := reactionContentID(r)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid content ID")
		return
	}

	var input struct {
		ReactionKey string `json:"reaction_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.ReactionKey == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_reaction_key", "reaction_key is required")
		return
	}

	reaction, err := h.reactions.AddReaction(r.Context(), keyCtx.UserID, contentID, input.ReactionKey)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": reaction})
}

// DeletePublicReaction handles DELETE /api/v1/public/content/{id}/reactions/{key}.
// Scope: api.write.reactions.
func (h *Handlers) DeletePublicReaction(w http.ResponseWriter, r *http.Request) {
	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}
	if h.reactions == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "reactions_unavailable", "Reaction service unavailable")
		return
	}

	contentID, err := reactionContentID(r)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid content ID")
		return
	}

	key := chi.URLParam(r, "key")
	if key == "" {
		key = r.URL.Query().Get("key")
	}
	if key == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_key", "Reaction key is required")
		return
	}

	if err := h.reactions.RemoveReaction(r.Context(), keyCtx.UserID, contentID, key); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}
