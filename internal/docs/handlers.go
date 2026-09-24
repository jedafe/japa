package docs

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
	"github.com/jedafe/japa/internal/publicapi"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/worker"
)

// TaskEnqueueClient is the interface for enqueueing background tasks.
type TaskEnqueueClient interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// PermissionChecker is the interface for RBAC permission checks.
type PermissionChecker interface {
	HasPermission(ctx context.Context, userID pgtype.UUID, permKey string) bool
}

// AudienceGates maps each audience to the RBAC permission node required to read
// it. Audiences not in this map (users, api) are public — no auth required.
// Ref: japaconnect-engineering/sections/44-documentation-system.md.
var AudienceGates = map[string]string{
	"premium":       "membership.pro",
	"organizations": "organizations.member",
	"moderators":    "moderation.queue.view",
	"admin":         "admin.panel.access",
	"superadmin":    "admin.panel.access", // super_admin group holds wildcard
}

// literalResolver resolves {{...}} template literals in user-facing text
// fields (§14). Implemented by *literals.Catalog; nil disables resolution.
type literalResolver interface {
	ResolveFields(ctx context.Context, v any, rc literals.RenderContext)
}

// Handlers holds dependencies for documentation HTTP handlers.
type Handlers struct {
	Service     *Service
	siteURL     string
	siteName    string
	AsynqClient TaskEnqueueClient
	rbac        PermissionChecker
	lit         literalResolver // nil disables literal resolution (§14)
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service, siteURL, siteName string) *Handlers {
	return &Handlers{Service: service, siteURL: siteURL, siteName: siteName}
}

// SetLiteralResolver injects the template-literal catalog so doc page
// titles/content render {{...}} placeholders (§14).
func (h *Handlers) SetLiteralResolver(l literalResolver) { h.lit = l }

// resolveLiterals resolves {{...}} placeholders on a doc DTO or slice.
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

// SetRBAC injects the RBAC resolver for audience-gating checks (§44).
func (h *Handlers) SetRBAC(r *rbac.RBAC) {
	if r == nil {
		return
	}

	h.rbac = r
}

// enqueueSearchIndex enqueues an async task to sync a doc page to the
// Meilisearch doc_pages index (SEARCH-001). operation is "index" or "delete".
func (h *Handlers) enqueueSearchIndex(pageID pgtype.UUID, operation string) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"entity_id":   pageID.String(),
		"entity_type": "doc_page",

		"operation": operation,
	})
	if err != nil {
		slog.Warn("docs: failed to marshal search index payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeSearchIndexSync, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeSearchIndexSync)),
	); err != nil {
		slog.Warn("docs: failed to enqueue search index task", "page_id", pageID.String(), "error", err)
	}
}

// checkAudienceGate verifies the requester may read the given audience.
// Public audiences (users, api) always pass. Gated audiences require auth
// and the RBAC permission node mapped in audienceGates (§44).
// Returns true if access is allowed; false and writes a 403 if denied.
func (h *Handlers) checkAudienceGate(w http.ResponseWriter, r *http.Request, audience string) bool {
	requiredPerm, gated := AudienceGates[audience]
	if !gated {
		return true // public audience
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusForbidden, "audience_forbidden", "This documentation section requires authentication")
		return false
	}

	if h.rbac == nil {
		// Fallback: deny if no RBAC resolver configured (defensive).
		errpkg.WriteError(w, http.StatusForbidden, "audience_forbidden", "Access denied")
		return false
	}

	if !h.rbac.HasPermission(r.Context(), user.UserID, requiredPerm) {
		errpkg.WriteError(w, http.StatusForbidden, "audience_forbidden", "You do not have access to this documentation section")
		return false
	}

	return true
}

// accessibleAudiences returns the list of audiences the requester may read,
// in canonical display order. Used to filter nav tree and search results.
func (h *Handlers) accessibleAudiences(r *http.Request) []string {
	user := auth.FromContext(r.Context())
	if user == nil || h.rbac == nil {
		// Guest or no RBAC: only public audiences.
		return []string{"users", "api"}
	}

	allowed := make([]string, 0, len(AudienceOrder))

	for _, aud := range AudienceOrder {
		requiredPerm, gated := AudienceGates[aud]
		if !gated {
			allowed = append(allowed, aud)
			continue
		}

		if h.rbac.HasPermission(r.Context(), user.UserID, requiredPerm) {
			allowed = append(allowed, aud)
		}
	}

	return allowed
}

// requireUser resolves the authenticated user from the request context.

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		errpkg.WriteError(w, http.StatusConflict, "already_exists", "Resource already exists")
		return
	}

	switch {
	default:
		errpkg.HandleServiceError(w, err)
	}
}

