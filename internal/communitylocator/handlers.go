// Package communitylocator — handlers.go provides the HTTP handlers for
// the Diaspora Community Locator routes (§101, ADR-109).
//
// Routes:
//
//	GET   /api/v1/community-locator                      — list ?dest=&city=&origin=&type=&lat=&lng=&radius=
//	GET   /api/v1/community-locator/:slug                — single organisation
//	POST  /api/v1/community-locator                      — submit organisation (auth, community_locator.submit)
//	POST  /api/v1/community-locator/:id/claim            — initiate claim (auth, community_locator.claim)
//	GET   /api/v1/admin/community-locator                — moderation queue (admin, community_locator.review)
//	PATCH /api/v1/admin/community-locator/:id/approve    — publish (admin, community_locator.review)
//	PATCH /api/v1/admin/community-locator/:id/verify     — mark verified (admin, community_locator.manage)
//	PATCH /api/v1/admin/community-locator/:id/claim/:claim_id — resolve claim (admin, community_locator.manage)
//
// Ref: japaconnect-engineering/sections/101-diaspora-community-locator.md
//
//	japaconnect-engineering/adr/ADR-109-community-locator.md
package communitylocator

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
)

// Handlers exposes the Diaspora Community Locator endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
	lit      literalResolver // nil disables literal resolution (§14)
}

// literalResolver resolves {{...}} template literals in user-facing text
// fields (§14). Implemented by *literals.Catalog; nil disables resolution.
type literalResolver interface {
	ResolveFields(ctx context.Context, v any, rc literals.RenderContext)
}

// SetLiteralResolver injects the template-literal catalog so user-facing
// titles/descriptions render {{...}} placeholders (§14).
func (h *Handlers) SetLiteralResolver(l literalResolver) { h.lit = l }

// resolveLiterals resolves {{...}} placeholders on a response DTO or slice.
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

// NewHandlers creates community locator handlers bound to the provided
// service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as servicedir/handlers.go)
// ---------------------------------------------------------------------------

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

// parsePageParams reads ?page=&limit= from the query. limit is clamped to
// maxLimit (values above it return maxLimit, not the default — an explicit
// oversized page size should not silently shrink the result set).
func parsePageParams(r *http.Request, maxLimit int32) (page, limit int32) {
	page = 1
	limit = 20

	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = int32(n) // #nosec
		}
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = int32(n) // #nosec
			if limit > maxLimit {
				limit = maxLimit
			}
		}
	}

	return page, limit
}

// ---------------------------------------------------------------------------
// Public handlers (list + single — anonymous browse per §101)
// ---------------------------------------------------------------------------

// HandleList handles GET /community-locator — list published community
// organisations with filters. Query params:
// ?dest=&city=&origin=&type=&lat=&lng=&radius=&page=&limit=.
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "community_locator_disabled", "Community locator is not available")
		return
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	var citySpaceID pgtype.UUID
	if v := r.URL.Query().Get("city"); v != "" {
		if err := citySpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid city space ID")
			return
		}
	}

	originCommunity := r.URL.Query().Get("origin")
	orgType := r.URL.Query().Get("type")

	var lat, lng, radiusKm float64

	if v := r.URL.Query().Get("lat"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			lat = f
		}
	}

	if v := r.URL.Query().Get("lng"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			lng = f
		}
	}

	if v := r.URL.Query().Get("radius"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			radiusKm = f
		}
	}

	page, limit := parsePageParams(r, 100)

	orgs, total, err := h.service.List(r.Context(), destSpaceID, citySpaceID, originCommunity, orgType, lat, lng, radiusKm, page, limit)
	if err != nil {
		slog.Error("communitylocator: list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list community organisations")

		return
	}

	h.resolveLiterals(r, &orgs)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": orgs,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleOrgTypes handles GET /community-locator/org-types — returns the
// allow-list of organisation types as [{value,label}] so the frontend
// submit form can render the dropdown from the backend (Rule 16) instead
// of hardcoding the options. Public/anonymous (the list itself is not
// sensitive); feature-flagged with the rest of the routes.
func (h *Handlers) HandleOrgTypes(w http.ResponseWriter, r *http.Request) {

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "community_locator_disabled", "Community locator is not available")
		return
	}

	types := h.service.OrgTypes()
	h.resolveLiterals(r, &types)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": types})
}

