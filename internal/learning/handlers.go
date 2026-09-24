// Package learning provides HTTP handlers for the learning management system.
package learning

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/content"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
	"github.com/jedafe/japa/internal/worker"
)

// TaskEnqueueClient is the interface for enqueueing background tasks.
type TaskEnqueueClient interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// JSONLDResolver resolves admin-managed JSON-LD templates (Section 14 /
// Section 26). The learning platform uses it to emit Course structured data
// from the active template.
type JSONLDResolver interface {
	ResolveJSONLD(ctx context.Context, contentType string, fields map[string]string) (json.RawMessage, error)
}

// RAGEntityIndexer is the subset of the RAG indexer that learning needs to
// index track content into content_embeddings (§69/ADR-069). Implemented
// by rag.Service; nil disables RAG indexing (graceful degradation).
type RAGEntityIndexer interface {
	IndexEntity(ctx context.Context, entityType string, entityID pgtype.UUID, visibility string, spaceID, authorID pgtype.UUID, body, language string) error
	PurgeEntity(ctx context.Context, entityType string, entityID pgtype.UUID) error
}

// settingsReader is the subset of config.Settings needed by the learning
// handlers to resolve hot-reloadable system_settings (e.g.
// i18n.default_language for RAG indexing). Nil falls back to defaults.
type settingsReader interface {
	GetStringOr(ctx context.Context, key, fallback string) string
}

// literalResolver resolves {{...}} template literals in user-facing text
// fields (§14 Global Template Literals). Implemented by *literals.Catalog;
// nil disables resolution.
type literalResolver interface {
	ResolveFields(ctx context.Context, v any, rc literals.RenderContext)
}

// Handlers exposes HTTP endpoints for the learning platform. Each handler
// reads platform context from the request, parses input, delegates to the
// store, and writes a JSON response. Guard clauses at the top of every
// handler keep the happy path un-indented.
type Handlers struct {
	store          *Store
	AsynqClient    TaskEnqueueClient
	jsonldResolver JSONLDResolver
	ragIndexer     RAGEntityIndexer // nil disables RAG indexing (§69)
	settings       settingsReader   // nil falls back to defaults
	lit            literalResolver  // nil disables literal resolution (§14)
}

// NewHandlers creates a learning handler bound to the provided store. A nil
// resolver is acceptable — JSON-LD is simply omitted when unavailable.
func NewHandlers(store *Store, resolver JSONLDResolver) *Handlers {
	return &Handlers{store: store, jsonldResolver: resolver}
}

// SetRAGIndexer injects the RAG indexer for embedding learning tracks into
// content_embeddings (§69/ADR-069 BUG-001 fix). nil disables RAG indexing.
func (h *Handlers) SetRAGIndexer(r RAGEntityIndexer) { h.ragIndexer = r }

// SetSettings injects the hot-reloadable settings reader for resolving
// system_settings (e.g. i18n.default_language for RAG indexing). nil falls
// back to compiled-in defaults.
func (h *Handlers) SetSettings(s settingsReader) { h.settings = s }

// SetLiteralResolver injects the template-literal catalog so track, module,
// and lesson text renders {{...}} placeholders (§14).
func (h *Handlers) SetLiteralResolver(l literalResolver) { h.lit = l }

// resolveLiterals resolves {{...}} placeholders on a learning DTO or slice.
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

// resolveRAGLanguage resolves the indexing language from i18n.default_language
// (§7 i18n). Falls back to "en" when settings are not injected or the key is
// unset. BUG-002 (rag-audit): replaces hardcoded "en" at the call sites.
func (h *Handlers) resolveRAGLanguage(ctx context.Context) string {
	if h.settings != nil {
		return h.settings.GetStringOr(ctx, "i18n.default_language", "en")
	}

	return "en"
}

// indexTrackRAG fires-and-forgets a RAG indexing call for a learning track.
// The body is the track title + description concatenated for embedding.
// Errors are logged, never propagated (Rule 8).
func (h *Handlers) indexTrackRAG(ctx context.Context, trackID pgtype.UUID, title string, description *string, language string) {
	if h.ragIndexer == nil {
		return
	}

	text := title
	if description != nil && *description != "" {
		if text != "" {
			text += "\n\n"
		}

		text += *description
	}

	if text == "" {
		return
	}

	if language == "" {
		language = "en"
	}

	go func() { // #nosec G118 -- fire-and-forget: request ctx is cancelled on HTTP return (Rule 8)
		bgCtx := context.Background()
		// Learning tracks are a public corpus — no scoped visibility (§69).
		if err := h.ragIndexer.IndexEntity(bgCtx, "learning_track", trackID, "public", pgtype.UUID{}, pgtype.UUID{}, text, language); err != nil {
			slog.Warn("learning: RAG index failed for track", "track_id", trackID.String(), "error", err)
		}
	}()
}

