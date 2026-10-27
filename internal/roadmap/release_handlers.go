// Package roadmap — HTTP handlers for versioned releases (§113, ADR-135).
//
// Public routes serve the release timeline, per-release changelog detail,
// the unreleased commit queue, and the RSS feed. Admin routes manage the
// release lifecycle, item attachments, and the entries triage queue.
//
// Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
//      (Versioned Releases, Commit-Sourced Entries)
package roadmap

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
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
// of published releases. SiteURL/SiteName are set at wiring time.
func (h *Handlers) GetReleasesFeed(w http.ResponseWriter, r *http.Request) {
	out, err := h.Service.ListPublicReleases(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<rss version="2.0"><channel>`)
	fmt.Fprintf(&b, "<title>%s Releases</title>", xmlEscape(h.SiteName))
	fmt.Fprintf(&b, "<link>%s/roadmap/releases</link>", xmlEscape(h.SiteURL))
	fmt.Fprintf(&b, "<description>Product releases for %s</description>", xmlEscape(h.SiteName))
	for _, rel := range out.Releases {
		title := rel.Version
		if rel.Title != nil && *rel.Title != "" {
			title = fmt.Sprintf("%s — %s", rel.Version, *rel.Title)
		}
		b.WriteString("<item>")
		fmt.Fprintf(&b, "<title>%s</title>", xmlEscape(title))
		fmt.Fprintf(&b, "<link>%s/roadmap/releases/%s</link>", xmlEscape(h.SiteURL), xmlEscape(rel.Slug))
		fmt.Fprintf(&b, "<guid isPermaLink=\"true\">%s/roadmap/releases/%s</guid>", xmlEscape(h.SiteURL), xmlEscape(rel.Slug))
		if rel.Summary != nil {
			fmt.Fprintf(&b, "<description>%s</description>", xmlEscape(*rel.Summary))
		}
		if rel.ReleasedAt != nil {
			fmt.Fprintf(&b, "<pubDate>%s</pubDate>", rel.ReleasedAt.Format(time.RFC1123Z))
		}
		fmt.Fprintf(&b, "<category>%s</category>", xmlEscape(rel.Surface))
		b.WriteString("</item>")
	}
	b.WriteString("</channel></rss>")
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

// xmlEscape escapes the five XML entities for RSS output.
func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", ">", "&gt;",
		`"`, "&quot;", "'", "&apos;",
	)
	return r.Replace(s)
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
