// Package roadmap — HTTP handlers for versioned releases (§113, ADR-135).
//
// Public routes serve the release timeline, per-release changelog detail,
// the unreleased commit queue, and the RSS feed. Admin routes manage the
// release lifecycle, item attachments, and the entries triage queue.
//
// Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
//
//	(Versioned Releases, Commit-Sourced Entries)
package roadmap

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/clientip"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/rss/model"
)

// ---------------------------------------------------------------------------
// Public
// ---------------------------------------------------------------------------

// ListReleases handles GET /api/v1/roadmap/releases — published releases
// grouped payload with computed support_state + unreleased_count.
func (h *Handlers) ListReleases(w http.ResponseWriter, r *http.Request) {
	out, err := h.Service.ListPublicReleases(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

// GetUnreleasedEntries handles GET /api/v1/roadmap/releases/unreleased —
// the public-type commit queue ("In development").
func (h *Handlers) GetUnreleasedEntries(w http.ResponseWriter, r *http.Request) {
	entries, err := h.Service.ListUnreleasedEntries(r.Context(), r.URL.Query().Get("scope"))
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if entries == nil {
		entries = make([]EntryDTO, 0)
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": entries})
}

// GetRelease handles GET /api/v1/roadmap/releases/{slug} — release detail
// with notes body, shipped items, and commit entries.
func (h *Handlers) GetRelease(w http.ResponseWriter, r *http.Request) {
	detail, err := h.Service.GetReleaseBySlug(r.Context(), chi.URLParam(r, "slug"), r.URL.Query().Get("scope"))
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": detail})
}

// GetReleasesFeed handles GET /api/v1/roadmap/releases/feed.xml — RSS 2.0
// of published releases. SiteURL/SiteName are set at wiring time. Renders
// through the shared internal/rss/model so all platform feeds emit the same
// XML shape (RSS-011); §26 requires Cache-Control max-age=900 on all feeds.
func (h *Handlers) GetReleasesFeed(w http.ResponseWriter, r *http.Request) {
	out, err := h.Service.ListPublicReleases(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Respect trusted X-Forwarded-Host like the other feed handlers so feed
	// links carry the public host rather than the configured fallback.
	siteURL := clientip.ForwardedBaseURL(r, h.SiteURL)

	items := make([]model.Item, 0, len(out.Releases))
	for _, rel := range out.Releases {
		title := rel.Version
		if rel.Title != nil && *rel.Title != "" {
			title = fmt.Sprintf("%s — %s", rel.Version, *rel.Title)
		}
		item := model.Item{
			Title:    title,
			Link:     fmt.Sprintf("%s/roadmap/releases/%s", siteURL, rel.Slug),
			GUID:     model.NewGUID(fmt.Sprintf("%s/roadmap/releases/%s", siteURL, rel.Slug), true),
			Category: rel.Surface,
		}
		if rel.Summary != nil {
			item.Description = *rel.Summary
		}
		if rel.ReleasedAt != nil {
			item.PubDate = rel.ReleasedAt.Format(time.RFC1123Z)
		}
		items = append(items, item)
	}

	xmlBytes := model.Render(
		h.SiteName+" Releases",
		siteURL+"/roadmap/releases",
		"Product releases for "+h.SiteName,
		"en",
		siteURL+"/roadmap/releases/feed.xml",
		items,
	)
	if xmlBytes == nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to render feed")
		return
	}

	etag := model.ETag(xmlBytes)
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=900")
	w.Header().Set("ETag", etag)

	if model.IfNoneMatch(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(xmlBytes)
}

// ---------------------------------------------------------------------------
// Admin — releases
// ---------------------------------------------------------------------------

// releaseRequest is the create/patch body for admin release endpoints.
type releaseRequest struct {
	Version    *string `json:"version"`
	Slug       *string `json:"slug"`
	Title      *string `json:"title"`
	Summary    *string `json:"summary"`
	Body       *string `json:"body"`
	Surface    *string `json:"surface"`
	IsESR      *bool   `json:"is_esr"`
	ReleasedAt *string `json:"released_at"`
	EOLAt      *string `json:"eol_at"`
	SortOrder  *int32  `json:"sort_order"`
	Status     *string `json:"status"`
}

func (r releaseRequest) toInput() ReleaseInput {
	return ReleaseInput{
		Version: r.Version, Slug: r.Slug, Title: r.Title, Summary: r.Summary,
		Body: r.Body, Surface: r.Surface, IsESR: r.IsESR,
		ReleasedAt: r.ReleasedAt, EOLAt: r.EOLAt, SortOrder: r.SortOrder,
		Status: r.Status,
	}
}

// AdminListReleases handles GET /api/v1/admin/roadmap/releases.
func (h *Handlers) AdminListReleases(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}
	q := r.URL.Query()
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	pageOffset, _ := strconv.Atoi(q.Get("page_offset"))
	out, err := h.Service.AdminListReleases(r.Context(), AdminReleaseListFilter{
		Surface:        q.Get("surface"),
		Status:         q.Get("status"),
		IncludeDeleted: q.Get("include_deleted") == "true",
		PageSize:       int32(pageSize),
		PageOffset:     int32(pageOffset),
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

// AdminGetRelease handles GET /api/v1/admin/roadmap/releases/{id}.
func (h *Handlers) AdminGetRelease(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid release id")
		return
	}
	rel, err := h.Service.AdminGetRelease(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rel})
}

// AdminCreateRelease handles POST /api/v1/admin/roadmap/releases.
func (h *Handlers) AdminCreateRelease(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}
	var input releaseRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Version == nil || *input.Version == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_version", "version is required")
		return
	}
	rel, err := h.Service.AdminCreateRelease(r.Context(), input.toInput())
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": rel})
}

// AdminUpdateRelease handles PATCH /api/v1/admin/roadmap/releases/{id}.
func (h *Handlers) AdminUpdateRelease(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid release id")
		return
	}
	var input releaseRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	rel, err := h.Service.AdminUpdateRelease(r.Context(), id, input.toInput())
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rel})
}

