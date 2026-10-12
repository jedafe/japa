// Package wiki — handlers.go provides the HTTP handlers for the Destination
// Wiki routes (§80, ADR-088).
//
// Routes:
//
//	GET    /api/v1/wiki/:space_slug                        — wiki index for space
//	GET    /api/v1/wiki/:space_slug/:page_slug             — single wiki page
//	GET    /api/v1/wiki/:space_slug/:page_slug/history     — revision history
//	POST   /api/v1/wiki/:space_slug/:page_slug/revisions   — submit edit (auth, wiki.edit; ops[] allowed)
//	POST   /api/v1/wiki/suggest                            — entity-keyed field ops (auth)
//	GET    /api/v1/wiki/field-types                        — proposable custom-field types
//	GET    /api/v1/wiki/entities/:type/:id/fields          — editable field schema + values
//	GET    /api/v1/wiki/entities/:type/:id/revisions       — entity edit history
//	POST   /api/v1/admin/wiki/:page_id/revisions/:id/approve — approve revision (auth, wiki.review)
//	POST   /api/v1/admin/wiki/:page_id/revisions/:id/reject  — reject revision (auth, wiki.review)
//	GET    /api/v1/admin/wiki/revisions                    — review queue, ?kind=&status= (auth, wiki.review)
//	POST   /api/v1/admin/wiki/revisions/:id/approve        — revision-keyed approve (auth, wiki.review)
//	POST   /api/v1/admin/wiki/revisions/:id/reject         — revision-keyed reject (auth, wiki.review)
//	PATCH  /api/v1/admin/wiki/:page_id                     — lock/unlock, feature, entity bind (auth, wiki.manage)
//	DELETE /api/v1/admin/wiki/:page_id                     — soft delete; ?force=true hard delete (auth, wiki.delete)
//	POST   /api/v1/admin/wiki/generate                     — trigger Content Agent draft (auth, wiki.manage)
//	GET    /api/v1/admin/wiki                              — admin list (auth, wiki.manage)
//
// Ref: japaconnect-engineering/sections/80-destination-wiki.md
//
//	japaconnect-engineering/adr/ADR-088-destination-wiki.md
package wiki

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/rag"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/customfields"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
)

// literalResolver resolves {{...}} template literals in user-facing text
// fields (§14 Global Template Literals). Implemented by *literals.Catalog;
// nil disables resolution.
type literalResolver interface {
	ResolveFields(ctx context.Context, v any, rc literals.RenderContext)
}

// Handlers exposes the wiki endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
	points   pointsChecker
	perms    permissionChecker
	rag      ragIndexer
	lit      literalResolver // nil disables literal resolution
}

// NewHandlers creates wiki handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// pointsChecker returns the user's lifetime points (reputation) for the
// reputation gate on wiki edits (§80: wiki.edit is reputation-gated).
type pointsChecker interface {
	GetPointsBalance(ctx context.Context, userID pgtype.UUID) (int64, error)
}

// permissionChecker checks RBAC nodes at runtime. Used by the community_edits
// gate (§80): when wiki.community_edits is false, only wiki.review/wiki.manage
// holders may submit. Also used for the wiki.lock gate (B7).
type permissionChecker interface {
	HasPermission(ctx context.Context, userID pgtype.UUID, permKey string) bool
}

// ragIndexer is the subset of the RAG indexer that wiki needs to index
// approved revision content into content_embeddings (B8, §69/ADR-069).
// Implemented by rag.Service; nil disables RAG indexing (graceful degradation).
type ragIndexer interface {
	IndexEntity(ctx context.Context, entityType string, entityID pgtype.UUID, visibility string, spaceID, authorID pgtype.UUID, body, language string) error
	RecordIndexFailure(ctx context.Context, entityType string, entityID pgtype.UUID, operation string, indexErr error) error
}

// SetPointsChecker injects the points service for reputation checks on
// wiki edits. Called once during server wiring.
func (h *Handlers) SetPointsChecker(pc pointsChecker) {
	h.points = pc
}

// SetPermissionChecker injects the RBAC resolver for the community_edits gate.
// Called once during server wiring.
func (h *Handlers) SetPermissionChecker(pc permissionChecker) {
	h.perms = pc
}

// SetRAGIndexer injects the RAG indexer for embedding approved wiki content
// (B8). Called once during server wiring; nil disables RAG indexing.
func (h *Handlers) SetRAGIndexer(ri ragIndexer) {
	h.rag = ri
}

// SetLiteralResolver injects the template-literal catalog so wiki page
// titles and revision content render {{...}} placeholders (§14).
func (h *Handlers) SetLiteralResolver(l literalResolver) {
	h.lit = l
}

// resolveLiterals resolves {{...}} placeholders on a wiki DTO or slice.
// Best-effort — failures are logged inside the resolver.
func (h *Handlers) resolveLiterals(r *http.Request, v any) {
	if literals.IsRawRequest(r) {
		return
	}

	if h.lit == nil {
		return
	}

	h.lit.ResolveFields(r.Context(), v, literals.NewRequestContext(r, ""))
}

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

func parsePageParams(r *http.Request) (page, limit int32) {
	page = 1
	limit = 20

	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = int32(n) // #nosec
		}
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n) // #nosec
		}
	}

	return page, limit
}

// checkReputationGate enforces the wiki.min_reputation_to_edit threshold
// (B4, §80) for the requesting user. Returns false — with the error already
// written — when the user is below threshold or the check cannot run
// (BUG-004: fail closed when the points checker is nil). The action phrase
// completes the user-facing message "You need more reputation to <action>".
func (h *Handlers) checkReputationGate(w http.ResponseWriter, r *http.Request, userID pgtype.UUID, action string) bool {
	threshold := h.service.MinReputationToEdit(r.Context())
	if threshold <= 0 {
		return true
	}

	if h.points == nil {
		slog.Error("wiki: points checker is nil, cannot enforce reputation gate (fail-closed)")
		errpkg.WriteError(w, http.StatusInternalServerError, "reputation_check_unavailable",
			"Reputation verification is currently unavailable")

		return false
	}

	points, err := h.points.GetPointsBalance(r.Context(), userID)
	if err != nil {
		slog.Error("wiki: reputation check failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "reputation_check_failed",
			"Failed to verify reputation")

		return false
	}

	if points < int64(threshold) {
		errpkg.WriteError(w, http.StatusForbidden, "insufficient_reputation",
			"You need more reputation to "+action)

		return false
	}

	return true
}