// ---------------------------------------------------------------------------
// Public Handlers (read-only, RBAC-gated by audience)
// ---------------------------------------------------------------------------

// ListDocPages handles GET /api/v1/docs/{audience}.
// Returns published pages for the given audience section.
func (h *Handlers) ListDocPages(w http.ResponseWriter, r *http.Request) {

	audience := chi.URLParam(r, "audience")
	if !ValidAudiences[audience] {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_audience", "Invalid audience section")
		return
	}

	if !h.checkAudienceGate(w, r, audience) {
		return
	}

	pages, err := h.Service.ListDocPagesByAudience(r.Context(), audience)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &pages)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": pages})
}

// GetDocPage handles GET /api/v1/docs/{audience}/{slug}.
// Returns a single published page.
func (h *Handlers) GetDocPage(w http.ResponseWriter, r *http.Request) {

	audience := chi.URLParam(r, "audience")
	slug := chi.URLParam(r, "slug")

	if !ValidAudiences[audience] {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_audience", "Invalid audience section")
		return
	}

	if !h.checkAudienceGate(w, r, audience) {
		return
	}

	page, err := h.Service.GetDocPageBySlug(r.Context(), audience, slug)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &page)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": page})
}

// ListDocNav handles GET /api/v1/docs/nav.
// Returns the published-pages navigation tree grouped by audience, filtered
// to only the audiences the requester may read (§44 — audience-gated nav).
func (h *Handlers) ListDocNav(w http.ResponseWriter, r *http.Request) {

	allowed := h.accessibleAudiences(r)

	nav, err := h.Service.GetDocNav(r.Context(), allowed)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &nav)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": nav})
}

// SearchDocPages handles GET /api/v1/docs/search?q=&audience=&limit=.
// Searches published pages by title or content. Results are filtered to
// only the audiences the requester may read (§44 — audience-filtered search).
// When audience is provided, results are further filtered to that section.
func (h *Handlers) SearchDocPages(w http.ResponseWriter, r *http.Request) {

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_query", "Query parameter q is required")
		return
	}

	audience := strings.TrimSpace(r.URL.Query().Get("audience"))
	if audience != "" && !ValidAudiences[audience] {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_audience", "Invalid audience section")
		return
	}

	// Restrict to accessible audiences (§44 — only return pages the requester can see).
	allowed := h.accessibleAudiences(r)
	if audience != "" {
		// If the requester asked for a specific audience they can't access, deny.
		allowedSet := make(map[string]bool, len(allowed))
		for _, a := range allowed {
			allowedSet[a] = true
		}

		if !allowedSet[audience] {
			errpkg.WriteError(w, http.StatusForbidden, "audience_forbidden", "You do not have access to this documentation section")
			return
		}

		allowed = []string{audience}
	}

	limit := int32(10)

	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 50 {
			limit = int32(n) // #nosec
		}
	}

	results, err := h.Service.SearchDocPagesInAudiences(r.Context(), q, allowed, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &results)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": results})
}