// purgeTrackRAG fires-and-forgets a RAG purge for a deleted/unpublished track.
func (h *Handlers) purgeTrackRAG(ctx context.Context, trackID pgtype.UUID) {
	if h.ragIndexer == nil {
		return
	}

	go func() { // #nosec G118 -- fire-and-forget: request ctx is cancelled on HTTP return (Rule 8)
		bgCtx := context.Background()
		if err := h.ragIndexer.PurgeEntity(bgCtx, "learning_track", trackID); err != nil {
			slog.Warn("learning: RAG purge failed for track", "track_id", trackID.String(), "error", err)
		}
	}()
}

// enqueueTrackCompletionUpdate enqueues an async track completion update task.
// Fire-and-forget: errors are logged but never propagated.
func (h *Handlers) enqueueTrackCompletionUpdate(userID, trackID pgtype.UUID) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"user_id":  userID.String(),
		"track_id": trackID.String(),
	})
	if err != nil {
		slog.Warn("learning: failed to marshal track completion payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeLearningUpdateTrackCompletion, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeLearningUpdateTrackCompletion)),
	); err != nil {
		slog.Warn("learning: failed to enqueue track completion task", "user_id", userID.String(), "track_id", trackID.String(), "error", err)
	}
}

// ---------------------------------------------------------------------------
// Public / authenticated user endpoints
// ---------------------------------------------------------------------------

// ListPublishedTracks returns all published tracks for the current platform.
func (h *Handlers) ListPublishedTracks(w http.ResponseWriter, r *http.Request) {

	tracks, err := h.store.ListPublishedTracks(r.Context())
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to list tracks")
		return
	}

	h.resolveLiterals(r, &tracks)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tracks})
}

// GetTrackDetail returns a published track with its modules and nested lessons
// as a single flat DTO. Per Section 36 the route returns "track detail: modules,
// lessons, instructor" in one response. The frontend TrackDetail type (ADR-060)
// expects a flat object with a `modules` array where each module carries its
// `lessons` — NOT the legacy `{track, modules}` envelope.
func (h *Handlers) GetTrackDetail(w http.ResponseWriter, r *http.Request) {

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Track slug is required")
		return
	}

	detail, err := h.store.GetTrackDetail(r.Context(), slug)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "track_not_found", "Track not found")
		return
	}

	h.resolveLiterals(r, detail)

	// JSON-LD structured data (Section 14 / Section 26). Resolve the
	// admin-managed Course template, interpolating the track's field values.
	// Best-effort: a missing template or resolution error degrades to no
	// json_ld field. Emitted as a sibling of `data` for frontend consumption.
	resp := map[string]any{"data": detail}

	if h.jsonldResolver != nil {
		if jsonLd, jErr := h.jsonldResolver.ResolveJSONLD(r.Context(), "Course", trackJSONLDFields(detail)); jErr == nil && len(jsonLd) > 0 {
			resp["json_ld"] = json.RawMessage(jsonLd)
		} else if jErr != nil {
			slog.Warn("learning: JSON-LD template resolution failed", "slug", slug, "error", jErr)
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, resp)
}

// trackJSONLDFields builds the {{field_key}} → value map for interpolating a
// Course JSON-LD template from a track detail.
func trackJSONLDFields(d *TrackDetail) map[string]string {
	desc := ""
	if d.Description != nil {
		desc = *d.Description
	}

	coverURL := ""
	if d.ThumbnailURL != nil {
		coverURL = *d.ThumbnailURL
	}

	instructor := ""
	if d.InstructorName != nil {
		instructor = *d.InstructorName
	}

	return map[string]string{
		"title":           d.Title,
		"description":     desc,
		"cover_image_url": coverURL,
		"language":        d.Language,
		"duration":        d.DurationISO,
		"instructor":      instructor,
	}
}