// writeFieldOpsError maps SubmitFieldOps/SuggestFields errors to the
// standard API error envelope (§80 community field ops): permission and
// resolution failures map to their sentinels, ErrDuplicateContent and
// ErrEditInProgress conflict, and the wrapped validation errors the service
// produces ("wiki: submit fields:"/"wiki: suggest:" prefixes) map to 400.
func writeFieldOpsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrOpNotPermitted):
		errpkg.WriteError(w, http.StatusForbidden, "op_not_permitted",
			"You do not have permission for one or more of these field operations")
	case errors.Is(err, ErrEntityNotResolvable):
		errpkg.WriteError(w, http.StatusNotFound, "entity_not_found",
			"The target entity could not be resolved")
	case errors.Is(err, ErrDuplicateContent):
		errpkg.WriteError(w, http.StatusConflict, "duplicate_content",
			"This submission is identical to an existing revision")
	case errors.Is(err, ErrEditInProgress):
		errpkg.WriteError(w, http.StatusConflict, "edit_in_progress",
			"A wiki edit is already in progress for this page")
	case strings.HasPrefix(err.Error(), "wiki: submit fields:"),
		strings.HasPrefix(err.Error(), "wiki: suggest:"):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_ops", err.Error())
	default:
		slog.Error("wiki: field ops failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "fields_submit_failed",
			"Failed to submit field edits")
	}
}

// indexApprovedContent fire-and-forget enqueues RAG indexing of approved
// wiki revision content (B8, §69/ADR-069) so Japabot can retrieve wiki
// knowledge. Skips when no indexer is wired, there is no prose, or the
// revision is page-less. Errors are logged, never propagated (Rule 8).
func (h *Handlers) indexApprovedContent(r *http.Request, pageID pgtype.UUID, content []byte) {
	if h.rag == nil || len(content) == 0 || !pageID.Valid {
		return
	}

	// BUG-002 (rag-audit): resolve the indexing language from
	// i18n.default_language (§7 i18n) instead of passing an empty string.
	// Falls back to "en" when settings are not injected or the key is unset.
	language := "en"
	if h.settings != nil {
		language = h.settings.GetStringOr(r.Context(), "i18n.default_language", "en")
	}

	go func() { // #nosec G118 -- fire-and-forget: request ctx is cancelled on HTTP return (Rule 8)
		ctx := context.Background()
		// Wiki pages are a public corpus — no scoped visibility (§69).
		err := rag.IndexWithRetry(ctx, "wiki_page", pageID.String(), "index", func(ctx context.Context) error {
			return h.rag.IndexEntity(ctx, "wiki_page", pageID, "public", pgtype.UUID{}, pgtype.UUID{}, string(content), language)
		})
		if err != nil {
			slog.Warn("wiki: RAG index failed on approval",
				"page_id", pageID.String(), "error", err)
			if recErr := h.rag.RecordIndexFailure(ctx, "wiki_page", pageID, "index", err); recErr != nil {
				slog.Warn("wiki: RAG index failure record failed",
					"page_id", pageID.String(), "error", recErr)
			}
		}
	}()
}

// ---------------------------------------------------------------------------
// Public handlers (browse — anonymous)
// ---------------------------------------------------------------------------

// HandleGetIndex handles GET /wiki/:space_slug — wiki index for a space.
// Also accepts ?dest= (destination space ID) as a geo-aware alternative to
// the URL path :space_slug parameter (ADR-086 geo-aware pattern).
func (h *Handlers) HandleGetIndex(w http.ResponseWriter, r *http.Request) {

	// ?dest= takes precedence over :space_slug when provided.
	if destParam := r.URL.Query().Get("dest"); destParam != "" {
		var destSpaceID pgtype.UUID
		if err := destSpaceID.Scan(destParam); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}

		index, err := h.service.GetWikiIndexByDestSpaceID(r.Context(), destSpaceID)
		if err != nil {
			if errors.Is(err, ErrSpaceNotFound) {
				errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Destination space not found")
				return
			}

			slog.Error("wiki: index by dest failed", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "index_failed", "Failed to get wiki index")

			return
		}

		h.resolveLiterals(r, &index.Pages)

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": index.Pages, "ancestors": index.Ancestors, "ancestors_alt": index.AncestorsAlt, "kind": index.Kind})

		return
	}

	spaceSlug := chi.URLParam(r, "space_slug")
	if spaceSlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Space slug or ?dest= is required")
		return
	}

	index, err := h.service.GetWikiIndex(r.Context(), spaceSlug)
	if err != nil {
		if errors.Is(err, ErrSpaceNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Destination space not found")
			return
		}

		slog.Error("wiki: index failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "index_failed", "Failed to get wiki index")

		return
	}

	h.resolveLiterals(r, &index.Pages)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": index.Pages, "ancestors": index.Ancestors, "ancestors_alt": index.AncestorsAlt, "kind": index.Kind})
}

// wikiPageTemplateItem is the public shape for one page template under a
// category — the link target for a lazy template page
// (/wiki/{space}/{slug}); label already has {name} substituted when the
// request carries ?space=. ParentTemplateID marks a nested topic template
// (§80 Wiki Topic Pages) — the frontend keeps it off top-level nav/cards;
// the topic stays reachable through its parent page.
type wikiPageTemplateItem struct {
	Slug             string `json:"slug"`
	Label            string `json:"label"`
	Position         int32  `json:"position"`
	ParentTemplateID string `json:"parent_template_id,omitempty"`
}

