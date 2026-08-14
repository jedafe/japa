// Package languagetesthub — handlers.go provides the HTTP handlers for
// the Language Test Preparation Hub routes (§99, ADR-107).
//
// Routes:
//
//	GET   /api/v1/language-tests                          — list tests (public)
//	GET   /api/v1/language-tests/requirements             — ?dest=&visa= required tests + scores (public)
//	GET   /api/v1/language-tests/:test_slug               — test hub page: resources + stories summary (public)
//	GET   /api/v1/language-tests/:test_slug/resources     — resources for a test (public)
//	GET   /api/v1/language-tests/:test_slug/stories       — score stories ?dest= (public)
//	POST  /api/v1/language-tests/:test_slug/stories       — submit score story (auth, language_test_hub.story)
//	GET   /api/v1/admin/language-tests/requirements       — manage requirements (admin, language_test_hub.manage)
//	POST  /api/v1/admin/language-tests/requirements       — create requirement (admin, language_test_hub.manage)
//	POST  /api/v1/admin/language-tests/requirements/generate — Content Agent draft placeholder (admin, language_test_hub.manage)
//	PATCH /api/v1/admin/language-tests/requirements/:id/approve — HITL approve (admin, language_test_hub.manage)
//
// Ref: japaconnect-engineering/sections/99-language-test-hub.md
//
//	japaconnect-engineering/adr/ADR-107-language-test-hub.md
package languagetesthub

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/worker"
)

// TaskEnqueueClient is the interface for enqueueing background tasks.
type TaskEnqueueClient interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Handlers exposes the Language Test Preparation Hub endpoints.
type Handlers struct {
	service     *Service
	settings    *config.Settings
	asynqClient TaskEnqueueClient
}

// NewHandlers creates language test hub handlers bound to the provided
// service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// SetAsynqClient injects the Asynq client for enqueueing background tasks
// (e.g. Meilisearch index sync on requirement approval).
func (h *Handlers) SetAsynqClient(c TaskEnqueueClient) { h.asynqClient = c }

// enqueueSearchIndex enqueues an async task to sync a language test
// requirement to the Meilisearch language_test_requirements index
// (ADR-088 JapaBot Indexability). operation is "index" or "delete".
func (h *Handlers) enqueueSearchIndex(reqID, tenantID pgtype.UUID, operation string) {
	if h.asynqClient == nil {
		return
	}
	payload, err := json.Marshal(map[string]string{
		"entity_id":   reqID.String(),
		"entity_type": "language_test_requirement",
		"tenant_id":   tenantID.String(),
		"operation":   operation,
	})
	if err != nil {
		slog.Warn("languagetesthub: failed to marshal search index payload", "error", err)
		return
	}
	if _, err := h.asynqClient.Enqueue(
		asynq.NewTask(worker.TypeSearchIndexSync, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeSearchIndexSync)),
	); err != nil {
		slog.Warn("languagetesthub: failed to enqueue search index task",
			"req_id", reqID.String(), "error", err)
	}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as servicedir/handlers.go)
// ---------------------------------------------------------------------------

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
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
			page = int32(n)
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n)
		}
	}
	return page, limit
}

// ---------------------------------------------------------------------------
// Public handlers (anonymous browse per §99)
// ---------------------------------------------------------------------------

// HandleListTests handles GET /language-tests — list all language tests.
func (h *Handlers) HandleListTests(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "language_test_hub_disabled", "Language test hub is not available")
		return
	}

	tests, err := h.service.ListTests(r.Context())
	if err != nil {
		slog.Error("languagetesthub: list tests failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list language tests")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tests})
}

// HandleListRequirements handles GET /language-tests/requirements —
// required tests + minimum scores for a destination/visa combination.
// Query params: ?dest=&visa=
func (h *Handlers) HandleListRequirements(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "language_test_hub_disabled", "Language test hub is not available")
		return
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	visaCategory := r.URL.Query().Get("visa")

	requirements, err := h.service.ListRequirements(r.Context(), tenantID, destSpaceID, visaCategory)
	if err != nil {
		slog.Error("languagetesthub: list requirements failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list requirements")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": requirements})
}

// HandleGetTest handles GET /language-tests/:test_slug — test hub page
// returning the test details plus a summary of resources and score
// stories.
func (h *Handlers) HandleGetTest(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "language_test_hub_disabled", "Language test hub is not available")
		return
	}

	slug := chi.URLParam(r, "test_slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Test slug is required")
		return
	}

	test, err := h.service.GetTestBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "test_not_found", "Language test not found")
			return
		}
		slog.Error("languagetesthub: get test failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get language test")
		return
	}

	resources, err := h.service.ListResources(r.Context(), tenantID, test.ID, 1, 5)
	if err != nil {
		slog.Error("languagetesthub: get test resources failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get language test")
		return
	}

	stories, err := h.service.ListStories(r.Context(), tenantID, test.ID, pgtype.UUID{}, 1, 5)
	if err != nil {
		slog.Error("languagetesthub: get test stories failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get language test")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"test":              test,
			"resources_summary": resources,
			"stories_summary":   stories,
		},
	})
}

// HandleListResources handles GET /language-tests/:test_slug/resources —
// list prep resources for a test.
func (h *Handlers) HandleListResources(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "language_test_hub_disabled", "Language test hub is not available")
		return
	}

	slug := chi.URLParam(r, "test_slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Test slug is required")
		return
	}

	test, err := h.service.GetTestBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "test_not_found", "Language test not found")
			return
		}
		slog.Error("languagetesthub: list resources get test failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get resources")
		return
	}

	page, limit := parsePageParams(r)

	resources, err := h.service.ListResources(r.Context(), tenantID, test.ID, page, limit)
	if err != nil {
		slog.Error("languagetesthub: list resources failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list resources")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": resources,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
		},
	})
}