// GetLesson returns a single lesson by its slug within the track/module hierarchy.
func (h *Handlers) GetLesson(w http.ResponseWriter, r *http.Request) {

	trackSlug := chi.URLParam(r, "trackSlug")
	moduleSlug := chi.URLParam(r, "moduleSlug")
	lessonSlug := chi.URLParam(r, "lessonSlug")

	if trackSlug == "" || moduleSlug == "" || lessonSlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_params", "Track, module, and lesson slugs are required")
		return
	}

	track, err := h.store.GetTrackBySlug(r.Context(), trackSlug)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "track_not_found", "Track not found")
		return
	}

	module, err := h.store.GetModuleBySlug(r.Context(), track.ID, moduleSlug)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "module_not_found", "Module not found")
		return
	}

	// Use the enriched LessonDetail (ADR-060) so the frontend receives
	// module_title, track context, content_html, and prev/next navigation.
	// Pass the authenticated user's ID (if any) so IsCompleted is populated
	// from lesson_progress — the lesson player shows the "Completed" state
	// on SSR render without waiting for CSR.
	var userID pgtype.UUID
	if user := auth.FromContext(r.Context()); user != nil {
		userID = user.UserID
	}

	lesson, err := h.store.GetLessonDetail(r.Context(), module.ID, lessonSlug, userID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "lesson_not_found", "Lesson not found")
		return
	}

	// Access control (Section 36 / ADR-060): preview lessons are public; every
	// other lesson requires an authenticated enrollment in the parent track.
	// The frontend redirects a 403 to the track's enroll page.
	if !lesson.IsPreview {
		enrolled := false
		if userID.Valid {
			if _, e := h.store.GetEnrollment(r.Context(), userID, track.ID); e == nil {
				enrolled = true
			} else if !errors.Is(e, pgx.ErrNoRows) {
				errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to check enrollment")
				return
			}
		}
		if !enrolled {
			errpkg.WriteError(w, http.StatusForbidden, "not_enrolled", "Enroll in this track to access this lesson")
			return
		}
	}

	h.resolveLiterals(r, lesson)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": lesson})
}

// UpdateProgress records or updates a user's progress on a lesson.
func (h *Handlers) UpdateProgress(w http.ResponseWriter, r *http.Request) {

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	lessonIDStr := chi.URLParam(r, "id")

	var lessonID pgtype.UUID

	if err := lessonID.Scan(lessonIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid lesson ID")
		return
	}

	// Verify the lesson exists and belongs to this platform.
	lesson, err := h.store.GetLessonByID(r.Context(), lessonID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "lesson_not_found", "Lesson not found")
		return
	}

	_ = lesson // lesson validated; proceed with progress update

	var input UpdateProgressInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if input.Status == "" {
		input.Status = "in_progress"
	}

	if input.Status != "not_started" && input.Status != "in_progress" && input.Status != "completed" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", "Status must be not_started, in_progress, or completed")
		return
	}

	progress, err := h.store.UpsertProgress(r.Context(), user.UserID, lessonID, input)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to update progress")
		return
	}

	// When a lesson is marked completed, synchronously update the enrollment's
	// progress_pct (same rationale as CompleteLesson — the async worker may
	// not be running).
	if input.Status == "completed" {
		mod, merr := h.store.GetModuleByID(r.Context(), lesson.ModuleID)
		if merr == nil {
			completed, total, perr := h.store.GetUserTrackProgress(r.Context(), user.UserID, mod.TrackID)
			if perr == nil && total > 0 {
				pct := float64(completed) / float64(total) * 100
				if uerr := h.store.UpdateEnrollmentProgress(r.Context(), user.UserID, mod.TrackID, pct); uerr != nil {
					slog.Warn("learning: update_progress — enrollment progress update failed", "user_id", user.UserID.String(), "track_id", mod.TrackID.String(), "error", uerr)
				}
			}
		}

		h.maybeIssueCertificate(r.Context(), user.UserID, lesson.ModuleID)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": progress})
}

// GetMyCertificates returns all certificates earned by the authenticated user.
func (h *Handlers) GetMyCertificates(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	certs, err := h.store.ListCertificatesByUser(r.Context(), user.UserID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to list certificates")
		return
	}

	h.resolveLiterals(r, &certs)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": certs})
}

// ListCertificatesByUser handles GET /users/{id}/certificates — public earned-
// certificate badges (social proof). All issued certs are public; non-active
// users are already 404'd by the requireActiveProfileUser gate on the route.
func (h *Handlers) ListCertificatesByUser(w http.ResponseWriter, r *http.Request) {

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID format")
		return
	}

	certs, err := h.store.ListCertificatesByUser(r.Context(), userID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to list certificates")
		return
	}

	h.resolveLiterals(r, &certs)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": certs})
}

// ---------------------------------------------------------------------------
// Admin endpoints (require learning.admin.manage permission)
// ---------------------------------------------------------------------------

// ListAllTracks returns every track for the platform (admin view).
func (h *Handlers) ListAllTracks(w http.ResponseWriter, r *http.Request) {

	tracks, err := h.store.ListAllTracks(r.Context())
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to list tracks")
		return
	}

	h.resolveLiterals(r, &tracks)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tracks})
}

