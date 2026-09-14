// Package wiki — handlers.go provides the HTTP handlers for the Destination
// Wiki routes (§80, ADR-088).
//
// Routes:
//
//	GET    /api/v1/wiki/:space_slug                        — wiki index for space
//	GET    /api/v1/wiki/:space_slug/:page_slug             — single wiki page
//	GET    /api/v1/wiki/:space_slug/:page_slug/history     — revision history
//	POST   /api/v1/wiki/:space_slug/:page_slug/revisions   — submit edit (auth, wiki.edit)
//	POST   /api/v1/admin/wiki/:page_id/revisions/:id/approve — approve revision (auth, wiki.review)
//	POST   /api/v1/admin/wiki/:page_id/revisions/:id/reject  — reject revision (auth, wiki.review)
//	PATCH  /api/v1/admin/wiki/:page_id                     — lock/unlock, feature (auth, wiki.manage)
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

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers exposes the wiki endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
	points   pointsChecker
	perms    permissionChecker
	rag      ragIndexer
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
	IndexEntity(ctx context.Context, entityType string, entityID pgtype.UUID, body, language string) error
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

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	return pgtype.UUID{}, true
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

// ---------------------------------------------------------------------------
// Public handlers (browse — anonymous)
// ---------------------------------------------------------------------------

// HandleGetIndex handles GET /wiki/:space_slug — wiki index for a space.
// Also accepts ?dest= (destination space ID) as a geo-aware alternative to
// the URL path :space_slug parameter (ADR-086 geo-aware pattern).
func (h *Handlers) HandleGetIndex(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	// ?dest= takes precedence over :space_slug when provided.
	if destParam := r.URL.Query().Get("dest"); destParam != "" {
		var destSpaceID pgtype.UUID
		if err := destSpaceID.Scan(destParam); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}

		pages, err := h.service.GetWikiIndexByDestSpaceID(r.Context(), destSpaceID)
		if err != nil {
			if errors.Is(err, ErrSpaceNotFound) {
				errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Destination space not found")
				return
			}

			slog.Error("wiki: index by dest failed", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "index_failed", "Failed to get wiki index")

			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": pages})

		return
	}

	spaceSlug := chi.URLParam(r, "space_slug")
	if spaceSlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Space slug or ?dest= is required")
		return
	}

	pages, err := h.service.GetWikiIndex(r.Context(), spaceSlug)
	if err != nil {
		if errors.Is(err, ErrSpaceNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Destination space not found")
			return
		}

		slog.Error("wiki: index failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "index_failed", "Failed to get wiki index")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": pages})
}

// WikiCategoryDef describes a wiki page category (slug + display label key).
//
//nolint:revive // exported: stutter naming is intentional
type WikiCategoryDef struct {
	Slug     string `json:"slug"`
	LabelKey string `json:"label_key"`
	Label    string `json:"label"`
}

// wikiCategories is the canonical list of wiki page categories, ordered by
// display priority. The slugs match the wiki_pages.category enum values
// defined in migrations/000301_create_wiki_tables.up.sql.
//
// Ref: japaconnect-engineering/sections/80-destination-wiki.md.
var wikiCategories = []WikiCategoryDef{
	{Slug: "overview", LabelKey: "wiki.category_overview", Label: "Overview"},
	{Slug: "visa", LabelKey: "wiki.category_visa", Label: "Visa"},
	{Slug: "cost_of_living", LabelKey: "wiki.category_cost_of_living", Label: "Cost of Living"},
	{Slug: "healthcare", LabelKey: "wiki.category_healthcare", Label: "Healthcare"},
	{Slug: "banking", LabelKey: "wiki.category_banking", Label: "Banking"},
	{Slug: "community", LabelKey: "wiki.category_community", Label: "Community"},
	{Slug: "housing", LabelKey: "wiki.category_housing", Label: "Housing"},
	{Slug: "other", LabelKey: "wiki.category_other", Label: "Other"},
}

// HandleGetCategories handles GET /wiki/categories — returns the canonical
// list of wiki page categories. This allows the frontend to derive the
// category list from the API rather than hardcoding it (Rule 16).
func (h *Handlers) HandleGetCategories(w http.ResponseWriter, r *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": wikiCategories})
}

// HandleGetDestinations handles GET /wiki/destinations — returns active
// destination spaces (country/state/city). Optional query params:
//
//	?level=        country|state|city
//	?parent=       parent space slug (returns its direct children)
//	?region=       continent/region value (e.g. Africa, Europe) — filters country-level spaces
//	?has_wiki=     true (default) or false — when true, only spaces with at
//	               least one active wiki page are returned.
func (h *Handlers) HandleGetDestinations(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	level := strings.ToLower(q.Get("level"))
	parentSlug := strings.ToLower(q.Get("parent"))
	region := q.Get("region")

	hasWiki := q.Get("has_wiki") != "false"

	dests, err := h.service.GetWikiDestinations(r.Context(), level, parentSlug, region, hasWiki)
	if err != nil {
		slog.Error("wiki: destinations failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "destinations_failed", "Failed to get wiki destinations")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": dests})
}

// HandleGetPage handles GET /wiki/:space_slug/:page_slug — single wiki page
// with its current approved revision content.
func (h *Handlers) HandleGetPage(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	spaceSlug := chi.URLParam(r, "space_slug")
	pageSlug := chi.URLParam(r, "page_slug")

	if spaceSlug == "" || pageSlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Space and page slugs are required")
		return
	}

	page, err := h.service.GetWikiPage(r.Context(), spaceSlug, pageSlug)
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": page})
}