// HandleGet handles GET /community-locator/:slug — single published
// community organisation.
func (h *Handlers) HandleGet(w http.ResponseWriter, r *http.Request) {

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "community_locator_disabled", "Community locator is not available")
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Organisation slug is required")
		return
	}

	org, err := h.service.GetBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "org_not_found", "Community organisation not found")
			return
		}

		slog.Error("communitylocator: get by slug failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get community organisation")

		return
	}

	h.resolveLiterals(r, &org)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": org})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (submit, claim — auth required)
// ---------------------------------------------------------------------------

// HandleSubmit handles POST /community-locator — submit a new community
// organisation. Auth required (community_locator.submit permission checked
// at route registration). Creates with status='pending'.
func (h *Handlers) HandleSubmit(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "community_locator_disabled", "Community locator is not available")
		return
	}

	var input SubmitInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "Name is required")
		return
	}

	if input.OrgType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_org_type", "org_type is required")
		return
	}

	if !validOrgTypes[input.OrgType] {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_org_type", "org_type must be one of: cultural_association, church, mosque, professional_network, student_union, informal_group, other")
		return
	}

	if input.OriginCommunity == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_origin_community", "origin_community is required")
		return
	}

	if input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination_space_id", "destination_space_id is required")
		return
	}

	if input.Slug == "" {
		input.Slug = slugify(input.Name)
	}

	org, err := h.service.Submit(r.Context(), user.UserID, input)
	if err != nil {
		if errors.Is(err, ErrDuplicateSlug) {
			errpkg.WriteError(w, http.StatusConflict, "duplicate_slug", "An organisation with this slug already exists")
			return
		}

		slog.Error("communitylocator: submit failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "submit_failed", "Failed to submit community organisation")

		return
	}

	h.resolveLiterals(r, &org)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": org})
}

// HandleClaim handles POST /community-locator/:id/claim — initiate the
// claim flow on a community organisation. Auth required
// (community_locator.claim permission checked at route registration).
func (h *Handlers) HandleClaim(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "community_locator_disabled", "Community locator is not available")
		return
	}

	if err := h.service.InitiateClaim(r.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "org_not_found", "Community organisation not found")
			return
		}

		slog.Error("communitylocator: claim failed", "error", err, "user_id", user.UserID)
		errpkg.WriteError(w, http.StatusInternalServerError, "claim_failed", "Failed to initiate claim")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"claim_status": ClaimStatusPending}})
}

// ---------------------------------------------------------------------------
// Admin handlers (moderation queue, approve, verify, claim resolution)
// ---------------------------------------------------------------------------

// HandleAdminList handles GET /admin/community-locator — moderation queue
// listing all community organisations (any status). Admin-only
// (community_locator.review permission checked at route registration).
// Supports ?dest=&origin=&type=&status=&q=&sort=&dir=&page=&limit=.
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "community_locator_unavailable", "Community locator not configured")
		return
	}

	// Geo-aware filter: ?dest= (destination space ID).
	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	originCommunity := r.URL.Query().Get("origin")
	orgType := r.URL.Query().Get("type")
	status := r.URL.Query().Get("status")
	q := r.URL.Query().Get("q")
	sort := r.URL.Query().Get("sort")
	dir := r.URL.Query().Get("dir")

	page, limit := parsePageParams(r, 200)

	orgs, total, err := h.service.AdminList(r.Context(), destSpaceID, originCommunity, orgType, status, q, sort, dir, page, limit)
	if err != nil {
		slog.Error("communitylocator: admin list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list community organisations")

		return
	}

	h.resolveLiterals(r, &orgs)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": orgs,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleAdminApprove handles PATCH /admin/community-locator/:id/approve —
// publish a community organisation. Admin-only (community_locator.review
// permission checked at route registration). Sets status='published',
// reviewed_by, reviewed_at.
func (h *Handlers) HandleAdminApprove(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "community_locator_unavailable", "Community locator not configured")
		return
	}

	org, err := h.service.ApproveOrg(r.Context(), id, user.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "org_not_found", "Community organisation not found")
			return
		}

		slog.Error("communitylocator: admin approve failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve community organisation")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": org})
}