// CreateTrack creates a new learning track.
func (h *Handlers) CreateTrack(w http.ResponseWriter, r *http.Request) {

	var input CreateTrackInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}

	if input.Slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	input.Slug = content.Slugify(input.Slug)
	if input.Slug == "" || input.Slug == "untitled" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_slug", "Slug could not be normalized")
		return
	}

	if input.Difficulty == "" {
		input.Difficulty = "beginner"
	}

	track, err := h.store.CreateTrack(r.Context(), input)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to create track")
		return
	}

	// RAG indexing — fire-and-forget (§69/ADR-069 BUG-001 fix: learning
	// tracks are part of the RAG corpus). Only index published tracks.
	if input.IsPublished {
		h.indexTrackRAG(r.Context(), track.ID, input.Title, input.Description, h.resolveRAGLanguage(r.Context()))
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": track})
}

// UpdateTrack updates an existing track.
func (h *Handlers) UpdateTrack(w http.ResponseWriter, r *http.Request) {

	idStr := chi.URLParam(r, "id")

	var id pgtype.UUID

	if err := id.Scan(idStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid track ID")
		return
	}

	var input UpdateTrackInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	track, err := h.store.UpdateTrack(r.Context(), id, input)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to update track")
		return
	}

	// RAG re-indexing — fire-and-forget (§69/ADR-069 BUG-001 fix). Only
	// published tracks are indexed; unpublished tracks are purged.
	if input.IsPublished {
		h.indexTrackRAG(r.Context(), id, input.Title, input.Description, h.resolveRAGLanguage(r.Context()))
	} else {
		h.purgeTrackRAG(r.Context(), id)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": track})
}

// DeleteTrack removes a track and cascades to its modules and lessons.
func (h *Handlers) DeleteTrack(w http.ResponseWriter, r *http.Request) {

	idStr := chi.URLParam(r, "id")

	var id pgtype.UUID

	if err := id.Scan(idStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid track ID")
		return
	}

	if err := h.store.DeleteTrack(r.Context(), id); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to delete track")
		return
	}

	// RAG purge — fire-and-forget (§69/ADR-069 BUG-001 fix: deleted tracks
	// must not be retrievable in RAG).
	h.purgeTrackRAG(r.Context(), id)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// CreateModule adds a module to a track.
func (h *Handlers) CreateModule(w http.ResponseWriter, r *http.Request) {

	trackIDStr := chi.URLParam(r, "trackId")

	var trackID pgtype.UUID

	if err := trackID.Scan(trackIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid track ID")
		return
	}

	var input CreateModuleInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}

	if input.Slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	input.Slug = content.Slugify(input.Slug)
	if input.Slug == "" || input.Slug == "untitled" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_slug", "Slug could not be normalized")
		return
	}

	module, err := h.store.CreateModule(r.Context(), trackID, input)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to create module")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": module})
}

// UpdateModule updates an existing module.
func (h *Handlers) UpdateModule(w http.ResponseWriter, r *http.Request) {

	idStr := chi.URLParam(r, "id")

	var id pgtype.UUID

	if err := id.Scan(idStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid module ID")
		return
	}

	var input UpdateModuleInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	module, err := h.store.UpdateModule(r.Context(), id, input)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to update module")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": module})
}

// DeleteModule removes a module and cascades to its lessons.
func (h *Handlers) DeleteModule(w http.ResponseWriter, r *http.Request) {

	idStr := chi.URLParam(r, "id")

	var id pgtype.UUID

	if err := id.Scan(idStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid module ID")
		return
	}

	if err := h.store.DeleteModule(r.Context(), id); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to delete module")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// CreateLesson adds a lesson to a module.
func (h *Handlers) CreateLesson(w http.ResponseWriter, r *http.Request) {

	moduleIDStr := chi.URLParam(r, "moduleId")

	var moduleID pgtype.UUID

	if err := moduleID.Scan(moduleIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid module ID")
		return
	}

	var input CreateLessonInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}

	if input.Slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	input.Slug = content.Slugify(input.Slug)
	if input.Slug == "" || input.Slug == "untitled" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_slug", "Slug could not be normalized")
		return
	}

	if input.LessonType == "" {
		input.LessonType = "article"
	}

	lesson, err := h.store.CreateLesson(r.Context(), moduleID, input)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to create lesson")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": lesson})
}

// UpdateLesson updates an existing lesson.
func (h *Handlers) UpdateLesson(w http.ResponseWriter, r *http.Request) {

	idStr := chi.URLParam(r, "id")

	var id pgtype.UUID

	if err := id.Scan(idStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid lesson ID")
		return
	}

	var input UpdateLessonInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	lesson, err := h.store.UpdateLesson(r.Context(), id, input)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to update lesson")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": lesson})
}