// wikiCategoryItem is the public GET /wiki/categories payload shape.
// slug is the wiki_pages.category token (matches each index page's
// `category` field — the value the tab strip and create form key on);
// page_slug is the stub page slug used in /wiki/{entity}/{slug} URLs.
type wikiCategoryItem struct {
	// ID is the category row's UUID — emitted so wiki.manage holders can
	// address admin routes (e.g. inline page-template authoring on the
	// member create form, §80). It carries no data beyond what an admin
	// could already list.
	ID           string `json:"id,omitempty"`
	Slug         string `json:"slug"`
	PageSlug     string `json:"page_slug"`
	Category     string `json:"category"`
	LabelKey     string `json:"label_key"`
	Label        string `json:"label"`
	FieldGroupID string `json:"field_group_id,omitempty"`
	Position     int32  `json:"position"`
	IsSystem     bool   `json:"is_system"`
	// Directory marks a category whose rollup config sets
	// mode:"directory" — its stub page is the child-space listing, so
	// the frontend must link page_slug even with no materialized pages.
	Directory bool `json:"directory,omitempty"`
	// Empty is only meaningful on ?space= requests for directory
	// categories: true when the space has zero matching descendants —
	// tabs/cards should hide it rather than link a dead listing.
	Empty bool `json:"empty,omitempty"`
	// PageTemplates lists the category's declared sibling pages — lazy
	// template pages the frontend links into (§80); empty when the
	// category declares none or the request has no space context.
	PageTemplates []wikiPageTemplateItem `json:"page_templates,omitempty"`
	// Icon is the admin-configured lucide icon name; empty falls back to
	// the frontend's slug→icon map.
	Icon string `json:"icon,omitempty"`
}

func categoryItem(c WikiCategory) wikiCategoryItem {
	item := wikiCategoryItem{
		Slug:     c.Category,
		PageSlug: c.Slug,
		Category: c.Category,
		LabelKey: "wiki.category_" + c.Category,
		Label:    c.Label,
		Position: c.Position,
		IsSystem: c.IsSystem,
	}
	if c.ID.Valid {
		item.ID = c.ID.String()
	}
	if c.Icon != nil {
		item.Icon = *c.Icon
	}
	if c.FieldGroupID.Valid {
		item.FieldGroupID = c.FieldGroupID.String()
	}
	if rcfg := parseRollupConfig(c.Rollup); rcfg != nil && rcfg.Mode == rollupModeDirectory {
		item.Directory = true
		item.Empty = c.Empty
	}

	return item
}

// HandleGetCategories handles GET /wiki/categories — the space's wiki type
// category template, resolved from wiki_type_categories (§80 Wiki Types
// amendment; replaces the hardcoded geo/university slices so new wiki types
// and admin-managed categories appear automatically — Rule 16). ?space=
// resolves the space's type and scopes the list; without it the union of
// every enabled type's categories is returned. scope carries the
// space_type slug ("university" still satisfies the frontend's
// scope==='university' check; "all" for the union).
func (h *Handlers) HandleGetCategories(w http.ResponseWriter, r *http.Request) {
	var (
		cats  []WikiCategory
		err   error
		scope = "all"
	)

	var templatesByCat map[pgtype.UUID][]wikiPageTemplateItem
	if spaceSlug := strings.TrimSpace(r.URL.Query().Get("space")); spaceSlug != "" {
		cats, scope, err = h.service.ListCategoriesForSpace(r.Context(), spaceSlug)
		// §80 lazy template pages: surface each category's declared
		// templates so the frontend can link them — the pages need no
		// wiki_pages row to resolve.
		if err == nil {
			if tmpls, terr := h.service.ListPageTemplatesForSpace(r.Context(), spaceSlug); terr == nil {
				templatesByCat = make(map[pgtype.UUID][]wikiPageTemplateItem, len(tmpls))
				for _, tm := range tmpls {
					item := wikiPageTemplateItem{Slug: tm.Slug, Label: tm.Label, Position: tm.Position}
					if tm.ParentTemplateID.Valid {
						item.ParentTemplateID = tm.ParentTemplateID.String()
					}
					templatesByCat[tm.CategoryID] = append(templatesByCat[tm.CategoryID], item)
				}
			} else {
				slog.Warn("wiki: categories page-templates lookup failed", "error", terr)
			}
		}
	} else {
		cats, err = h.service.ListAllCategories(r.Context())
	}
	if err != nil {
		if errors.Is(err, ErrSpaceNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Space not found")
			return
		}

		slog.Error("wiki: categories failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "categories_failed", "Failed to list wiki categories")

		return
	}

	out := make([]wikiCategoryItem, 0, len(cats))
	seen := make(map[string]bool, len(cats))
	for _, c := range cats {
		if seen[c.Category] {
			continue
		}
		seen[c.Category] = true
		item := categoryItem(c)
		item.PageTemplates = templatesByCat[c.ID]
		out = append(out, item)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out, "scope": scope})
}