// HandleAdminVerify handles PATCH /admin/community-locator/:id/verify —
// mark a community organisation as verified. Admin-only
// (community_locator.manage permission checked at route registration).
func (h *Handlers) HandleAdminVerify(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "community_locator_unavailable", "Community locator not configured")
		return
	}

	org, err := h.service.VerifyOrg(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "org_not_found", "Community organisation not found")
			return
		}

		slog.Error("communitylocator: admin verify failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "verify_failed", "Failed to verify community organisation")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": org})
}

// HandleAdminResolveClaim handles PATCH
// /admin/community-locator/:id/claim/:claim_id — resolve a pending claim
// on a community organisation. Admin-only (community_locator.manage
// permission checked at route registration). Claims are 1:1 with an org
// (a single pending claim_status per row — there is no separate claims
// table), so :claim_id is the org's own claim identifier and must match
// :id. Body: {"approve": true|false}.
func (h *Handlers) HandleAdminResolveClaim(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	claimID, ok := scanUUIDParam(w, r, "claim_id")
	if !ok {
		return
	}

	if claimID != id {
		errpkg.WriteError(w, http.StatusBadRequest, "claim_org_mismatch", "claim_id does not belong to this organisation")
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "community_locator_unavailable", "Community locator not configured")
		return
	}

	var body struct {
		Approve *bool `json:"approve"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if body.Approve == nil {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_approve", "approve field is required")
		return
	}

	org, err := h.service.ResolveClaim(r.Context(), id, *body.Approve)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "org_not_found", "Community organisation not found")
			return
		}

		slog.Error("communitylocator: admin resolve claim failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "resolve_claim_failed", "Failed to resolve claim")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": org})
}

// HandleAdminGenerate handles POST /admin/community-locator/generate —
// create an AI-generated draft community organisation entry (ADR-088
// universal AI content generation). Admin-only (community_locator.manage
// permission checked at route registration). The draft enters the HITL
// review queue and is not visible to end users until approved.
func (h *Handlers) HandleAdminGenerate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "community_locator_unavailable", "Community locator not configured")
		return
	}

	var input GenerateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "Name is required")
		return
	}

	if input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination_space_id", "destination_space_id is required")
		return
	}

	// ADR-088: the service calls the §20 AI Gateway (when a dispatcher
	// is wired via SetDispatcher) to populate the draft content, records
	// ai_model='content_agent_v1' and ai_prompt_hash=SHA256(prompt), and
	// inserts the row with status='ai_draft'. The draft enters the HITL
	// review queue and is not visible to end users until ApproveOrg.
	org, err := h.service.Generate(r.Context(), user.UserID, input)
	if err != nil {
		if errors.Is(err, ErrDuplicateSlug) {
			errpkg.WriteError(w, http.StatusConflict, "duplicate_slug", "An organisation with this slug already exists")
			return
		}

		slog.Error("communitylocator: admin generate failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "generate_failed", "Failed to generate community organisation draft")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": org})
}

// HandleAdminArchive handles PATCH /admin/community-locator/:id/archive —
// archive a published community organisation. Admin-only
// (community_locator.manage permission checked at route registration).
// Sets status='archived', is_active=false.
func (h *Handlers) HandleAdminArchive(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "community_locator_unavailable", "Community locator not configured")
		return
	}

	org, err := h.service.ArchiveOrg(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "org_not_found", "Community organisation not found")
			return
		}

		slog.Error("communitylocator: admin archive failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "archive_failed", "Failed to archive community organisation")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": org})
}

// HandleAdminDelete handles DELETE /admin/community-locator/:id —
// soft-delete a community organisation. Admin-only
// (community_locator.manage permission checked at route registration).
// Sets deleted_at=now() per Rule 11 (soft deletes only for user content).
func (h *Handlers) HandleAdminDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "community_locator_unavailable", "Community locator not configured")
		return
	}

	if err := h.service.DeleteOrg(r.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "org_not_found", "Community organisation not found")
			return
		}

		slog.Error("communitylocator: admin delete failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete community organisation")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
}