// DeleteLesson removes a lesson.
func (h *Handlers) DeleteLesson(w http.ResponseWriter, r *http.Request) {

	idStr := chi.URLParam(r, "id")

	var id pgtype.UUID

	if err := id.Scan(idStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid lesson ID")
		return
	}

	if err := h.store.DeleteLesson(r.Context(), id); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to delete lesson")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// Enrollment handlers
// ---------------------------------------------------------------------------

// EnrollInTrack handles POST /learn/university/tracks/:id/enroll (and legacy
// POST /learn/tracks/:id/enroll). Enrollment logic:
// - Free tracks (price_kobo = 0/null, not subscription_eligible): immediate
// enrollment with access_type="free".
// - Subscription-eligible tracks: check the user's active subscription plan.
// If the plan covers the track (either explicitly via
// subscription_plan_tracks or implicitly because the plan has no entries =
// all tracks), enroll with access_type="subscription" and enqueue a
// pro_stream revenue split for the creator. If no active subscription or
// the plan doesn't cover it, fall through to the paid check.
// - Paid tracks (price_kobo > 0): return 402 payment_required so the
// frontend can show the checkout modal.
func (h *Handlers) EnrollInTrack(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	trackIDStr := chi.URLParam(r, "id")

	var trackID pgtype.UUID

	if err := trackID.Scan(trackIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid track ID")
		return
	}

	track, err := h.store.GetTrackByID(r.Context(), trackID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "track_not_found", "Track not found")
		return
	}

	if !track.IsPublished {
		errpkg.WriteError(w, http.StatusNotFound, "track_not_found", "Track not found")
		return
	}

	// Subscription-eligible tracks: check if the user's plan covers this track.
	if track.SubscriptionEligible {
		hasAccess, planSlug := h.store.CheckSubscriptionAccess(r.Context(), user.UserID, trackID)
		if hasAccess {
			enrollment, err := h.store.EnrollUser(r.Context(), user.UserID, trackID, "subscription")
			if err != nil {
				errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to enroll")
				return
			}
			// Enqueue a pro_stream revenue split for the creator (async,
			// fire-and-forget per Rule 8). The university:process_enrollment
			// task handles the revenue split with source='pro_stream'.
			if h.AsynqClient != nil {
				payload, err := json.Marshal(map[string]string{

					"user_id":  user.UserID.String(),
					"track_id": trackIDStr,
					"source":   "pro_stream",
				})
				if err != nil {
					slog.Warn("learning: marshal pro_stream enrollment payload", "error", err)
				} else {
					task := asynq.NewTask("university:process_enrollment", payload)
					if _, err := h.AsynqClient.Enqueue(task); err != nil {
						slog.Warn("learning: enqueue pro_stream enrollment task", "track_id", trackIDStr, "error", err)
					}
				}
			}

			_ = planSlug // planSlug reserved for future logging/metrics

			errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": enrollment})

			return
		}
		// No subscription access — fall through to paid check below.
	}

	// Paid course check: if the track has a price > 0, return 402 so the
	// frontend can show the checkout modal.
	if track.PriceKobo != nil && *track.PriceKobo > 0 {
		errpkg.WriteJSON(w, http.StatusPaymentRequired, map[string]any{
			"error": map[string]any{
				"code":    "payment_required",
				"message": "Payment required to enroll in this course",
				"status":  http.StatusPaymentRequired,
			},
			"track_id":              trackIDStr,
			"price_kobo":            *track.PriceKobo,
			"currency":              track.Currency,
			"subscription_eligible": track.SubscriptionEligible,
		})

		return
	}

	// Free track, not subscription-eligible (or no active subscription).
	accessType := "free"

	enrollment, err := h.store.EnrollUser(r.Context(), user.UserID, trackID, accessType)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to enroll")
		return
	}

	// Return the enrollment directly as `data` so the frontend api.post<Enrollment>
	// unwraps it correctly (the client reads body.data).
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": enrollment})
}

// ListMyEnrollments handles GET /me/enrollments.
func (h *Handlers) ListMyEnrollments(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	enrollments, err := h.store.ListEnrollmentsByUser(r.Context(), user.UserID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to list enrollments")
		return
	}

	h.resolveLiterals(r, &enrollments)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": enrollments})
}

// ---------------------------------------------------------------------------
// Lesson by ID handler
// ---------------------------------------------------------------------------

// GetLessonByID handles GET /learn/lessons/:id.
func (h *Handlers) GetLessonByID(w http.ResponseWriter, r *http.Request) {

	lessonIDStr := chi.URLParam(r, "id")

	var lessonID pgtype.UUID

	if err := lessonID.Scan(lessonIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid lesson ID")
		return
	}

	lesson, err := h.store.GetLessonByID(r.Context(), lessonID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "lesson_not_found", "Lesson not found")
		return
	}

	// Same access control as GetLesson (Section 36): non-preview lessons
	// require an enrollment in the parent track. Lesson UUIDs are exposed in
	// the public track-detail payload, so authentication alone is not enough.
	if !lesson.IsPreview {
		enrolled := false
		if module, e := h.store.GetModuleByID(r.Context(), lesson.ModuleID); e == nil {
			user := auth.FromContext(r.Context())
			if user == nil {
				errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
				return
			}
			if _, e := h.store.GetEnrollment(r.Context(), user.UserID, module.TrackID); e == nil {
				enrolled = true
			} else if !errors.Is(e, pgx.ErrNoRows) {
				errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to check enrollment")
				return
			}
		}
		if !enrolled {
			errpkg.WriteError(w, http.StatusForbidden, "not_enrolled", "Enroll in this track to access this lesson")
			return
		}
	}

	h.resolveLiterals(r, lesson)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": lesson})
}

