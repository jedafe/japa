package learning

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes HTTP endpoints for the learning platform. Each handler
// reads tenant context from the request, parses input, delegates to the
// store, and writes a JSON response. Guard clauses at the top of every
// handler keep the happy path un-indented.
type Handlers struct {
	store *Store
}

// NewHandlers creates a learning handler bound to the provided store.
func NewHandlers(store *Store) *Handlers {
	return &Handlers{store: store}
}

// ---------------------------------------------------------------------------
// Public / authenticated user endpoints
// ---------------------------------------------------------------------------

// ListPublishedTracks returns all published tracks for the current tenant.
func (h *Handlers) ListPublishedTracks(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	tracks, err := h.store.ListPublishedTracks(r.Context(), t.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to list tracks")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": tracks})
}

// GetTrackDetail returns a published track with its modules and lesson counts.
func (h *Handlers) GetTrackDetail(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Track slug is required")
		return
	}

	track, err := h.store.GetTrackBySlug(r.Context(), t.ID, slug)
	if err != nil {
		writeError(w, http.StatusNotFound, "track_not_found", "Track not found")
		return
	}

	modules, err := h.store.ListModulesByTrack(r.Context(), t.ID, track.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to list modules")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"track":   track,
			"modules": modules,
		},
	})
}

// GetLesson returns a single lesson by its slug within the track/module hierarchy.
func (h *Handlers) GetLesson(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}
	trackSlug := chi.URLParam(r, "trackSlug")
	moduleSlug := chi.URLParam(r, "moduleSlug")
	lessonSlug := chi.URLParam(r, "lessonSlug")
	if trackSlug == "" || moduleSlug == "" || lessonSlug == "" {
		writeError(w, http.StatusBadRequest, "missing_params", "Track, module, and lesson slugs are required")
		return
	}

	track, err := h.store.GetTrackBySlug(r.Context(), t.ID, trackSlug)
	if err != nil {
		writeError(w, http.StatusNotFound, "track_not_found", "Track not found")
		return
	}

	module, err := h.store.GetModuleBySlug(r.Context(), t.ID, track.ID, moduleSlug)
	if err != nil {
		writeError(w, http.StatusNotFound, "module_not_found", "Module not found")
		return
	}

	lesson, err := h.store.GetLessonBySlug(r.Context(), t.ID, module.ID, lessonSlug)
	if err != nil {
		writeError(w, http.StatusNotFound, "lesson_not_found", "Lesson not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": lesson})
}

// UpdateProgress records or updates a user's progress on a lesson.
func (h *Handlers) UpdateProgress(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	lessonIDStr := chi.URLParam(r, "id")
	var lessonID pgtype.UUID
	if err := lessonID.Scan(lessonIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid lesson ID")
		return
	}

	// Verify the lesson exists and belongs to this tenant.
	lesson, err := h.store.GetLessonByID(r.Context(), t.ID, lessonID)
	if err != nil {
		writeError(w, http.StatusNotFound, "lesson_not_found", "Lesson not found")
		return
	}
	_ = lesson // lesson validated; proceed with progress update

	var input UpdateProgressInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}
	if input.Status == "" {
		input.Status = "in_progress"
	}
	if input.Status != "not_started" && input.Status != "in_progress" && input.Status != "completed" {
		writeError(w, http.StatusBadRequest, "invalid_status", "Status must be not_started, in_progress, or completed")
		return
	}

	progress, err := h.store.UpsertProgress(r.Context(), t.ID, user.UserID, lessonID, input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to update progress")
		return
	}

	// Auto-issue certificate when all lessons in a track are completed.
	if input.Status == "completed" {
		h.maybeIssueCertificate(r.Context(), t.ID, user.UserID, lesson.ModuleID)
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": progress})
}

// GetMyCertificates returns all certificates earned by the authenticated user.
func (h *Handlers) GetMyCertificates(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	certs, err := h.store.ListCertificatesByUser(r.Context(), user.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to list certificates")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": certs})
}

// ---------------------------------------------------------------------------
// Admin endpoints (require learning.admin.manage permission)
// ---------------------------------------------------------------------------

// ListAllTracks returns every track for the tenant (admin view).
func (h *Handlers) ListAllTracks(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	tracks, err := h.store.ListAllTracks(r.Context(), t.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to list tracks")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": tracks})
}

// CreateTrack creates a new learning track.
func (h *Handlers) CreateTrack(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	var input CreateTrackInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}
	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}
	if input.Slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}
	if input.Difficulty == "" {
		input.Difficulty = "beginner"
	}

	track, err := h.store.CreateTrack(r.Context(), t.ID, input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to create track")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": track})
}

