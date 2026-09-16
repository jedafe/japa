package comments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
// (Section 49 — blog/non-forum ↔ discussion cross-links).
func (s *Service) ListDiscussions(ctx context.Context, contentID pgtype.UUID, limit, offset int32) ([]DiscussionSummary, int32, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := s.read.ListContentThreadLinks(ctx, gen.ListContentThreadLinksParams{
		ContentID: contentID,

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

	total, err := s.read.CountContentThreadLinks(ctx,
		contentID,
	)
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
		// Validate the existing thread exists in the same platform before linking.
		if _, err := s.read.GetContentByID(ctx,
			input.ThreadID,
		); err != nil {
			return StartConversationResult{}, fmt.Errorf("resolving existing thread: %w", err)
		}

		threadID = input.ThreadID
	} else {
		// Derive a new discussion thread from the blog/content.
		blog, err := s.read.GetContentByID(ctx,
			contentID,
		)
		if err != nil {
			return StartConversationResult{}, fmt.Errorf("fetching content to derive discussion: %w", err)
		}

		title := "Discussing: " + blog.Title

		thread, err := s.write.CreateContent(ctx, gen.CreateContentParams{

			AuthorID:    userID,
			ContentType: "discussion",
			Title:       title,
			Slug:        content.Slugify(title),
			// Body is empty — users add comments to the thread.
			Body:       "",
			BodyHtml:   "",
			GeoSpaceID: blog.GeoSpaceID,
			SpaceID:    blog.SpaceID,
			Status:     "published",
			Visibility: blog.Visibility,
		})
		if err != nil {
			return StartConversationResult{}, fmt.Errorf("creating discussion thread: %w", err)
		}

		threadID = thread.ID
		createdThread = thread
	}

	link, err := s.write.CreateContentThreadLink(ctx, gen.CreateContentThreadLinkParams{

		ContentID: contentID,
		ThreadID:  threadID,
	})
	if err != nil {
		return StartConversationResult{}, fmt.Errorf("creating content thread link: %w", err)
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

	discussions, total, err := h.Service.ListDiscussions(r.Context(), contentID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

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
	ok = true
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

	result, err := h.Service.StartConversation(r.Context(), contentID, user.UserID, in)
	if err != nil {
		// GetContentByID returns ErrNotFound / pgx.ErrNoRows when the thread/content is missing.
		if errors.Is(err, errpkg.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Content or thread not found")
			return
		}

		handleServiceError(w, err)

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