// ---------------------------------------------------------------------------
// Complete lesson shortcut
// ---------------------------------------------------------------------------

// CompleteLesson handles POST /learn/lessons/:id/complete.
func (h *Handlers) CompleteLesson(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	lessonIDStr := chi.URLParam(r, "id")

	var lessonID pgtype.UUID

	if err := lessonID.Scan(lessonIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid lesson ID")
		return
	}

	lesson, err := h.store.GetLessonByID(r.Context(), lessonID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "lesson_not_found", "Lesson not found")
		return
	}

	// Resolve the module to get the correct track ID.
	mod, err := h.store.GetModuleByID(r.Context(), lesson.ModuleID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "module_not_found", "Module not found")
		return
	}

	progress, err := h.store.UpsertProgress(r.Context(), user.UserID, lessonID, UpdateProgressInput{Status: "completed"})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to complete lesson")
		return
	}

	// Synchronously update the enrollment's progress_pct. The async worker
	// (enqueueTrackCompletionUpdate) may not be running in all environments,
	// so we compute the percentage here to guarantee the enrollment record
	// reflects the latest completion state. This is a cheap COUNT query.
	completed, total, perr := h.store.GetUserTrackProgress(r.Context(), user.UserID, mod.TrackID)
	if perr == nil && total > 0 {
		pct := float64(completed) / float64(total) * 100
		if uerr := h.store.UpdateEnrollmentProgress(r.Context(), user.UserID, mod.TrackID, pct); uerr != nil {
			slog.Warn("learning: complete_lesson — enrollment progress update failed", "user_id", user.UserID.String(), "track_id", mod.TrackID.String(), "error", uerr)
		}
	}

	h.maybeIssueCertificate(r.Context(), user.UserID, lesson.ModuleID)

	// Enqueue async track completion update — fire-and-forget (best-effort
	// backup for environments where the worker is running).
	h.enqueueTrackCompletionUpdate(user.UserID, mod.TrackID)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": progress})
}

// ---------------------------------------------------------------------------
// Quiz handlers
// ---------------------------------------------------------------------------

// GetQuiz handles GET /learn/quizzes/:id.
func (h *Handlers) GetQuiz(w http.ResponseWriter, r *http.Request) {

	lessonIDStr := chi.URLParam(r, "id")

	var lessonID pgtype.UUID

	if err := lessonID.Scan(lessonIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid lesson ID")
		return
	}

	questions, err := h.store.GetQuizQuestions(r.Context(), lessonID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to get quiz questions")
		return
	}

	h.resolveLiterals(r, &questions)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": questions})
}

// SubmitQuiz handles POST /learn/quizzes/:id/submit.
func (h *Handlers) SubmitQuiz(w http.ResponseWriter, r *http.Request) {

	lessonIDStr := chi.URLParam(r, "id")

	var lessonID pgtype.UUID

	if err := lessonID.Scan(lessonIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid lesson ID")
		return
	}

	var input struct {
		Answers map[string]string `json:"answers"` // question_id -> answer
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	questions, err := h.store.GetQuizQuestionsWithAnswers(r.Context(), lessonID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to get quiz questions")
		return
	}

	correct := 0
	total := len(questions)
	correctAnswers := make(map[string]bool)

	for _, q := range questions {
		qID := q.ID.String()
		if input.Answers[qID] == q.Answer {
			correct++
			correctAnswers[qID] = true
		}
	}

	score := 0.0
	if total > 0 {
		score = float64(correct) / float64(total) * 100
	}

	passed := score >= 70

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"score":           score,
			"passed":          passed,
			"correct_answers": correctAnswers,
			"total":           total,
			"correct":         correct,
		},
	})
}

// ---------------------------------------------------------------------------
// Certificate handlers
// ---------------------------------------------------------------------------

// GetCertificateDetail handles GET /certificates/:id.
func (h *Handlers) GetCertificateDetail(w http.ResponseWriter, r *http.Request) {
	certIDStr := chi.URLParam(r, "id")

	var certID pgtype.UUID

	if err := certID.Scan(certIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid certificate ID")
		return
	}

	cert, err := h.store.GetCertificateByID(r.Context(), certID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "certificate_not_found", "Certificate not found")
		return
	}

	h.resolveLiterals(r, cert)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": cert})
}

