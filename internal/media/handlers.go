package media

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/worker"
)

// parseMediaID extracts and validates a media UUID from the :id URL parameter.
func parseMediaID(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_media_id", "Invalid media ID format")
		return pgtype.UUID{}, false
	}

	return id, true
}

// requireMediaAuth extracts user, tenant, and media ID from the request.
// Returns (userID, tenantID, mediaID, ok). If ok is false, an error has been written.
func (h *Handlers) requireMediaAuth(w http.ResponseWriter, r *http.Request) (pgtype.UUID, pgtype.UUID, pgtype.UUID, bool) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, false
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, false
	}

	mediaID, ok := parseMediaID(w, r)
	if !ok {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, false
	}

	return user.UserID, tenantID, mediaID, true
}

const defaultPageSize int32 = 20

// Maximum request body sizes.
const (
	// MaxMediaUploadBody is the maximum body size for media upload initiation (50MB).
	MaxMediaUploadBody int64 = 50 << 20
	// MaxStandardBody is the maximum body size for standard POST/PATCH requests (10MB).
	MaxStandardBody int64 = 10 << 20
)

// Handlers holds dependencies for media HTTP handlers.
type Handlers struct {
	service    *Service
	AsynqClient *asynq.Client
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// writeJSON serializes the given value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

// writeError writes a JSON error response following the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > int(MaxPageSize) {
		return MaxPageSize
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

// parsePageOffset extracts the page_offset query parameter.
func parsePageOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		return 0
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

// ---------------------------------------------------------------------------
// Media Handlers
// ---------------------------------------------------------------------------

// InitiateUpload handles POST /api/v1/media/upload.
// Request body: { "filename": "photo.jpg", "mime_type": "image/jpeg", "file_size": 123456 }
// Response: { "data": { "media_id": "...", "storage_path": "...", "presigned_url": "...", "expires_at": "..." } }.
func (h *Handlers) InitiateUpload(w http.ResponseWriter, r *http.Request) {
	// Enforce 50MB body limit on media upload endpoints.
	r.Body = http.MaxBytesReader(w, r.Body, MaxMediaUploadBody)

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Filename string `json:"filename"`
		MimeType string `json:"mime_type"`
		FileSize int64  `json:"file_size"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		// MaxBytesError is returned when the body exceeds the limit.
		maxBytesError := &http.MaxBytesError{}
		if errors.As(err, &maxBytesError) {
			writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "Request body exceeds 50MB limit")
			return
		}

		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")

		return
	}

	if input.Filename == "" {
		writeError(w, http.StatusBadRequest, "missing_filename", "filename is required")
		return
	}

	if input.MimeType == "" {
		writeError(w, http.StatusBadRequest, "missing_mime_type", "mime_type is required")
		return
	}

	if input.FileSize <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_file_size", "file_size must be positive")
		return
	}

	// MIME type validation: ensure declared type is in the allowed set.
	// Uses canonical comparison so alias types (image/jpg, image/pjpeg)
	// are accepted. The actual content inspection via http.DetectContentType
	// happens at ConfirmUpload time when the file bytes are available.
	if !IsAllowedMIMETypeForContext(input.MimeType, UploadContextMedia) {
		writeError(w, http.StatusBadRequest, "unsupported_mime_type",
			"MIME type '"+input.MimeType+"' is not allowed for media uploads")
		return
	}

	// Cross-check: if the filename extension suggests a different type than
	// declared, flag it. This catches obvious mismatches like declaring
	// "image/png" for a file named "virus.exe".
	if extMIME := MIMETypeFromExtension(input.Filename); extMIME != "" {
		if CanonicalMIMEType(extMIME) != CanonicalMIMEType(input.MimeType) {
			writeError(w, http.StatusBadRequest, "mime_extension_mismatch",
				"Declared MIME type '"+input.MimeType+"' does not match file extension '"+filepath.Ext(input.Filename)+"'")
			return
		}
	}

	result, err := h.service.InitiateUpload(r.Context(), tenantID, user.UserID, input.Filename, input.MimeType, input.FileSize)
	if err != nil {
		writeError(w, http.StatusBadRequest, "upload_error", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// ConfirmUpload handles POST /api/v1/media/confirm/{id}.
// Response: { "data": { "confirmed": true } }.
func (h *Handlers) ConfirmUpload(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	mediaID, ok := parseMediaID(w, r)
	if !ok {
		return
	}

	if err := h.service.ConfirmUpload(r.Context(), tenantID, mediaID); err != nil {
		writeError(w, http.StatusInternalServerError, "confirm_error", err.Error())
		return
	}

	// Enqueue background processing tasks — fire-and-forget.
	h.enqueueMediaTasks(r.Context(), tenantID, mediaID)

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"confirmed": true}})
}

// enqueueMediaTasks enqueues image/video processing and virus scan tasks
// based on the media's MIME type. Fire-and-forget: errors are logged only.
func (h *Handlers) enqueueMediaTasks(ctx context.Context, tenantID, mediaID pgtype.UUID) {
	if h.AsynqClient == nil {
		return
	}

	// Fetch media record to get MIME type for routing.
	m, err := h.service.GetMedia(ctx, tenantID, mediaID)
	if err != nil {
		slog.Warn("media: failed to fetch media for task enqueue",
			"media_id", mediaID.String(), "error", err)
		return
	}

	mediaIDStr := mediaID.String()
	tenantIDStr := tenantID.String()

	// Always enqueue virus scan for all confirmed uploads.
	scanPayload, _ := json.Marshal(map[string]string{
		"media_id": mediaIDStr,
	})
	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeMediaScan, scanPayload),
		asynq.Queue(worker.QueueForTask(worker.TypeMediaScan)),
	); err != nil {
		slog.Warn("media: failed to enqueue scan task",
			"media_id", mediaIDStr, "error", err)
	}

	// Route by MIME type for processing.
	mimeType := m.MimeType
	switch {
	case isImageMIME(mimeType):
		processPayload, _ := json.Marshal(map[string]string{
			"media_id":   mediaIDStr,
			"tenant_id":  tenantIDStr,
			"operation":  "optimize",
		})
		if _, err := h.AsynqClient.Enqueue(
			asynq.NewTask(worker.TypeImageProcessing, processPayload),
			asynq.Queue(worker.QueueForTask(worker.TypeImageProcessing)),
		); err != nil {
			slog.Warn("media: failed to enqueue image processing",
				"media_id", mediaIDStr, "error", err)
		}
	case isVideoMIME(mimeType):
		processPayload, _ := json.Marshal(map[string]string{
			"media_id":   mediaIDStr,
			"tenant_id":  tenantIDStr,
			"operation":  "transcode",
		})
		if _, err := h.AsynqClient.Enqueue(
			asynq.NewTask(worker.TypeVideoProcessing, processPayload),
			asynq.Queue(worker.QueueForTask(worker.TypeVideoProcessing)),
		); err != nil {
			slog.Warn("media: failed to enqueue video processing",
				"media_id", mediaIDStr, "error", err)
		}
	}
}

func isImageMIME(mimeType string) bool {
	switch mimeType {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/svg+xml":
		return true
	}
	return false
}

func isVideoMIME(mimeType string) bool {
	switch mimeType {
	case "video/mp4", "video/webm", "video/quicktime", "video/x-msvideo":
		return true
	}
	return false
}

// GetMedia handles GET /api/v1/media/{id}.
// Response: { "data": { ... } }.
func (h *Handlers) GetMedia(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	idStr := chi.URLParam(r, "id")

	var mediaID pgtype.UUID

	if err := mediaID.Scan(idStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_media_id", "Invalid media ID format")
		return
	}

	m, err := h.service.GetMedia(r.Context(), tenantID, mediaID)
	if err != nil {
		writeError(w, http.StatusNotFound, "media_not_found", "Media not found")
		return
	}

	// Generate imgproxy URL on serialization (never cached).
	url := h.service.GetMediaURL(&m, ImageProxyOpts{})
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"media": m,
			"url":   url,
		},
	})
}

// metadataInput is the decoded JSON body for UpdateMetadata and AdminUpdateMetadata.
type metadataInput struct {
	AltText  *string  `json:"alt_text"`
	Caption  *string  `json:"caption"`
	Width    *int32   `json:"width"`
	Height   *int32   `json:"height"`
	Duration *float64 `json:"duration_secs"`
}

// updateMetadataHandler is a shared helper for PATCH /api/v1/media/{id} and
// PATCH /admin/media/{id}. Both parse the same input and call UpdateMetadata.
func (h *Handlers) updateMetadataHandler(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxStandardBody)

	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	mediaID, ok := parseMediaID(w, r)
	if !ok {
		return
	}

	var input metadataInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		maxBytesError := &http.MaxBytesError{}
		if errors.As(err, &maxBytesError) {
			writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "Request body exceeds 10MB limit")
			return
		}

		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")

		return
	}

	if err := h.service.UpdateMetadata(r.Context(), tenantID, mediaID, input.AltText, input.Caption, input.Width, input.Height, input.Duration); err != nil {
		writeError(w, http.StatusInternalServerError, "update_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"updated": true}})
}

// UpdateMetadata handles PATCH /api/v1/media/{id}.
// Request body: { "alt_text": "...", "caption": "...", "width": 100, "height": 200 }
// Response: { "data": { "updated": true } }.
func (h *Handlers) UpdateMetadata(w http.ResponseWriter, r *http.Request) {
	h.updateMetadataHandler(w, r)
}

// DeleteMedia handles DELETE /api/v1/media/{id}.
// Response: { "data": { "deleted": true } }.
func (h *Handlers) DeleteMedia(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	mediaID, ok := parseMediaID(w, r)
	if !ok {
		return
	}

	if err := h.service.DeleteMedia(r.Context(), tenantID, mediaID); err != nil {
		writeError(w, http.StatusInternalServerError, "delete_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"deleted": true}})
}

// ListMedia handles GET /api/v1/media.
// Query params: page_size, page_offset, type, q (search), uploader_id
// Response: { "data": [...] }.
func (h *Handlers) ListMedia(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	var uploaderID *pgtype.UUID

	if uidStr := r.URL.Query().Get("uploader_id"); uidStr != "" {
		var uid pgtype.UUID
		if err := uid.Scan(uidStr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_uploader_id", "Invalid uploader_id format")
			return
		}

		uploaderID = &uid
	}

	var mimeType *string
	if mt := r.URL.Query().Get("type"); mt != "" {
		mimeType = &mt
	}

	var query *string
	if q := r.URL.Query().Get("q"); q != "" {
		query = &q
	}

	items, err := h.service.ListMedia(r.Context(), tenantID, uploaderID, mimeType, query, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

// ListMyMedia handles GET /api/v1/me/media — the caller's own uploaded media,
// reusing ListMedia's uploader_id filter with the authenticated user's ID.
func (h *Handlers) ListMyMedia(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	var mimeType *string
	if mt := r.URL.Query().Get("type"); mt != "" {
		mimeType = &mt
	}

	var query *string
	if q := r.URL.Query().Get("q"); q != "" {
		query = &q
	}

	items, err := h.service.ListMedia(r.Context(), tenantID, &user.UserID, mimeType, query, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

// ---------------------------------------------------------------------------
// Admin Handlers — Media Quarantine
// ---------------------------------------------------------------------------

// ListQuarantined handles GET /admin/media/quarantined.
// Returns all media items currently in quarantine for admin review.
func (h *Handlers) ListQuarantined(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	items, err := h.service.ListQuarantined(r.Context(), tenantID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_quarantined_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

// ApproveQuarantined handles POST /admin/media/{id}/approve.
// Clears quarantine and confirms the media item as safe.
func (h *Handlers) ApproveQuarantined(w http.ResponseWriter, r *http.Request) {
	userID, tenantID, mediaID, ok := h.requireMediaAuth(w, r)
	if !ok {
		return
	}

	if err := h.service.ApproveMedia(r.Context(), mediaID, tenantID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "approve_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "approved"}})
}

// RejectQuarantined handles POST /admin/media/{id}/reject.
// Soft-deletes the quarantined media after admin review.
func (h *Handlers) RejectQuarantined(w http.ResponseWriter, r *http.Request) {
	userID, tenantID, mediaID, ok := h.requireMediaAuth(w, r)
	if !ok {
		return
	}

	if err := h.service.RejectMedia(r.Context(), mediaID, tenantID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "reject_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "rejected"}})
}

// AdminUpdateMetadata handles PATCH /admin/media/{id}.
// Admin-level metadata update that does not enforce ownership.
func (h *Handlers) AdminUpdateMetadata(w http.ResponseWriter, r *http.Request) {
	h.updateMetadataHandler(w, r)
}

// BulkDelete handles POST /api/v1/media/bulk-delete.
// Request body: { "media_ids": ["...", "..."] }
// Response: { "data": { "deleted": 2 } }.
func (h *Handlers) BulkDelete(w http.ResponseWriter, r *http.Request) {
	// Enforce 10MB body limit on standard POST endpoints.
	r.Body = http.MaxBytesReader(w, r.Body, MaxStandardBody)

	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		MediaIDs []string `json:"media_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		maxBytesError := &http.MaxBytesError{}
		if errors.As(err, &maxBytesError) {
			writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "Request body exceeds 10MB limit")
			return
		}

		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")

		return
	}

	if len(input.MediaIDs) == 0 {
		writeError(w, http.StatusBadRequest, "missing_media_ids", "media_ids is required")
		return
	}

	ids := make([]pgtype.UUID, 0, len(input.MediaIDs))

	for _, idStr := range input.MediaIDs {
		var id pgtype.UUID
		if err := id.Scan(idStr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_media_id", "Invalid media ID format: "+idStr)
			return
		}

		ids = append(ids, id)
	}

	if err := h.service.BulkDelete(r.Context(), tenantID, ids); err != nil {
		writeError(w, http.StatusInternalServerError, "bulk_delete_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]int{"deleted": len(ids)},
	})
}