// HandleGetHistory handles GET /wiki/:space_slug/:page_slug/history —
// revision history for a wiki page.
func (h *Handlers) HandleGetHistory(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	spaceSlug := chi.URLParam(r, "space_slug")
	pageSlug := chi.URLParam(r, "page_slug")

	if spaceSlug == "" || pageSlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Space and page slugs are required")
		return
	}

	page, limit := parsePageParams(r)

	revs, total, err := h.service.GetRevisionHistory(r.Context(), spaceSlug, pageSlug, page, limit)
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

	_, ok = requireTenant(w, r)
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

	threshold := h.service.MinReputationToEdit(r.Context())
	if threshold > 0 {
		if h.points == nil {
			slog.Error("wiki: points checker is nil, cannot enforce reputation gate (fail-closed)")
			errpkg.WriteError(w, http.StatusInternalServerError, "reputation_check_unavailable",
				"Reputation verification is currently unavailable")

			return
		}

		points, err := h.points.GetPointsBalance(r.Context(), user.UserID)
		if err != nil {
			slog.Error("wiki: reputation check failed", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "reputation_check_failed",
				"Failed to verify reputation")

			return
		}

		if points < int64(threshold) {
			errpkg.WriteError(w, http.StatusForbidden, "insufficient_reputation",
				"You need more reputation to create a wiki page")
			return
		}
	}

	var input CreateWikiPageInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	input.SpaceSlug = spaceSlug

	pageID, revID, err := h.service.CreatePageWithRevision(r.Context(), user.UserID, input)
	if err != nil {
		if errors.Is(err, ErrSpaceNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Destination not found")
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
			"slug":        input.Slug,
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

	_, ok = requireTenant(w, r)
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
	threshold := h.service.MinReputationToEdit(r.Context())
	if threshold > 0 {
		if h.points == nil {
			slog.Error("wiki: points checker is nil, cannot enforce reputation gate (fail-closed)")
			errpkg.WriteError(w, http.StatusInternalServerError, "reputation_check_unavailable",
				"Reputation verification is currently unavailable")

			return
		}

		points, err := h.points.GetPointsBalance(r.Context(), user.UserID)
		if err != nil {
			slog.Error("wiki: reputation check failed", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "reputation_check_failed",
				"Failed to verify reputation")

			return
		}

		if points < int64(threshold) {
			errpkg.WriteError(w, http.StatusForbidden, "insufficient_reputation",
				"You need more reputation to edit this wiki page")
			return
		}
	}

	var input SubmitRevisionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if len(input.Content) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_content", "Content is required")
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

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"revision_id": revID,
			"status":      "pending",
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
	_, ok = requireTenant(w, r)
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
		slog.Error("wiki: approve failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve revision")

		return
	}

	// B8: fire-and-forget RAG indexing of the approved content. The RAG
	// indexer's IndexEntity calls ExtractPlainText internally, which handles
	// TipTap JSON. Errors are logged, never propagated (Rule 8).
	if h.rag != nil && len(content) > 0 {
		// BUG-002 (rag-audit): resolve the indexing language from
		// i18n.default_language (§7 i18n) instead of passing an empty string.
		// Falls back to "en" when settings are not injected or the key is unset.
		language := "en"
		if h.settings != nil {
			language = h.settings.GetStringOr(r.Context(), "i18n.default_language", "en")
		}

		go func() { // #nosec G118 -- fire-and-forget: request ctx is cancelled on HTTP return (Rule 8)
			ctx := context.Background()
			if err := h.rag.IndexEntity(ctx, "wiki_page", pageID, string(content), language); err != nil {
				slog.Warn("wiki: RAG index failed on approval",
					"page_id", pageID.String(), "error", err)
			}
		}()
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"approved": true}})
}

// HandleRejectRevision handles POST /admin/wiki/:page_id/revisions/:id/reject.
// The frontend sends { "reason": "..." }; we accept both "reason" (UI field)
// and "notes" (legacy/direct-API alias) and persist to hitl_review_notes (B2).
func (h *Handlers) HandleRejectRevision(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	_, ok = requireTenant(w, r)
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

	if err := h.service.RejectRevision(r.Context(), revID, user.UserID, notes); err != nil {
		slog.Error("wiki: reject failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "reject_failed", "Failed to reject revision")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"rejected": true}})
}

// HandleUpdatePage handles PATCH /admin/wiki/:page_id — lock/unlock, feature,
// activate. The route middleware checks wiki.manage. When the is_locked field
// is being toggled, the handler additionally checks wiki.lock (§80 RBAC:
// wiki.lock is a distinct permission for lock/unlock, B7).
func (h *Handlers) HandleUpdatePage(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	_, ok = requireTenant(w, r)
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
	_, ok = requireTenant(w, r)
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
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var spaceID pgtype.UUID
	if v := r.URL.Query().Get("space_id"); v != "" {
		if err := spaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid space_id")
			return
		}
	}

	category := r.URL.Query().Get("category")
	q := r.URL.Query().Get("q")

	var lockedPtr *bool

	if v := r.URL.Query().Get("locked"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			lockedPtr = &b
		}
	}

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	pages, total, err := h.service.AdminListPages(r.Context(), spaceID, category, q, lockedPtr, page, limit)
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