// SubmitDocPageFeedback handles POST /api/v1/docs/{audience}/{slug}/feedback.
// Increments the aggregate helpful_yes / helpful_no counters for a doc page.
// Requires authentication; guests may vote client-side until a captcha/anon
// path is added. CSRF is enforced by the route middleware (DOC-011).
func (h *Handlers) SubmitDocPageFeedback(w http.ResponseWriter, r *http.Request) {

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "auth_required", "Authentication required to submit feedback")
		return
	}

	audience := chi.URLParam(r, "audience")
	slug := chi.URLParam(r, "slug")

	if !ValidAudiences[audience] {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_audience", "Invalid documentation section")
		return
	}

	var input struct {
		Vote string `json:"vote"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if input.Vote != "up" && input.Vote != "down" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_vote", "Vote must be 'up' or 'down'")
		return
	}

	if err := h.Service.SubmitDocPageFeedback(r.Context(), audience, slug, input.Vote); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{"data": map[string]any{"status": "accepted"}})
}

// ListDocAudienceGates handles GET /api/v1/docs/audience-gates.
// Returns the RBAC permission node required to read each gated audience.
// Public audiences (users, api) are omitted — absence of a gate means public.
func (h *Handlers) ListDocAudienceGates(w http.ResponseWriter, r *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": AudienceGates})
}

// ListDocAudiences handles GET /api/v1/docs/audiences.
// Returns the canonical ordered list of documentation audience slugs.
// Public — used by the docs app nav switcher and the admin page (DOC-009).
func (h *Handlers) ListDocAudiences(w http.ResponseWriter, r *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": h.Service.ListAudiences()})
}

// ListDocStatuses handles GET /api/v1/docs/statuses.
// Returns the canonical ordered list of doc page review statuses.
// Gated by docs.manage at the route level (DOC-009).
func (h *Handlers) ListDocStatuses(w http.ResponseWriter, r *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": h.Service.ListStatuses()})
}

// ---------------------------------------------------------------------------
// Admin Handlers (write operations)
// ---------------------------------------------------------------------------

// ListAllDocPages handles GET /api/v1/admin/docs/pages.
// Returns all pages (any status) for admin listing. Optionally filters by ?audience=.
func (h *Handlers) ListAllDocPages(w http.ResponseWriter, r *http.Request) {

	audience := r.URL.Query().Get("audience")
	if audience != "" {
		if !ValidAudiences[audience] {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_audience", "Invalid audience section")
			return
		}

		pages, err := h.Service.ListAllDocPagesByAudience(r.Context(), audience)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": pages})

		return
	}

	pages, err := h.Service.ListAllDocPages(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": pages})
}

// CreateDocPage handles POST /api/v1/admin/docs/pages.
func (h *Handlers) CreateDocPage(w http.ResponseWriter, r *http.Request) {

	var req CreateDocPageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if req.Title == "" || req.Slug == "" || req.Audience == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "title, slug, and audience are required")
		return
	}

	pageID, err := h.Service.CreateDocPage(r.Context(), req)
	if err != nil {
		slog.Error("CreateDocPage failed", "err", err, "title", req.Title, "slug", req.Slug)
		handleServiceError(w, err)

		return
	}

	// Enqueue Meilisearch index sync for the new doc page.
	h.enqueueSearchIndex(pageID, "index")

	// Fetch the created page to return it (any status — use GetDocPageByID).
	page, fetchErr := h.Service.GetDocPageByID(r.Context(), pageID)
	if fetchErr != nil {
		// Page was created but we can't fetch it — return minimal success.
		errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"id": pageID.String(), "status": "created", "slug": req.Slug, "audience": req.Audience}})
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": page})
}

// GetAdminDocPage handles GET /api/v1/admin/docs/pages/{id}.
// Returns a single page by ID (any status) for admin detail view.
func (h *Handlers) GetAdminDocPage(w http.ResponseWriter, r *http.Request) {

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid page ID")
		return
	}

	page, err := h.Service.GetDocPageByID(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": page})
}

// UpdateDocPage handles PATCH /api/v1/admin/docs/pages/{id}.
func (h *Handlers) UpdateDocPage(w http.ResponseWriter, r *http.Request) {

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid page ID")
		return
	}

	var req UpdateDocPageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	// Title is required only when content is being updated (not for status-only
	// toggles like approve/publish which use dedicated handlers).
	if req.Title == "" && req.Content != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "title is required when updating content")
		return
	}

	if err := h.Service.UpdateDocPage(r.Context(), id, req); err != nil {
		handleServiceError(w, err)
		return
	}

	// SEARCH-001: enqueue Meilisearch index sync for the updated doc page.
	h.enqueueSearchIndex(id, "index")

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

// ApproveDocPage handles POST /api/v1/admin/docs/pages/{id}/approve.
// Sets review_status to 'approved' and records the reviewer (§44).
func (h *Handlers) ApproveDocPage(w http.ResponseWriter, r *http.Request) {

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid page ID")
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	if err := h.Service.ApproveDocPage(r.Context(), id, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	h.enqueueSearchIndex(id, "index")
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "approved"}})
}

// PublishDocPage handles POST /api/v1/admin/docs/pages/{id}/publish.
// Sets review_status to 'published'. Requires current status = 'approved' (§44).
func (h *Handlers) PublishDocPage(w http.ResponseWriter, r *http.Request) {

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid page ID")
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	if err := h.Service.PublishDocPage(r.Context(), id, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	h.enqueueSearchIndex(id, "index")
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "published"}})
}

// DeleteDocPage handles DELETE /api/v1/admin/docs/{id}.
func (h *Handlers) DeleteDocPage(w http.ResponseWriter, r *http.Request) {

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid page ID")
		return
	}

	if err := h.Service.DeleteDocPage(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	// SEARCH-001: enqueue Meilisearch document deletion for the removed doc page.
	h.enqueueSearchIndex(id, "delete")

	errpkg.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ---------------------------------------------------------------------------
// OpenAPI Spec (Section 44 — docs subdomain)
// ---------------------------------------------------------------------------

// OpenAPIHandler serves the OpenAPI 3.0.3 specification as JSON.
func (h *Handlers) OpenAPIHandler(w http.ResponseWriter, _ *http.Request) {
	// Canonical spec lives in internal/publicapi (the more complete generator);
	// serving it here too avoids two independently hand-maintained specs
	// drifting apart (§6.4 dedupe).
	spec := publicapi.GetPublicOpenAPISpec(h.siteURL, h.siteName)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(spec); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}