// AdminSetReleaseStatus handles PATCH /admin/roadmap/releases/{id}/status
// — draft→published→yanked; {absorb_unreleased} sweeps the entry queue on
// publish.
func (h *Handlers) AdminSetReleaseStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid release id")
		return
	}
	var input struct {
		Status           string `json:"status"`
		AbsorbUnreleased *bool  `json:"absorb_unreleased"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	absorb := true
	if input.AbsorbUnreleased != nil {
		absorb = *input.AbsorbUnreleased
	}
	rel, err := h.Service.AdminSetReleaseStatus(r.Context(), id, ReleaseStatus(input.Status), absorb)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rel})
}

// AdminDeleteRelease handles DELETE /admin/roadmap/releases/{id} — soft
// delete; ?force=true hard-deletes (gated by admin.hard_delete upstream).
func (h *Handlers) AdminDeleteRelease(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid release id")
		return
	}
	if err := h.Service.AdminDeleteRelease(r.Context(), id, r.URL.Query().Get("force") == "true"); err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// AdminRestoreRelease handles POST /admin/roadmap/releases/{id}/restore.
func (h *Handlers) AdminRestoreRelease(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid release id")
		return
	}
	if err := h.Service.AdminRestoreRelease(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// AdminReplaceReleaseItems handles PUT /admin/roadmap/releases/{id}/items
// — replaces the shipped-item attachment set.
func (h *Handlers) AdminReplaceReleaseItems(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid release id")
		return
	}
	var input struct {
		ItemIDs []string `json:"item_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	ids := make([]pgtype.UUID, 0, len(input.ItemIDs))
	for _, raw := range input.ItemIDs {
		var uid pgtype.UUID
		if err := uid.Scan(raw); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_item_id", "Invalid item UUID: "+raw)
			return
		}
		ids = append(ids, uid)
	}
	if err := h.Service.ReplaceReleaseItems(r.Context(), id, ids); err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Admin — entries
// ---------------------------------------------------------------------------

// AdminIngestEntries handles POST /admin/roadmap/releases/entries/ingest
// — GitHub push-shape commits → parsed/deduped entries.
func (h *Handlers) AdminIngestEntries(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}
	var input struct {
		Commits []GitHubCommit `json:"commits"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	created, skipped, err := h.Service.IngestCommits(r.Context(), input.Commits)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"created": created, "skipped": skipped,
	}})
}

// AdminListEntries handles GET /admin/roadmap/releases/entries.
func (h *Handlers) AdminListEntries(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}
	q := r.URL.Query()
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	pageOffset, _ := strconv.Atoi(q.Get("page_offset"))
	out, err := h.Service.AdminListEntries(r.Context(), EntryFilter{
		Unreleased: q.Get("unreleased") == "true",
		CommitType: q.Get("type"),
		Scope:      q.Get("scope"),
		PageSize:   int32(pageSize),
		PageOffset: int32(pageOffset),
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

// AdminUpdateEntry handles PATCH /admin/roadmap/releases/entries/{id}.
func (h *Handlers) AdminUpdateEntry(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid entry id")
		return
	}
	var input EntryUpdate
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	entry, err := h.Service.AdminUpdateEntry(r.Context(), id, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": entry})
}

// AdminDeleteEntry handles DELETE /admin/roadmap/releases/entries/{id}.
func (h *Handlers) AdminDeleteEntry(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid entry id")
		return
	}
	if err := h.Service.AdminDeleteEntry(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}
