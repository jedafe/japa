package comments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// Content Thread Links (Section 49 — Comment Thread Link Flow)
//
// GET  /api/v1/content/{id}/discussions — list linked discussion threads
// POST /api/v1/content/{id}/discussions — start a conversation: create a
//   discussion thread derived from the blog/content (or link an existing
//   thread via { thread_id }) and record the content_thread_links cross-link.
// ---------------------------------------------------------------------------

// DiscussionSummary is the public shape for a linked discussion thread entry
// returned by GET /content/{id}/discussions.
type DiscussionSummary struct {
	ID                 pgtype.UUID        `json:"id"`
	ThreadID           pgtype.UUID        `json:"thread_id"`
	ThreadTitle        string             `json:"thread_title"`
	ThreadSlug         string             `json:"thread_slug"`
	ThreadCommentCount int32              `json:"thread_comment_count"`
	ThreadAuthorID     pgtype.UUID        `json:"thread_author_id"`
	LinkCount          int32              `json:"link_count"`
	CreatedAt          pgtype.Timestamptz `json:"created_at"`
}

// ListDiscussions returns the discussion threads linked to a content item
// (Section 49 — blog/non-forum ↔ discussion cross-links). Threads are
// filtered per-viewer: drafts and audience-restricted threads (members,
// private, private-space) are only listed for viewers who can see them —
// same predicate as CanViewComments (C-021 parity).
func (s *Service) ListDiscussions(ctx context.Context, contentID, viewerID pgtype.UUID, authed bool, limit, offset int32) ([]DiscussionSummary, int32, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := s.read.ListContentThreadLinks(ctx, gen.ListContentThreadLinksParams{
		ContentID: contentID,
		UserID:    viewerID,
		Authed:    authed,

		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing content thread links: %w", err)
	}

	out := make([]DiscussionSummary, len(rows))
	for i := range rows {
		out[i] = DiscussionSummary{
			ID:                 rows[i].ID,
			ThreadID:           rows[i].ThreadID,
			ThreadTitle:        rows[i].ThreadTitle,
			ThreadSlug:         rows[i].ThreadSlug,
			ThreadCommentCount: rows[i].ThreadCommentCount,
			ThreadAuthorID:     rows[i].ThreadAuthorID,
			LinkCount:          rows[i].LinkCount,
			CreatedAt:          rows[i].CreatedAt,
		}
	}

	total, err := s.read.CountContentThreadLinks(ctx, gen.CountContentThreadLinksParams{
		ContentID: contentID,
		UserID:    viewerID,
		Authed:    authed,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("counting content thread links: %w", err)
	}

	return out, total, nil
}

// StartConversationInput is the body for POST /content/{id}/discussions.
type StartConversationInput struct {
	// ThreadID, when set, links an existing discussion thread instead of
	// creating a new one.
	ThreadID pgtype.UUID
}

// StartConversationResult holds the cross-link plus the newly created thread
// (when one was created). CreatedThread is zero-valued (ID.Valid == false)
// when an existing thread was linked instead.
type StartConversationResult struct {
	Link          gen.ContentThreadLink
	CreatedThread gen.CreateContentRow
}

// StartConversation creates a discussion thread derived from the blog/content
// (or links an existing thread when ThreadID is supplied) and records the
// content_thread_links cross-link. Returns the link and the newly created
// thread (if any).
func (s *Service) StartConversation(ctx context.Context, contentID, userID pgtype.UUID, input StartConversationInput) (StartConversationResult, error) {
	var threadID pgtype.UUID

	var createdThread gen.CreateContentRow

	if input.ThreadID.Valid {
		// Validate the existing thread exists in the same platform before
		// linking, and that it is actually a discussion — linking an arbitrary
		// content item would surface a non-thread as a conversation (C-013).
		thread, err := s.read.GetContentByID(ctx,
			input.ThreadID,
		)
		if err != nil {
			return StartConversationResult{}, fmt.Errorf("resolving existing thread: %w", err)
		}

		if thread.ContentType != "discussion" {
			return StartConversationResult{}, fmt.Errorf("linked thread must be a discussion: %w", errpkg.ErrInvalid)
		}

		threadID = input.ThreadID
	} else {
		// Unified-discussion spec: the canonical conversation for a content
		// row is its thread_anchors `discussion` row — get-or-create it via
		// the anchor so this endpoint never mints a competing auto-titled
		// thread. CreateAnchor is idempotent and applies the same
		// status/visibility/space/flag gates as every mint path; a
		// discussion row resolves to itself.
		anchor, err := s.CreateAnchor(ctx, userID, "content", contentID.String())
		if err != nil {
			return StartConversationResult{}, fmt.Errorf("resolving canonical thread: %w", err)
		}
		threadID = anchor.ContentID
	}

	// A discussion row resolves to itself — linking it to itself would
	// double-count its own comments in RecountContentCommentCount's linked
	// subquery. Only real parent→thread pairs get a link row.
	var link gen.ContentThreadLink
	if threadID != contentID {
		var err error
		link, err = s.write.CreateContentThreadLink(ctx, gen.CreateContentThreadLinkParams{
			ContentID: contentID,
			ThreadID:  threadID,
		})
		if err != nil {
			return StartConversationResult{}, fmt.Errorf("creating content thread link: %w", err)
		}
	}

	return StartConversationResult{Link: link, CreatedThread: createdThread}, nil
}

// ---------------------------------------------------------------------------
// HTTP Handlers
// ---------------------------------------------------------------------------

// ListDiscussions handles GET /api/v1/content/{id}/discussions.
func (h *Handlers) ListDiscussions(w http.ResponseWriter, r *http.Request) {
	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	limit := parsePageSize(r)

	var offset int32

	if os := r.URL.Query().Get("page_offset"); os != "" {
		if n, err := strconv.Atoi(os); err == nil && n >= 0 {
			// #nosec G109 // value bounded by MaxPageSize check
			// #nosec G115 // value bounded by caller
			offset = int32(n)
		}
	}

	// C-021 parity: linked discussions are only as visible as the parent
	// content — 404 (never 403) so the endpoint cannot probe restricted
	// content for thread links.
	viewerID := viewerIDFromRequest(r)
	authed := auth.FromContext(r.Context()) != nil
	visible, err := h.Service.CanViewComments(r.Context(), contentID, viewerID, authed)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if !visible {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Content not found")
		return
	}

	discussions, total, err := h.Service.ListDiscussions(r.Context(), contentID, viewerID, authed, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &discussions)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": discussions,
		"meta": map[string]any{"total": total, "page_size": limit, "page_offset": offset},
	})
}