// HandleGetFieldTypes handles GET /wiki/field-types — the field types
// community members may propose through §80 field ops (propose_field /
// propose_group). Returned as {value, label} so the frontend can override
// labels via i18n while staying driven by the canonical list.
func (h *Handlers) HandleGetFieldTypes(w http.ResponseWriter, r *http.Request) {
	types := customfields.ProposableFieldTypes()
	out := make([]map[string]string, 0, len(types))
	for _, ft := range types {
		out = append(out, map[string]string{
			"value": ft,
			"label": strings.ReplaceAll(ft, "_", " "),
		})
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

// HandleGetDestinations handles GET /wiki/destinations — returns active
// destination spaces (country/state/city/university). Optional query params:
//
//	?level=        country|state|city|university
//	?parent=       parent space slug (returns its direct children)
//	?within=       primary-space slug — matches spaces linked to it or to
//	               one of its descendants via space_parent_spaces/parent_id
//	?region=       continent/region value (e.g. Africa, Europe) — filters country-level spaces
//	?q=            case-insensitive name search
//	?has_wiki=     true (default) or false — when true, only spaces with at
//	               least one active wiki page are returned.
//	?sort=         name (default, A–Z) | page_count (most guides first)
//	?page=         1-based page number (default 1)
//	?limit=        page size, capped at 100. Passing either ?page or ?limit
//	               engages pagination; ?page alone sizes pages via
//	               wiki.destinations_page_limit. Neither = the full list.
//
// §80 Reader UX — paginated responses carry the standard sidecar:
// {data, pagination:{page, limit, total}}.
func (h *Handlers) HandleGetDestinations(w http.ResponseWriter, r *http.Request) {

	q := r.URL.Query()
	level := strings.ToLower(q.Get("level"))
	parentSlug := strings.ToLower(q.Get("parent"))
	within := strings.ToLower(strings.TrimSpace(q.Get("within")))
	region := q.Get("region")
	search := strings.TrimSpace(q.Get("q"))
	sort := strings.ToLower(strings.TrimSpace(q.Get("sort")))

	hasWiki := q.Get("has_wiki") != "false"

	// Pagination engages only when the caller asks for it (?page or ?limit) —
	// legacy callers pass neither and get the full list. A page request
	// without an explicit limit uses wiki.destinations_page_limit.
	paginate := q.Has("page") || q.Has("limit")
	var page, limit int32 = 1, 0
	if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = int32(n) // #nosec
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = int32(n) // #nosec
		}
	}

	dests, total, effLimit, err := h.service.GetWikiDestinations(r.Context(), level, parentSlug, region, search, within, hasWiki, sort, page, limit, paginate)
	if err != nil {
		if errors.Is(err, ErrInvalidInput) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", err.Error())
			return
		}

		slog.Error("wiki: destinations failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "destinations_failed", "Failed to get wiki destinations")

		return
	}

	resp := map[string]any{"data": dests}
	// The sidecar only ships on paginated calls — legacy callers that pass
	// neither ?page nor ?limit keep the original {data:[...]} shape (§80).
	if paginate {
		resp["pagination"] = map[string]any{
			"page":  page,
			"limit": effLimit,
			"total": total,
		}
	}
	errpkg.WriteJSON(w, http.StatusOK, resp)
}