// UpdateTrack updates an existing track.
func (h *Handlers) UpdateTrack(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	idStr := chi.URLParam(r, "id")
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid track ID")
		return
	}

	var input UpdateTrackInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	track, err := h.store.UpdateTrack(r.Context(), t.ID, id, input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to update track")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": track})
}

// DeleteTrack removes a track and cascades to its modules and lessons.
func (h *Handlers) DeleteTrack(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	idStr := chi.URLParam(r, "id")
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid track ID")
		return
	}

	if err := h.store.DeleteTrack(r.Context(), t.ID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to delete track")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// CreateModule adds a module to a track.
func (h *Handlers) CreateModule(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	trackIDStr := chi.URLParam(r, "trackId")
	var trackID pgtype.UUID
	if err := trackID.Scan(trackIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid track ID")
		return
	}

	var input CreateModuleInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}
	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}
	if input.Slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	module, err := h.store.CreateModule(r.Context(), t.ID, trackID, input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to create module")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": module})
}

// UpdateModule updates an existing module.
func (h *Handlers) UpdateModule(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	idStr := chi.URLParam(r, "id")
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid module ID")
		return
	}

	var input UpdateModuleInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	module, err := h.store.UpdateModule(r.Context(), t.ID, id, input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to update module")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": module})
}

// DeleteModule removes a module and cascades to its lessons.
func (h *Handlers) DeleteModule(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	idStr := chi.URLParam(r, "id")
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid module ID")
		return
	}

	if err := h.store.DeleteModule(r.Context(), t.ID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to delete module")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// CreateLesson adds a lesson to a module.
func (h *Handlers) CreateLesson(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	moduleIDStr := chi.URLParam(r, "moduleId")
	var moduleID pgtype.UUID
	if err := moduleID.Scan(moduleIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid module ID")
		return
	}

	var input CreateLessonInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}
	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}
	if input.Slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}
	if input.LessonType == "" {
		input.LessonType = "article"
	}

	lesson, err := h.store.CreateLesson(r.Context(), t.ID, moduleID, input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to create lesson")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": lesson})
}

// UpdateLesson updates an existing lesson.
func (h *Handlers) UpdateLesson(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	idStr := chi.URLParam(r, "id")
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid lesson ID")
		return
	}

	var input UpdateLessonInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	lesson, err := h.store.UpdateLesson(r.Context(), t.ID, id, input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to update lesson")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": lesson})
}

// DeleteLesson removes a lesson.
func (h *Handlers) DeleteLesson(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	idStr := chi.URLParam(r, "id")
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid lesson ID")
		return
	}

	if err := h.store.DeleteLesson(r.Context(), t.ID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to delete lesson")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// GetTrackProgress returns the user's progress for a specific track.
func (h *Handlers) GetTrackProgress(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	trackIDStr := chi.URLParam(r, "trackId")
	var trackID pgtype.UUID
	if err := trackID.Scan(trackIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid track ID")
		return
	}

	completed, total, err := h.store.GetUserTrackProgress(r.Context(), user.UserID, trackID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to get progress")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"completed": completed,
			"total":     total,
			"percent":   calculatePercent(completed, total),
		},
	})
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// maybeIssueCertificate checks if the user has completed all lessons in a
// track and issues a certificate if so. Errors are logged but not returned
// — certificate issuance is best-effort.
func (h *Handlers) maybeIssueCertificate(ctx context.Context, tenantID, userID, moduleID pgtype.UUID) {
	// Resolve the module to find its track_id.
	mod, err := h.store.GetModuleByID(ctx, tenantID, moduleID)
	if err != nil {
		return
	}

	completed, total, err := h.store.GetUserTrackProgress(ctx, userID, mod.TrackID)
	if err != nil || total == 0 || completed < total {
		return
	}

	// Generate a unique certificate number.
	certNumber := fmt.Sprintf("JC-%d-%s", time.Now().Unix(), userID.Bytes[:8])

	_, _ = h.store.IssueCertificate(ctx, tenantID, userID, mod.TrackID, certNumber)
}

func calculatePercent(completed, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(completed) / float64(total) * 100
}

// writeJSON sends a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Log encoding errors — the header is already sent so we cannot
		// change the status code, but we should not silently swallow it.
		fmt.Printf("learning: encode response: %v\n", err)
	}
}

// writeError sends a structured JSON error response.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}