// DownloadCertificate handles GET /certificates/:id/download.
func (h *Handlers) DownloadCertificate(w http.ResponseWriter, r *http.Request) {
	certIDStr := chi.URLParam(r, "id")

	var certID pgtype.UUID

	if err := certID.Scan(certIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid certificate ID")
		return
	}

	cert, err := h.store.GetCertificateByID(r.Context(), certID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "certificate_not_found", "Certificate not found")
		return
	}

	if cert.MediaID != nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"download_url": "/api/v1/media/" + cert.MediaID.String(),
			},
		})

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"certificate_number": cert.CertificateNumber,
			"issued_at":          cert.IssuedAt,
			"message":            "PDF generation pending",
		},
	})
}

// GetTrackProgress returns the user's progress for a specific track.
func (h *Handlers) GetTrackProgress(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	trackIDStr := chi.URLParam(r, "trackId")

	var trackID pgtype.UUID

	if err := trackID.Scan(trackIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid track ID")
		return
	}

	completed, total, err := h.store.GetUserTrackProgress(r.Context(), user.UserID, trackID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to get progress")
		return
	}

	// Fetch the completed lesson IDs so the curriculum rail can restore
	// completion state on page refresh without checking each lesson.
	completedLessonIDs, _ := h.store.ListCompletedLessonIDsByUserAndTrack(r.Context(), user.UserID, trackID)
	completedIDs := make([]string, 0, len(completedLessonIDs))

	for _, id := range completedLessonIDs {
		if id.Valid {
			completedIDs = append(completedIDs, id.String())
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"completed":            completed,
			"total":                total,
			"percent":              calculatePercent(completed, total),
			"completed_lesson_ids": completedIDs,
		},
	})
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// maybeIssueCertificate checks if the user has completed all lessons in a
// track and issues a certificate if so. Errors are logged but not returned
// — certificate issuance is best-effort.
func (h *Handlers) maybeIssueCertificate(ctx context.Context, userID, moduleID pgtype.UUID) {
	// Resolve the module to find its track_id.
	mod, err := h.store.GetModuleByID(ctx, moduleID)
	if err != nil {
		return
	}

	completed, total, err := h.store.GetUserTrackProgress(ctx, userID, mod.TrackID)
	if err != nil || total == 0 || completed < total {
		return
	}

	// Generate a unique certificate number. Use the UUID's hex string
	// representation (not raw bytes) — raw UUID bytes contain invalid UTF-8
	// sequences that PostgreSQL's text column rejects (SQLSTATE 22021).
	certNumber := fmt.Sprintf("JC-%d-%s", time.Now().Unix(), hex.EncodeToString(userID.Bytes[:8]))

	if _, err := h.store.IssueCertificate(ctx, userID, mod.TrackID, certNumber); err != nil {
		slog.Error("issue certificate", "user_id", userID, "track_id", mod.TrackID, "error", err)
	}
}

func calculatePercent(completed, total int) float64 {
	if total == 0 {
		return 0
	}

	return float64(completed) / float64(total) * 100
}

// MaxPageSize — parsePageSize extracts the page_size query param, clamped to [1, MaxPageSize].
const MaxPageSize = 100

func parsePageSize(r *http.Request) int32 {
	n, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if n < 1 {
		n = 20
	}

	if n > MaxPageSize {
		n = MaxPageSize
	}

	return int32(n) // #nosec G109 G115 -- bounded by MaxPageSize check above
}

func parsePageSizeOffset(r *http.Request) int32 {
	n, _ := strconv.Atoi(r.URL.Query().Get("page_offset"))
	if n < 0 {
		n = 0
	}

	return int32(n) // #nosec G109 G115 -- bounded by non-negative check above
}

// ─── Admin Learning Extras (P5.6 — Task 14) ──────────────────────────────

// ListTrackEnrollments handles GET /admin/learn/tracks/:id/enrollments.
func (h *Handlers) ListTrackEnrollments(w http.ResponseWriter, r *http.Request) {

	var trackID pgtype.UUID
	if err := trackID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid track ID")
		return
	}

	limit, offset := int32(20), int32(0)

	if v := r.URL.Query().Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n) // #nosec G109 G115 -- bounded by n <= 100 check above
		}
	}

	if v := r.URL.Query().Get("page_offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = int32(n) // #nosec G109 G115 -- bounded by n >= 0 check above
		}
	}

	enrollments, err := h.store.ListEnrollmentsByTrack(r.Context(), trackID, limit, offset)
	if err != nil {
		slog.Error("learning: list track enrollments", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list enrollments")

		return
	}

	h.resolveLiterals(r, &enrollments)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": enrollments})
}