// StartConversation handles POST /api/v1/content/{id}/discussions.
func (h *Handlers) StartConversation(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		ThreadID *string `json:"thread_id"`
	}

	// Body is optional (omitting thread_id creates a new discussion).
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
			return
		}
	}

	var in StartConversationInput
	if input.ThreadID != nil && *input.ThreadID != "" {
		if err := in.ThreadID.Scan(*input.ThreadID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_thread_id", "Invalid thread_id format")
			return
		}
	}

	// C-021: starting a conversation on content the caller cannot see leaks
	// the content's existence (and spawns a public discussion on it).
	visible, err := h.Service.CanViewComments(r.Context(), contentID, user.UserID, true)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if !visible {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Content not found")
		return
	}

	// When a NEW discussion thread is being created it lands in the blog's
	// space — the caller must hold the space's posting_perm, not just
	// comments.create (C-013). Linking an existing thread adds no content.
	if h.spaceChecker != nil && !in.ThreadID.Valid {
		spaceID, err := h.Service.GetContentSpaceID(r.Context(), contentID)
		if err != nil {
			slog.Warn("comments: failed to resolve content space for posting_perm check", "content_id", contentID.String(), "error", err)
		} else if spaceID.Valid {
			allowed, err := h.spaceChecker.CanPostInSpace(r.Context(), spaceID, user.UserID)
			if err != nil {
				slog.Warn("comments: space posting_perm check failed", "space_id", spaceID.String(), "error", err)
			} else if !allowed {
				errpkg.WriteError(w, http.StatusForbidden, "space_permission_denied",
					spaceDenialMessage(r.Context(), h.spaceChecker, spaceID, user.UserID, "post"))
				return
			}
		}
	}

	result, err := h.Service.StartConversation(r.Context(), contentID, user.UserID, in)
	if err != nil {
		switch {
		// GetContentByID returns ErrNotFound / pgx.ErrNoRows when the thread/content is missing.
		case errors.Is(err, errpkg.ErrNotFound) || errors.Is(err, pgx.ErrNoRows):
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Content or thread not found")
		// Content-service sentinels the derived-thread path can surface
		// (mirror of the content handler mapping — these are not errpkg
		// sentinels so the default branch would 500 them).
		case errors.Is(err, content.ErrMissingPrimarySpace):
			errpkg.WriteError(w, http.StatusUnprocessableEntity, "missing_primary_space",
				"Content requires a primary space before publishing or scheduling")
		case errors.Is(err, content.ErrInvalidStatus):
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", err.Error())
		case errors.Is(err, content.ErrInvalidVisibility):
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_visibility", err.Error())
		case errors.Is(err, content.ErrDuplicateContent):
			errpkg.WriteError(w, http.StatusConflict, "duplicate_content", "Content with this body already exists")
		default:
			handleServiceError(w, err)
		}

		return
	}

	// Section 49 — fan out the new discussion thread to the author's
	// followers' feeds (BL-005). Only when a new thread was created (not
	// when linking an existing one). Fire-and-forget per Rule 8.
	if result.CreatedThread.ID.Valid {
		h.enqueueContentFanOut(
			result.CreatedThread.ID, user.UserID,
			result.CreatedThread.SpaceID, "discussion", "followers",
		)
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"id":         result.Link.ID,
		"thread_id":  result.Link.ThreadID,
		"link_count": result.Link.LinkCount,
	}})
}
