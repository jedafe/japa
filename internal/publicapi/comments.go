package publicapi

// Phase 4 — key-scoped comment writes (spec 2026-09-24-agent-data-api-phase3,
// Phase 4 scope: comments only). The session-side abuse model is reused
// wholesale through the adapter: spam pipeline, IP block middleware, space
// commenting/replying permissions, rate-limit recording.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/clientip"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// PublicComment is the wire shape returned by POST /public/content/{id}/comments.
type PublicComment struct {
	ID        string `json:"id"`
	ContentID string `json:"content_id"`
	ParentID  string `json:"parent_id,omitempty"`
	Depth     int32  `json:"depth"`
	CreatedAt string `json:"created_at"`
}

// ErrContentBlocked is returned when the spam pipeline rejects a comment body.
var ErrContentBlocked = errors.New("content blocked by safety filters")

// CommentService is the narrow seam into the session-side comment domain,
// satisfied by an adapter in internal/server that composes comments.Service,
// the spam pipeline, and the space permission checker. Implementations must
// return errpkg.ErrNotFound for content that is absent or not visible to the
// key owner (never 403 — the endpoint must not leak restricted content), and
// publicapi.ErrContentBlocked when the spam pipeline rejects the body.
type CommentService interface {
	// CreateComment creates a comment (or reply when parentID is valid)
	// authored by the API key owner. body is plain text — the adapter wraps
	// it in a TipTap paragraph document. clientIP feeds the spam pipeline's
	// IP-scoring layer.
	CreateComment(ctx context.Context, userID, contentID, parentID pgtype.UUID, body, clientIP string) (PublicComment, error)
}

// SetCommentService injects the comment adapter.
func (h *Handlers) SetCommentService(svc CommentService) {
	h.comments = svc
}

// CreatePublicComment handles POST /api/v1/public/content/{id}/comments.
// Requires api.write.comments (admin-granted — holding the scope is the
// approval, per §47).
func (h *Handlers) CreatePublicComment(w http.ResponseWriter, r *http.Request) {
	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}
	if h.comments == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content id format")
		return
	}

	var input struct {
		Body        string `json:"body"`
		ParentID    string `json:"parent_id"`
		IsAnonymous bool   `json:"is_anonymous"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if strings.TrimSpace(input.Body) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_body", "Comment body is required")
		return
	}

	// API-key calls are always identified — anonymous posting via key would
	// hide the author behind a named credential.
	if input.IsAnonymous {
		errpkg.WriteError(w, http.StatusBadRequest, "anonymous_not_allowed", "API keys cannot post anonymous comments")
		return
	}

	var parentID pgtype.UUID
	if input.ParentID != "" {
		if err := parentID.Scan(input.ParentID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_parent_id", "Invalid parent_id format")
			return
		}
	}

	comment, err := h.comments.CreateComment(r.Context(), keyCtx.UserID, contentID, parentID,
		input.Body, clientip.FromRequest(r))
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": comment})
}