// ListAllModules handles GET /admin/learn/modules.
func (h *Handlers) ListAllModules(w http.ResponseWriter, r *http.Request) {

	modules, err := h.store.ListAllModules(r.Context())
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to list modules")
		return
	}

	h.resolveLiterals(r, &modules)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": modules})
}

// ListAllLessons handles GET /admin/learn/lessons.
func (h *Handlers) ListAllLessons(w http.ResponseWriter, r *http.Request) {

	lessons, err := h.store.ListAllLessons(r.Context())
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to list lessons")
		return
	}

	h.resolveLiterals(r, &lessons)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": lessons})
}

// ListModulesByTrack handles GET /admin/learn/tracks/{trackId}/modules.
func (h *Handlers) ListModulesByTrack(w http.ResponseWriter, r *http.Request) {

	trackIDStr := chi.URLParam(r, "trackId")

	var trackID pgtype.UUID

	if err := trackID.Scan(trackIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid track ID")
		return
	}

	modules, err := h.store.ListModulesByTrack(r.Context(), trackID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to list modules")
		return
	}

	h.resolveLiterals(r, &modules)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": modules})
}

// PublishTrack handles POST /admin/learn/tracks/:id/publish.
func (h *Handlers) PublishTrack(w http.ResponseWriter, r *http.Request) {

	var trackID pgtype.UUID
	if err := trackID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid track ID")
		return
	}

	if err := h.store.PublishTrack(r.Context(), trackID, true); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to publish track")
		return
	}

	// RAG indexing — fire-and-forget (§69/ADR-069 BUG-001 fix: published
	// learning tracks are part of the RAG corpus). Fetch the track text
	// from the store for embedding.
	if track, err := h.store.GetTrackByID(r.Context(), trackID); err == nil && track != nil {
		h.indexTrackRAG(r.Context(), trackID, track.Title, track.Description, track.Language)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": trackID.String(), "published": true}})
}

// ---------------------------------------------------------------------------
// Learning track reviews (AUDIT-BE-ROUTE-040)
// Reuse Section 29 reviews with entity_type=learning_track.
// ---------------------------------------------------------------------------

const learningTrackEntityType = "learning_track"

// CreateTrackReview handles POST /api/v1/learn/tracks/{id}/reviews.
// Only enrolled users can review a track. The review is created via the
// polymorphic reviews table with entity_type=learning_track.
func (h *Handlers) CreateTrackReview(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var trackID pgtype.UUID
	if err := trackID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid track ID")
		return
	}

	var input struct {
		Rating int16  `json:"rating"`
		Title  string `json:"title"`
		Body   string `json:"body"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Rating < 1 || input.Rating > 5 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_rating", "Rating must be between 1 and 5")
		return
	}

	var title *string
	if input.Title != "" {
		title = &input.Title
	}

	var body *string
	if input.Body != "" {
		body = &input.Body
	}

	review, err := h.store.CreateTrackReview(r.Context(), trackID, user.UserID, input.Rating, title, body)
	if err != nil {
		handleReviewError(w, err)
		return
	}

	h.resolveLiterals(r, review)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": review})
}

// ListTrackReviews handles GET /api/v1/learn/tracks/{id}/reviews.
// Public — no auth required. Returns reviews + aggregate rating.
func (h *Handlers) ListTrackReviews(w http.ResponseWriter, r *http.Request) {

	var trackID pgtype.UUID
	if err := trackID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid track ID")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	reviews, err := h.store.ListTrackReviews(r.Context(), trackID, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to list reviews")
		return
	}

	h.resolveLiterals(r, &reviews)

	aggregate, err := h.store.GetTrackReviewAggregate(r.Context(), trackID)
	if err != nil {
		// Aggregate may not exist yet — default to zeros.
		aggregate = TrackReviewAggregate{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":      reviews,
		"aggregate": aggregate,
	})
}

// handleReviewError maps a CreateTrackReview store error to the correct HTTP
// status code. Constraint violations become 409 (duplicate) or 403 (FK = not
// enrolled / track doesn't exist); everything else is a 500.
func handleReviewError(w http.ResponseWriter, err error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation — user already reviewed this track
			errpkg.WriteError(w, http.StatusConflict, "duplicate_review", "You have already reviewed this track")
			return
		case "23503": // foreign_key_violation — track or user doesn't exist
			errpkg.WriteError(w, http.StatusForbidden, "not_enrolled", "You must be enrolled to review this track")
			return
		}
	}

	if errors.Is(err, pgx.ErrNoRows) {
		errpkg.WriteError(w, http.StatusNotFound, "track_not_found", "Track not found")
		return
	}

	slog.Error("learning: create track review", "error", err)
	errpkg.WriteError(w, http.StatusInternalServerError, "review_error", "An unexpected error occurred")
}