// HandlePageFeedback handles POST /wiki/:space_slug/:page_slug/feedback —
// the member's "Was this page helpful?" vote (§80 Reader UX). Body:
// {vote: "up"|"down"|""} — an empty vote retracts a prior vote. Requires
// auth; the service returns the post-write aggregate counts.
func (h *Handlers) HandlePageFeedback(w http.ResponseWriter, r *http.Request) {
	spaceSlug := chi.URLParam(r, "space_slug")
	pageSlug := chi.URLParam(r, "page_slug")

	if spaceSlug == "" || pageSlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Space and page slugs are required")
		return
	}

	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Sign in to rate this page")
		return
	}

	var body struct {
		Vote string `json:"vote"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}

	dto, err := h.service.SubmitPageFeedback(r.Context(), u.UserID, spaceSlug, pageSlug, strings.TrimSpace(strings.ToLower(body.Vote)))
	if err != nil {
		if errors.Is(err, ErrSpaceNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Destination space not found")
			return
		}

		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "page_not_found", "Wiki page not found")
			return
		}

		if errors.Is(err, ErrInvalidInput) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", err.Error())
			return
		}

		slog.Error("wiki: feedback failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "feedback_failed", "Failed to record feedback")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": dto})
}

// HandleGetPage handles GET /wiki/:space_slug/:page_slug — single wiki page
// with its current approved revision content.
func (h *Handlers) HandleGetPage(w http.ResponseWriter, r *http.Request) {

	spaceSlug := chi.URLParam(r, "space_slug")
	pageSlug := chi.URLParam(r, "page_slug")

	if spaceSlug == "" || pageSlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Space and page slugs are required")
		return
	}

	var userID pgtype.UUID
	if u := auth.FromContext(r.Context()); u != nil {
		userID = u.UserID
	}

	page, err := h.service.GetWikiPage(r.Context(), userID, spaceSlug, pageSlug)
	if err != nil {
		if errors.Is(err, ErrSpaceNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Destination space not found")
			return
		}

		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "page_not_found", "Wiki page not found")
			return
		}

		slog.Error("wiki: get page failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get wiki page")

		return
	}

	h.resolveLiterals(r, &page)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": page})
}

// HandleGetHistory handles GET /wiki/:space_slug/:page_slug/history —
// revision history for a wiki page.
func (h *Handlers) HandleGetHistory(w http.ResponseWriter, r *http.Request) {

	spaceSlug := chi.URLParam(r, "space_slug")
	pageSlug := chi.URLParam(r, "page_slug")

	if spaceSlug == "" || pageSlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Space and page slugs are required")
		return
	}

	page, limit := parsePageParams(r)

	// Pending revision bodies are unmoderated — visible only to the submitter
	// and to users holding wiki.review/wiki.manage. Anonymous/other viewers
	// see the pending row redacted (service-side).
	var viewerID pgtype.UUID
	canReview := false
	if u := auth.FromContext(r.Context()); u != nil {
		viewerID = u.UserID
		if h.perms != nil {
			canReview = h.perms.HasPermission(r.Context(), u.UserID, "wiki.review") ||
				h.perms.HasPermission(r.Context(), u.UserID, "wiki.manage")
		}
	}

	revs, total, err := h.service.GetRevisionHistory(r.Context(), viewerID, canReview, spaceSlug, pageSlug, page, limit)
	if err != nil {
		if errors.Is(err, ErrSpaceNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Destination space not found")
			return
		}

		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "page_not_found", "Wiki page not found")
			return
		}

		slog.Error("wiki: history failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "history_failed", "Failed to get revision history")

		return
	}

	h.resolveLiterals(r, &revs)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": revs,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleGetEntityFields handles GET /wiki/entities/:type/:id/fields — the
// editable field schema for an entity (§80 community field ops, spec §8.1):
// field groups assigned to its entity class, current values, and per-field
// can_view/can_edit flags for the requesting user. Anonymous callers resolve
// the guest user group's can_view grants (§06) — never can_edit.
func (h *Handlers) HandleGetEntityFields(w http.ResponseWriter, r *http.Request) {
	entityType := chi.URLParam(r, "type")
	if entityType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_type", "Entity type is required")
		return
	}

	entityID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var userID pgtype.UUID
	if u := auth.FromContext(r.Context()); u != nil {
		userID = u.UserID
	}

	dto, err := h.service.GetEntityFields(r.Context(), userID, entityType, entityID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "entity_not_found", "Entity not found")
			return
		}

		slog.Error("wiki: entity fields failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "entity_fields_failed", "Failed to get entity fields")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": dto})
}

// HandleGetEntityRevisions handles GET /wiki/entities/:type/:id/revisions —
// the revision history (fields/mixed) targeting an entity (§80).
func (h *Handlers) HandleGetEntityRevisions(w http.ResponseWriter, r *http.Request) {
	entityType := chi.URLParam(r, "type")
	if entityType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_type", "Entity type is required")
		return
	}

	entityID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	page, limit := parsePageParams(r)

	// Pending revision bodies are unmoderated — redact them unless the viewer
	// is the submitter or a reviewer (same rule as page history).
	var viewerID pgtype.UUID
	canReview := false
	if u := auth.FromContext(r.Context()); u != nil {
		viewerID = u.UserID
		if h.perms != nil {
			canReview = h.perms.HasPermission(r.Context(), u.UserID, "wiki.review") ||
				h.perms.HasPermission(r.Context(), u.UserID, "wiki.manage")
		}
	}

	revs, total, err := h.service.ListEntityRevisions(r.Context(), viewerID, canReview, entityType, entityID, page, limit)
	if err != nil {
		slog.Error("wiki: entity revisions failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "entity_revisions_failed", "Failed to get entity revisions")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": revs,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (submit revision — auth, wiki.edit)
// ---------------------------------------------------------------------------

// HandleCreatePage handles POST /wiki/:space_slug — create a new wiki page
// with an initial pending revision. Auth required (wiki.edit permission checked
// at route registration). Enforces the same community_edits and reputation
// gates as HandleSubmitRevision.
func (h *Handlers) HandleCreatePage(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	spaceSlug := chi.URLParam(r, "space_slug")
	if spaceSlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Space slug is required")
		return
	}

	if !h.service.CommunityEditsAllowed(r.Context()) && h.perms != nil {
		isMod := h.perms.HasPermission(r.Context(), user.UserID, "wiki.review") ||
			h.perms.HasPermission(r.Context(), user.UserID, "wiki.manage")
		if !isMod {
			errpkg.WriteError(w, http.StatusForbidden, "community_edits_disabled",
				"Community edits are currently disabled for this wiki")
			return
		}
	}

	// B4: reputation gate (§80) — wiki.edit is reputation-gated.
	if !h.checkReputationGate(w, r, user.UserID, "create a wiki page") {
		return
	}

	var input CreateWikiPageInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	input.SpaceSlug = spaceSlug

	pageID, revID, pageSlug, err := h.service.CreatePageWithRevision(r.Context(), user.UserID, input)
	if err != nil {
		if errors.Is(err, ErrSpaceNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Destination not found")
			return
		}

		if errors.Is(err, ErrInvalidInput) {
			// Strip the sentinel's own text — clients see the specific
			// detail ("invalid page slug"), not "invalid input: …".
			msg := strings.TrimPrefix(err.Error(), ErrInvalidInput.Error()+": ")
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", msg)

			return
		}

		if errors.Is(err, ErrPageExists) {
			errpkg.WriteError(w, http.StatusConflict, "page_exists", "A wiki page with this slug already exists")
			return
		}

		if errors.Is(err, ErrDuplicateContent) {
			errpkg.WriteError(w, http.StatusConflict, "duplicate_content", "This content is identical to an existing revision")
			return
		}

		slog.Error("wiki: create page failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create wiki page")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"page_id":     pageID,
			"revision_id": revID,
			"slug":        pageSlug,
			"status":      "pending",
		},
	})
}

// HandleSubmitRevision handles POST /wiki/:space_slug/:page_slug/revisions —
// submit a wiki edit for review. Auth required (wiki.edit permission checked
// at route registration). The handler enforces two §80 business-logic gates
// before delegating to the service:
//   - wiki.community_edits: when false, only wiki.review/wiki.manage holders
//     may submit (mod-only edits).
//   - wiki.min_reputation_to_edit: the submitter's lifetime points must meet
//     the configured threshold.
func (h *Handlers) HandleSubmitRevision(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	spaceSlug := chi.URLParam(r, "space_slug")
	pageSlug := chi.URLParam(r, "page_slug")

	if spaceSlug == "" || pageSlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Space and page slugs are required")
		return
	}

	// B5: community_edits gate (§80). When wiki.community_edits is false,
	// community submissions are blocked — only mods/admins (wiki.review or
	// wiki.manage holders) may submit.
	if !h.service.CommunityEditsAllowed(r.Context()) && h.perms != nil {
		isMod := h.perms.HasPermission(r.Context(), user.UserID, "wiki.review") ||
			h.perms.HasPermission(r.Context(), user.UserID, "wiki.manage")
		if !isMod {
			errpkg.WriteError(w, http.StatusForbidden, "community_edits_disabled",
				"Community edits are currently disabled for this wiki")
			return
		}
	}

	// B4: reputation gate (§80). wiki.edit is "member with reputation ≥
	// threshold". The route middleware checks the wiki.edit permission node;
	// this checks the lifetime-points threshold on top.
	// BUG-004: fail closed when the points checker is nil — if the reputation
	// gate cannot be enforced, the edit must be rejected rather than silently
	// allowed. This prevents a bypass if SetPointsChecker was never called.
	if !h.checkReputationGate(w, r, user.UserID, "edit this wiki page") {
		return
	}

	var input SubmitRevisionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if len(input.Content) == 0 && len(input.Ops) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_content", "Content or field ops are required")
		return
	}

	// §80 community field ops: a submission carrying ops is stored as ONE
	// revision — kind='fields' for ops-only, kind='mixed' when prose rides
	// along — so one user action produces one queue entry and approval
	// applies prose + ops atomically in a single transaction.
	if len(input.Ops) > 0 {
		if !h.service.StructuredEditsEnabled(r.Context()) {
			errpkg.WriteError(w, http.StatusNotFound, "feature_disabled", "This feature is not available")
			return
		}

		canManage := h.perms != nil && h.perms.HasPermission(r.Context(), user.UserID, PermFieldsManage)

		// Validate-then-resolve ordering lives in the service (§80): a
		// rejected ops submission must not materialize a lazy template page
		// into an empty wiki_pages row.
		revID, autoApproved, err := h.service.SubmitPageFieldOps(r.Context(), user.UserID, spaceSlug, pageSlug, input.Content, input.Ops, input.Summary, canManage)
		if err != nil {
			switch {
			case errors.Is(err, ErrSpaceNotFound):
				errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Destination space not found")
			case errors.Is(err, pgx.ErrNoRows):
				errpkg.WriteError(w, http.StatusNotFound, "page_not_found", "Wiki page not found")
			case errors.Is(err, ErrPageLocked):
				errpkg.WriteError(w, http.StatusForbidden, "page_locked", "This wiki page is locked to mod-only edits")
			default:
				writeFieldOpsError(w, err)
			}
			return
		}

		status := "pending"
		if autoApproved {
			status = "approved"
		}

		errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
			"revision_id":   revID,
			"status":        status,
			"auto_approved": autoApproved,
		}})

		return
	}

	revID, err := h.service.SubmitRevision(r.Context(), user.UserID, spaceSlug, pageSlug, input)
	if err != nil {
		if errors.Is(err, ErrSpaceNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Destination space not found")
			return
		}

		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "page_not_found", "Wiki page not found")
			return
		}

		if errors.Is(err, ErrInvalidInput) {
			msg := strings.TrimPrefix(err.Error(), ErrInvalidInput.Error()+": ")
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", msg)

			return
		}

		if errors.Is(err, ErrPageLocked) {
			errpkg.WriteError(w, http.StatusForbidden, "page_locked", "This wiki page is locked to mod-only edits")
			return
		}

		if errors.Is(err, ErrEditInProgress) {
			errpkg.WriteError(w, http.StatusConflict, "edit_in_progress", "A wiki edit is already in progress for this page")
			return
		}

		if errors.Is(err, ErrDuplicateContent) {
			errpkg.WriteError(w, http.StatusConflict, "duplicate_content", "This content is identical to an existing revision")
			return
		}

		slog.Error("wiki: submit revision failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "submit_failed", "Failed to submit revision")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"revision_id": revID,
		"status":      "pending",
	}})
}

// SuggestFieldsInput is the request body for POST /wiki/suggest — an
// entity-keyed structured-edit suggestion (§80 community field ops).
type SuggestFieldsInput struct {
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	Ops        []FieldOp `json:"ops"`
	Summary    string    `json:"summary"`
}

// HandleSuggestFields handles POST /wiki/suggest — submit field ops against
// an entity that may not have a host wiki page (page-less revisions). Auth
// required; the per-op wiki.fields.* grant map is enforced in the service
// (§10). Submissions carrying set_value ops are reputation-gated at the
// wiki.min_reputation_to_edit threshold (§80: wiki.fields.edit is rep-gated
// at the same threshold as wiki.edit).
func (h *Handlers) HandleSuggestFields(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var input SuggestFieldsInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.EntityType == "" || input.EntityID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "entity_type and entity_id are required")
		return
	}

	var entityID pgtype.UUID
	if err := entityID.Scan(input.EntityID); err != nil || !entityID.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid entity_id")
		return
	}

	if len(input.Ops) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_ops", "At least one field operation is required")
		return
	}

	// Reputation gate applies only to set_value ops (the wiki.fields.edit
	// tier); propose/manage ops are gated purely by their RBAC nodes.
	needsReputation := false

	for _, op := range input.Ops {
		if op.Op == OpSetValue {
			needsReputation = true
			break
		}
	}

	if needsReputation && !h.checkReputationGate(w, r, user.UserID, "submit field edits") {
		return
	}

	canManage := h.perms != nil && h.perms.HasPermission(r.Context(), user.UserID, PermFieldsManage)

	revID, autoApproved, err := h.service.SuggestFields(r.Context(), user.UserID, input.EntityType, entityID, input.Ops, input.Summary, canManage)
	if err != nil {
		writeFieldOpsError(w, err)
		return
	}

	status := "pending"
	if autoApproved {
		status = "approved"
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"revision_id":   revID,
			"status":        status,
			"auto_approved": autoApproved,
		},
	})
}

// ---------------------------------------------------------------------------
// Admin handlers (review, manage, generate — wiki.review / wiki.manage)
// ---------------------------------------------------------------------------

// HandleApproveRevision handles POST /admin/wiki/:page_id/revisions/:id/approve.
// After approval, fire-and-forget enqueues RAG indexing of the approved
// content (B8, §69/ADR-069) so Japabot can retrieve wiki knowledge.
func (h *Handlers) HandleApproveRevision(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	pageID, ok := scanUUIDParam(w, r, "page_id")
	if !ok {
		return
	}

	revID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Notes string `json:"notes"`
	}

	_ = json.NewDecoder(r.Body).Decode(&body)

	content, err := h.service.ApproveRevision(r.Context(), revID, pageID, user.UserID, body.Notes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "revision_not_found", "Revision not found for this page")
			return
		}

		if errors.Is(err, ErrRevisionNotPending) {
			errpkg.WriteError(w, http.StatusConflict, "revision_not_pending", "Revision is not pending review")
			return
		}

		slog.Error("wiki: approve failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve revision")

		return
	}

	// B8: fire-and-forget RAG indexing of the approved content. The RAG
	// indexer's IndexEntity calls ExtractPlainText internally, which handles
	// TipTap JSON. Errors are logged, never propagated (Rule 8).
	h.indexApprovedContent(r, pageID, content)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"approved": true}})
}

// HandleRejectRevision handles POST /admin/wiki/:page_id/revisions/:id/reject.
// The frontend sends { "reason": "..." }; we accept both "reason" (UI field)
// and "notes" (legacy/direct-API alias) and persist to hitl_review_notes (B2).
// The revision must belong to page_id — the service returns pgx.ErrNoRows on
// a mismatch, mapped to 404 like the approve route.
func (h *Handlers) HandleRejectRevision(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	pageID, ok := scanUUIDParam(w, r, "page_id")
	if !ok {
		return
	}

	revID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Reason string `json:"reason"`
		Notes  string `json:"notes"`
	}

	_ = json.NewDecoder(r.Body).Decode(&body)

	notes := body.Reason
	if notes == "" {
		notes = body.Notes
	}

	if err := h.service.RejectRevision(r.Context(), revID, pageID, user.UserID, notes); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "revision_not_found", "Revision not found for this page")
			return
		}

		if errors.Is(err, ErrRevisionNotPending) {
			errpkg.WriteError(w, http.StatusConflict, "revision_not_pending", "Revision is not pending review")
			return
		}

		slog.Error("wiki: reject failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "reject_failed", "Failed to reject revision")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"rejected": true}})
}

// HandleAdminListRevisions handles GET /admin/wiki/revisions — the review
// queue filtered by ?kind= (content|fields|mixed) and ?status=
// (pending|approved|rejected), paginated via ?page=&limit= (§80 community
// field ops review surface).
func (h *Handlers) HandleAdminListRevisions(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	kind := r.URL.Query().Get("kind")
	switch kind {
	case "", "content", "fields", "mixed":
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_filter", "kind must be one of content, fields, mixed")
		return
	}

	status := r.URL.Query().Get("status")
	switch status {
	case "", "pending", "approved", "rejected":
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_filter", "status must be one of pending, approved, rejected")
		return
	}

	q := r.URL.Query().Get("q")
	// Whitelisted in the SQL CASE (created_at:asc|desc); unknown values
	// fall through to the created_at DESC default.
	sort := r.URL.Query().Get("sort")

	deleted := r.URL.Query().Get("deleted")
	switch deleted {
	case "", "only", "all":
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_filter", "deleted must be one of only, all")
		return
	}

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	revs, total, err := h.service.AdminListRevisions(r.Context(), kind, status, q, sort, deleted, page, limit)
	if err != nil {
		slog.Error("wiki: admin list revisions failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list revisions")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": revs,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleApproveRevisionByID handles POST /admin/wiki/revisions/:id/approve —
// the revision-keyed approve route (§80), canonical for page-less ops
// revisions whose wiki_page_id is NULL. For fields/mixed revisions the
// service applies the ops array transactionally with the status flip and
// enqueues wiki:fields_propagate.
func (h *Handlers) HandleApproveRevisionByID(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	revID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Notes  string `json:"notes"`
		Reason string `json:"reason"`
	}

	_ = json.NewDecoder(r.Body).Decode(&body)

	notes := body.Notes
	if notes == "" {
		notes = body.Reason
	}

	// Load the revision first so a missing row 404s cleanly and the
	// content/page context is available for post-approval RAG indexing.
	rev, err := h.service.GetRevision(r.Context(), revID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "revision_not_found", "Revision not found")
			return
		}

		slog.Error("wiki: approve by id: load failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve revision")

		return
	}

	if err := h.service.ApproveRevisionByID(r.Context(), revID, user.UserID, notes); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "revision_not_found", "Revision not found")
			return
		}

		if errors.Is(err, ErrRevisionNotPending) {
			errpkg.WriteError(w, http.StatusConflict, "revision_not_pending", "Revision is not pending review")
			return
		}

		slog.Error("wiki: approve by id failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve revision")

		return
	}

	// B8: index approved prose; fields-only revisions carry no content.
	if rev.Kind != "fields" {
		h.indexApprovedContent(r, rev.WikiPageID, rev.Content)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"approved": true}})
}

// HandleRejectRevisionByID handles POST /admin/wiki/revisions/:id/reject —
// the revision-keyed reject counterpart (§80). Accepts "reason" or "notes"
// like the page-scoped route.
func (h *Handlers) HandleRejectRevisionByID(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	revID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Reason string `json:"reason"`
		Notes  string `json:"notes"`
	}

	_ = json.NewDecoder(r.Body).Decode(&body)

	notes := body.Reason
	if notes == "" {
		notes = body.Notes
	}

	// Load first so a missing row 404s cleanly (mirrors HandleApproveRevisionByID).
	if _, err := h.service.GetRevision(r.Context(), revID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "revision_not_found", "Revision not found")
			return
		}

		slog.Error("wiki: reject by id: load failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "reject_failed", "Failed to reject revision")

		return
	}

	if err := h.service.RejectRevisionByID(r.Context(), revID, user.UserID, notes); err != nil {
		if errors.Is(err, ErrRevisionNotPending) {
			errpkg.WriteError(w, http.StatusConflict, "revision_not_pending", "Revision is not pending review")
			return
		}

		slog.Error("wiki: reject by id failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "reject_failed", "Failed to reject revision")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"rejected": true}})
}

// HandleDeleteWikiRevision handles DELETE /admin/wiki/revisions/:id — soft
// delete marks the row deleted_at (every reader excludes it); ?force=true
// permanently removes an already-soft-deleted row (409 revision_not_deleted
// on a live one). The route middleware enforces wiki.delete plus
// admin.hard_delete when force=true (Rule 11).
func (h *Handlers) HandleDeleteWikiRevision(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	revID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	force := r.URL.Query().Get("force") == "true"
	if err := h.service.DeleteRevision(r.Context(), revID, force); err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			errpkg.WriteError(w, http.StatusNotFound, "revision_not_found", "Revision not found")
		case errors.Is(err, ErrRevisionNotDeleted):
			errpkg.WriteError(w, http.StatusConflict, "revision_not_deleted",
				"Revision must be soft-deleted before it can be permanently removed")
		default:
			slog.Error("wiki: delete revision failed", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete revision")
		}
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true, "force": force}})
}

// HandleUpdatePage handles PATCH /admin/wiki/:page_id — lock/unlock, feature,
// activate, and data_entity_type binding (§80: binds the page to a child
// entity class so field ops target it instead of the host space). The route
// middleware checks wiki.manage. When the is_locked field is being toggled,
// the handler additionally checks wiki.lock (§80 RBAC: wiki.lock is a
// distinct permission for lock/unlock, B7).
func (h *Handlers) HandleUpdatePage(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	pageID, ok := scanUUIDParam(w, r, "page_id")
	if !ok {
		return
	}

	var input UpdatePageInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// B7: when is_locked is being changed, require wiki.lock in addition to
	// wiki.manage (§80 defines wiki.lock as a distinct permission node).
	if input.IsLocked != nil && h.perms != nil {
		if !h.perms.HasPermission(r.Context(), user.UserID, "wiki.lock") {
			errpkg.WriteError(w, http.StatusForbidden, "lock_permission_required",
				"Locking/unlocking wiki pages requires the wiki.lock permission")
			return
		}
	}

	if err := h.service.UpdatePage(r.Context(), pageID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "page_not_found", "Wiki page not found")
			return
		}

		slog.Error("wiki: update page failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update wiki page")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"updated": true}})
}

// HandleDeletePage handles DELETE /admin/wiki/:page_id — soft delete (§80).
// ?force=true permanently removes the page and cascades its revisions;
// force requires a prior soft delete (409 page_not_deleted on a live page).
// The route middleware enforces wiki.delete.
func (h *Handlers) HandleDeletePage(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	pageID, ok := scanUUIDParam(w, r, "page_id")
	if !ok {
		return
	}

	force := r.URL.Query().Get("force") == "true"
	if err := h.service.DeletePage(r.Context(), pageID, force); err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			errpkg.WriteError(w, http.StatusNotFound, "page_not_found", "Wiki page not found")
		case errors.Is(err, ErrPageNotDeleted):
			errpkg.WriteError(w, http.StatusConflict, "page_not_deleted",
				"Page must be soft-deleted before it can be permanently removed")
		default:
			slog.Error("wiki: delete page failed", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete wiki page")
		}
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true, "force": force}})
}

// HandleGenerate handles POST /admin/wiki/generate — create a stub wiki page
// for a space/category combination so the Content Agent can later fill in a
// draft revision (ADR-088). The page shell is created now; the AI draft
// content enters the HITL review queue as a pending ai_generated revision
// when the Content Agent task runs (B3: replaces the former stub).
func (h *Handlers) HandleGenerate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var input GeneratePageInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.SpaceSlug == "" || input.PageSlug == "" || input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "space_slug, page_slug, and title are required")
		return
	}

	pageID, err := h.service.GeneratePage(r.Context(), user.UserID, input)
	if err != nil {
		if errors.Is(err, ErrSpaceNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Destination space not found")
			return
		}

		if errors.Is(err, ErrPageExists) {
			// Idempotent: page already exists. Return 200 with the slug so the
			// caller can still trigger an AI draft for the existing page.
			errpkg.WriteJSON(w, http.StatusOK, map[string]any{
				"data": map[string]any{
					"status":     "already_exists",
					"space_slug": input.SpaceSlug,
					"page_slug":  input.PageSlug,
					"title":      input.Title,
					"category":   input.Category,
				},
			})

			return
		}

		slog.Error("wiki: generate failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "generate_failed", "Failed to generate wiki page")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"status":     "created",
			"page_id":    pageID,
			"space_slug": input.SpaceSlug,
			"page_slug":  input.PageSlug,
			"title":      input.Title,
			"category":   input.Category,
		},
	})
}

// HandleAdminList handles GET /admin/wiki — admin list of all wiki pages.
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	var spaceID pgtype.UUID
	if v := r.URL.Query().Get("space_id"); v != "" {
		if err := spaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid space_id")
			return
		}
	}

	// The admin UI's destination autocomplete sends ?dest= with the space UUID
	// (GeoCountry.id); accept a slug too so the param works for either shape.
	var destSlug string
	if v := r.URL.Query().Get("dest"); v != "" {
		var destID pgtype.UUID
		if err := destID.Scan(v); err == nil && destID.Valid {
			if spaceID.Valid && spaceID != destID {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_filter", "conflicting space_id and dest filters")
				return
			}
			spaceID = destID
		} else {
			destSlug = v
		}
	}

	sort := r.URL.Query().Get("sort")

	category := r.URL.Query().Get("category")
	q := r.URL.Query().Get("q")

	var lockedPtr *bool

	if v := r.URL.Query().Get("locked"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			lockedPtr = &b
		}
	}

	var featuredPtr *bool

	if v := r.URL.Query().Get("featured"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			featuredPtr = &b
		}
	}

	var activePtr *bool

	if v := r.URL.Query().Get("active"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			activePtr = &b
		}
	}

	// ?status= filters the latest revision's status — the queue's "show me
	// pages with pending work" workflow.
	status := r.URL.Query().Get("status")
	switch status {
	case "", "pending", "approved", "rejected":
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_filter", "status must be one of pending, approved, rejected")
		return
	}

	// §80 soft delete: ?deleted=only|all surfaces deleted pages for the
	// force-delete flow; any other value behaves like the default (exclude).
	deleted := r.URL.Query().Get("deleted")
	if deleted != "only" && deleted != "all" {
		deleted = ""
	}

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	pages, total, err := h.service.AdminListPages(r.Context(), spaceID, destSlug, category, q, sort, status, lockedPtr, featuredPtr, activePtr, deleted, page, limit)
	if err != nil {
		slog.Error("wiki: admin list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list wiki pages")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": pages,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}