// HandleListStories handles GET /language-tests/:test_slug/stories — list
// published score stories for a test. Query params: ?dest=&page=&limit=
func (h *Handlers) HandleListStories(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "language_test_hub_disabled", "Language test hub is not available")
		return
	}

	slug := chi.URLParam(r, "test_slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Test slug is required")
		return
	}

	test, err := h.service.GetTestBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "test_not_found", "Language test not found")
			return
		}
		slog.Error("languagetesthub: list stories get test failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get stories")
		return
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	page, limit := parsePageParams(r)

	stories, err := h.service.ListStories(r.Context(), tenantID, test.ID, destSpaceID, page, limit)
	if err != nil {
		slog.Error("languagetesthub: list stories failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list stories")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": stories,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
		},
	})
}

// HandleCreateStory handles POST /language-tests/:test_slug/stories —
// submit a score story. Auth required (language_test_hub.story
// permission checked at route registration).
func (h *Handlers) HandleCreateStory(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "language_test_hub_disabled", "Language test hub is not available")
		return
	}

	slug := chi.URLParam(r, "test_slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Test slug is required")
		return
	}

	var input CreateStoryRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.FinalScore == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_final_score", "final_score is required")
		return
	}

	story, err := h.service.CreateStory(r.Context(), tenantID, user.UserID, slug, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "test_not_found", "Language test not found")
			return
		}
		slog.Error("languagetesthub: create story failed", "error", err, "user_id", user.UserID)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to submit score story")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": story})
}

// ---------------------------------------------------------------------------
// Admin handlers (language_test_hub.manage permission)
// ---------------------------------------------------------------------------

// HandleAdminListRequirements handles GET /admin/language-tests/requirements
// — moderation queue listing all requirements (any status). Admin-only
// (language_test_hub.manage permission checked at route registration).
// Supports ?status=&dest=&q=&page=&limit=.
func (h *Handlers) HandleAdminListRequirements(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "language_test_hub_unavailable", "Language test hub not configured")
		return
	}

	status := r.URL.Query().Get("status")
	q := r.URL.Query().Get("q")

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	requirements, total, err := h.service.AdminListRequirements(r.Context(), tenantID, destSpaceID, status, q, page, limit)
	if err != nil {
		slog.Error("languagetesthub: admin list requirements failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list requirements")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": requirements,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleAdminCreateRequirement handles POST /admin/language-tests/requirements
// — create a requirement. Admin-only (language_test_hub.manage permission
// checked at route registration).
func (h *Handlers) HandleAdminCreateRequirement(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "language_test_hub_unavailable", "Language test hub not configured")
		return
	}

	var input CreateRequirementRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination", "destination_space_id is required")
		return
	}
	if input.TestSlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_test_slug", "test_slug is required")
		return
	}
	if input.MinimumScore == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_minimum_score", "minimum_score is required")
		return
	}

	requirement, err := h.service.CreateRequirement(r.Context(), tenantID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "test_not_found", "Language test not found")
			return
		}
		slog.Error("languagetesthub: admin create requirement failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create requirement")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": requirement})
}

// HandleAdminGenerateRequirement handles POST
// /admin/language-tests/requirements/generate — creates an AI-drafted
// requirement in 'ai_draft' status via the §20 AI Gateway (ADR-088
// Content Agent draft endpoint pattern). The draft is NOT visible to end
// users and requires HITL approval before publication. Admin-only
// (language_test_hub.manage permission checked at route registration).
func (h *Handlers) HandleAdminGenerateRequirement(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "language_test_hub_unavailable", "Language test hub not configured")
		return
	}

	var input GenerateRequirementRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination", "destination_space_id is required")
		return
	}
	if input.TestSlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_test_slug", "test_slug is required")
		return
	}

	draft, err := h.service.GenerateDraft(r.Context(), tenantID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "test_not_found", "Language test not found")
			return
		}
		slog.Error("languagetesthub: admin generate requirement failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "generate_failed", "Failed to generate requirement draft")
		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
		"data":    draft,
		"status":  "accepted",
		"message": "AI draft requirement created in 'ai_draft' status. Review and approve via PATCH /admin/language-tests/requirements/:id/approve before publication.",
	})
}

// HandleAdminApproveRequirement handles PATCH
// /admin/language-tests/requirements/:id/approve — HITL approve a
// requirement. Admin-only (language_test_hub.manage permission checked at
// route registration). Optional body: {"review_notes": "..."}
func (h *Handlers) HandleAdminApproveRequirement(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "language_test_hub_unavailable", "Language test hub not configured")
		return
	}

	var body ApproveRequirementRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		// Body is optional; ignore decode errors and proceed with empty notes.
		body = ApproveRequirementRequest{}
	}

	if err := h.service.ApproveRequirement(r.Context(), id, tenantID, user.UserID, body.ReviewNotes); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "requirement_not_found", "Requirement not found")
			return
		}
		slog.Error("languagetesthub: admin approve requirement failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve requirement")
		return
	}

	// Enqueue Meilisearch index sync so JapaBot RAG retrieval picks up the
	// newly published, HITL-reviewed requirement (ADR-088 JapaBot
	// Indexability). Fire-and-forget per Rule 8.
	h.enqueueSearchIndex(id, tenantID, "index")

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":               id.String(),
			"status":           StatusPublished,
			"hitl_reviewed_by": user.UserID.String(),
			"hitl_reviewed":    true,
		},
	})
}
