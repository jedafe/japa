// Package comments — HTTP handlers for the unified comment system (Section 49).
package comments

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/httpx"
	"github.com/jedafe/japa/internal/spam"
	"github.com/jedafe/japa/internal/tenant"
)

const defaultPageSize int32 = 20

// Handlers holds dependencies for comment HTTP handlers.
type Handlers struct {
	Service      *Service
	SpamPipeline *spam.SpamPipeline
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// SetSpamPipeline injects the spam detection pipeline for comment safety checks.
func (h *Handlers) SetSpamPipeline(p *spam.SpamPipeline) {
	h.SpamPipeline = p
}

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > 100 {
		return 100
	}

	return int32(n) //nolint:gosec // G109: bounded by max check above
}

// ---------------------------------------------------------------------------
// Comment Handlers
// ---------------------------------------------------------------------------

// ListComments handles GET /api/v1/content/{id}/comments.
func (h *Handlers) ListComments(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	limit := parsePageSize(r)

	var offset int32

	if os := r.URL.Query().Get("page_offset"); os != "" {
		n, err := strconv.Atoi(os)
		if err == nil && n >= 0 {
			offset = int32(n) //nolint:gosec // G109: validated
		}
	}

	parentIDStr := r.URL.Query().Get("parent_id")
	if parentIDStr != "" {
		var parentID pgtype.UUID
		if err := parentID.Scan(parentIDStr); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_parent_id", "Invalid parent_id format")
			return
		}

		replies, err := h.Service.ListRepliesByParent(r.Context(), tenantID, parentID, limit, offset)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": replies})

		return
	}

	comments, err := h.Service.ListCommentsByContent(r.Context(), tenantID, contentID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": comments})
}

// GetComment handles GET /api/v1/comments/{id}.
func (h *Handlers) GetComment(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	commentID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comment ID")
		return
	}

	comment, err := h.Service.GetComment(r.Context(), tenantID, commentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": comment})
}

// CreateComment handles POST /api/v1/content/{id}/comments.
func (h *Handlers) CreateComment(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		ParentID *string `json:"parent_id"`
		Body     string  `json:"body"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Body == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_body", "Comment body is required")
		return
	}

	createInput := CreateCommentInput{
		ContentID: contentID,
		Body:      input.Body,
	}

	if input.ParentID != nil && *input.ParentID != "" {
		if err := createInput.ParentID.Scan(*input.ParentID); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_parent_id", "Invalid parent_id format")
			return
		}
	}

	// Spam pipeline check — multi-layer content safety (BE-034).
	if h.SpamPipeline != nil {
		clientIP := r.Header.Get("X-Forwarded-For")
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}
		spamResult, err := h.SpamPipeline.Check(r.Context(), tenantID, spam.SpamCheckRequest{
			Content:    input.Body,
			UserID:     user.UserID,
			IP:         clientIP,
			UserType:   "existing",
			EntityType: "comment",
		})
		if err != nil {
			slog.Warn("spam check failed", "error", err)
		} else if spamResult.Blocked {
			httpx.WriteError(w, http.StatusForbidden, "content_blocked", "Content blocked by safety filters")
			return
		}
	}

	comment, err := h.Service.CreateComment(r.Context(), tenantID, user.UserID, createInput)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": comment})
}

// UpdateComment handles PATCH /api/v1/comments/{id}.
func (h *Handlers) UpdateComment(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	commentID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		Body string `json:"body"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Body == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_body", "Comment body is required")
		return
	}

	comment, err := h.Service.UpdateComment(r.Context(), tenantID, commentID, user.UserID, input.Body)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": comment})
}

// DeleteComment handles DELETE /api/v1/comments/{id}.
func (h *Handlers) DeleteComment(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	commentID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.SoftDeleteComment(r.Context(), tenantID, commentID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// VoteComment handles POST /api/v1/comments/{id}/vote.
func (h *Handlers) VoteComment(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	commentID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		Value int16 `json:"value"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if err := h.Service.VoteComment(r.Context(), tenantID, commentID, user.UserID, input.Value); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"voted": true}})
}

// RemoveVote handles DELETE /api/v1/comments/{id}/vote.
func (h *Handlers) RemoveVote(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	commentID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.RemoveCommentVote(r.Context(), commentID, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// PinComment handles POST /api/v1/comments/{id}/pin.
func (h *Handlers) PinComment(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	commentID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.PinComment(r.Context(), tenantID, commentID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"pinned": true}})
}

// UnpinComment handles DELETE /api/v1/comments/{id}/pin.
func (h *Handlers) UnpinComment(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	commentID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.UnpinComment(r.Context(), tenantID, commentID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// handleServiceError translates a Service-layer error into the appropriate HTTP
// response.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
	case errors.Is(err, errpkg.ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, "already_exists", "Resource already exists")
	case errors.Is(err, errpkg.ErrMaxDepth):
		httpx.WriteError(w, http.StatusBadRequest, "depth_exceeded", err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}
