package publicapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// parseLimit reads the ?limit query parameter (Phase 2 pagination contract),
// clamped to maxSize. ?page_size is accepted as an alias so both pagination
// vocabularies work on every public endpoint.
func parseLimit(r *http.Request, defaultSize, maxSize int32) int32 {
	ps := defaultSize

	v := r.URL.Query().Get("limit")
	if v == "" {
		v = r.URL.Query().Get("page_size")
	}
	if v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil && n > 0 {
			if n > int64(maxSize) {
				ps = maxSize
			} else {
				ps = int32(n)
			}
		}
	}

	return ps
}

// ---------------------------------------------------------------------------
// Feeds (agent data API Phase 2)
// Ref: docs/superpowers/specs/2026-09-24-agent-data-api-phase2-design.md
// ---------------------------------------------------------------------------

// GetExploreFeed handles GET /api/v1/public/feeds/{name}.
// name ∈ {popular, latest, rising}; ?sort= overrides the name mapping,
// ?limit&offset paginate.
func (h *Handlers) GetExploreFeed(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}

	limit := parseLimit(r, defaultPageSize, 100)
	offset := parseOffset(r)

	items, err := h.Service.ExploreFeed(r.Context(), chi.URLParam(r, "name"),
		r.URL.Query().Get("sort"), limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": items, "limit": limit, "offset": offset,
	})
}

// GetTagFeed handles GET /api/v1/public/feeds/tag/{tag}.
// Cursor-paginated: ?cursor&limit. next_cursor is emitted only when the
// page is full.
func (h *Handlers) GetTagFeed(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}

	tag := chi.URLParam(r, "tag")
	if tag == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_tag", "Tag slug is required")
		return
	}

	limit := parseLimit(r, defaultPageSize, 100)

	items, nextCursor, err := h.Service.TagFeed(r.Context(), tag,
		r.URL.Query().Get("cursor"), limit)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "tag_not_found", "Tag not found")
			return
		}
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": items, "next_cursor": nextCursor, "limit": limit,
	})
}

// GetSpaceFeed handles GET /api/v1/public/feeds/space/{slug}.
// The key owner's UserID is passed as the viewer so member-only items
// resolve exactly as the web space feed does.
func (h *Handlers) GetSpaceFeed(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Space slug is required")
		return
	}

	limit := parseLimit(r, defaultPageSize, 100)
	offset := parseOffset(r)
	keyCtx := APIKeyFromContext(r.Context())

	items, err := h.Service.SpaceFeed(r.Context(), keyCtx.UserID, slug,
		r.URL.Query().Get("sort"), limit, offset)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Space not found")
			return
		}
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": items, "limit": limit, "offset": offset,
	})
}

// GetMyFeed handles GET /api/v1/public/me/feed — the key owner's ranked
// for-you feed. The user identity comes from the API key context, never a
// query parameter.
func (h *Handlers) GetMyFeed(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}

	keyCtx := APIKeyFromContext(r.Context())
	limit := parseLimit(r, defaultPageSize, 100)
	offset := parseOffset(r)

	items, err := h.Service.MyFeed(r.Context(), keyCtx.UserID,
		r.URL.Query().Get("sort"), limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": items, "limit": limit, "offset": offset,
	})
}

// ---------------------------------------------------------------------------
// Markdown representation
// ---------------------------------------------------------------------------

// GetContentMarkdown handles GET /api/v1/public/content/{id}/markdown —
// the same published+public row as GetContentByID, rendered as
// text/markdown with a small front-matter block.
func (h *Handlers) GetContentMarkdown(w http.ResponseWriter, r *http.Request) {
	if h.Queries == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	if !requireAPIKey(w, r) {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid content ID")
		return
	}

	content, err := h.Queries.GetPublishedContentByID(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.writeContentMarkdown(w, r, content)
}

// writeContentMarkdown writes the text/markdown representation shared by
// GetContentMarkdown and the Accept: text/markdown branch of
// GetContentByID. Author and space are best-effort lookups — resolution
// failures degrade to the author UUID / blank space rather than failing
// the request.
func (h *Handlers) writeContentMarkdown(w http.ResponseWriter, r *http.Request, content gen.GetPublishedContentByIDRow) {
	author := content.AuthorID.String()
	if u, err := h.Queries.GetPublicUserByID(r.Context(), content.AuthorID); err == nil {
		author = u.Username
	}

	space := ""
	if content.SpaceID.Valid {
		if s, err := h.Queries.GetSpaceByID(r.Context(), content.SpaceID); err == nil {
			space = s.Slug
		}
	}

	canonical := "/content/" + content.Slug
	if content.CanonicalUrl != nil && *content.CanonicalUrl != "" {
		canonical = *content.CanonicalUrl
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "---\ntitle: %q\nauthor: %s\nspace: %s\ncanonical: %s\n---\n\n%s",
		content.Title, author, space, canonical, content.Body)
}

// ---------------------------------------------------------------------------
// Scam alerts (safety surface)
// ---------------------------------------------------------------------------

// ListScamAlerts handles GET /api/v1/public/scam-alerts.
// Params: ?scam_type, ?destination_space_id (UUID), ?limit&offset.
func (h *Handlers) ListScamAlerts(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}

	destSpaceID, err := parseUUIDQuery(r, "destination_space_id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
		return
	}

	limit := parseLimit(r, defaultPageSize, 100)
	offset := parseOffset(r)

	reports, total, err := h.Service.ListScamAlerts(r.Context(), destSpaceID,
		r.URL.Query().Get("scam_type"), limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": reports, "total": total, "limit": limit, "offset": offset,
	})
}

// GetScamAlertSchema handles GET /api/v1/public/scam-alerts/schema —
// the canonical scam_type and platform option lists.
func (h *Handlers) GetScamAlertSchema(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": h.Service.ScamAlertSchema(),
	})
}

// GetScamAlert handles GET /api/v1/public/scam-alerts/{id} — a single
// confirmed scam report.
func (h *Handlers) GetScamAlert(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid scam report ID")
		return
	}

	report, err := h.Service.GetScamAlert(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "report_not_found", "Scam report not found")
			return
		}
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": report})
}

// ---------------------------------------------------------------------------
// Semantic search (premium)
// ---------------------------------------------------------------------------

// SemanticSearch handles GET /api/v1/public/search/semantic.
// Params: ?q (required), ?lang (default "en"), ?space (slug), ?limit (max 50).
func (h *Handlers) SemanticSearch(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}

	q := r.URL.Query()
	lang := strings.TrimSpace(q.Get("lang"))
	if lang == "" {
		lang = "en"
	}
	limit := parseLimit(r, defaultPageSize, 50)

	result, err := h.Service.SemanticSearch(r.Context(), q.Get("q"), lang,
		strings.TrimSpace(q.Get("space")), limit)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Space not found")
			return
		}
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, result)
}
