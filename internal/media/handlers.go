package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/worker"
)

// MediaResponse is the API response shape for media items, matching frontend expectations.
// //nolint:revive // exported: stutter naming is intentional
type MediaResponse struct {
	ID           string   `json:"id"`
	Filename     string   `json:"filename"`
	MimeType     string   `json:"mime_type"`
	FileSize     int64    `json:"file_size"`
	AltText      *string  `json:"alt_text"`
	Caption      *string  `json:"caption"`
	Width        *int32   `json:"width"`
	Height       *int32   `json:"height"`
	DurationSecs *float64 `json:"duration_secs,omitempty"`
	Status       string   `json:"status"`
	Flagged      bool     `json:"flagged"`
	CreatedAt    string   `json:"created_at"`
	URL          string   `json:"url"`
	UserGroupID  *string  `json:"user_group_id"`
	// Stock attribution (present when media was imported from a stock provider).
	SourcePlatform        *string `json:"source_platform,omitempty"`
	SourcePhotographer    *string `json:"source_photographer,omitempty"`
	SourcePhotographerURL *string `json:"source_photographer_url,omitempty"`
	SourceLicense         *string `json:"source_license,omitempty"`
	SourceLicenseURL      *string `json:"source_license_url,omitempty"`
}

// toMediaResponse transforms a gen.Medium into the API response format.
func toMediaResponse(m *gen.Medium, url string) MediaResponse {
	status := "pending"
	if m.Confirmed {
		status = "approved"
	} else if m.QuarantineReason != nil && *m.QuarantineReason != "" {
		status = "quarantined"
	} else if m.RejectedAt.Valid {
		status = "rejected"
	}

	var userGroupID *string

	if m.UserGroupID.Valid {
		s := m.UserGroupID.String()
		userGroupID = &s
	}

	// DurationSecs is a pgtype.Numeric — extract the float64 value when valid.
	var duration *float64

	if m.DurationSecs.Valid {
		if f, err := m.DurationSecs.Float64Value(); err == nil && f.Valid {
			v := f.Float64
			duration = &v
		}
	}

	return MediaResponse{
		ID:                    m.ID.String(),
		Filename:              m.OriginalFilename,
		MimeType:              m.MimeType,
		FileSize:              m.FileSizeBytes,
		AltText:               m.AltText,
		Caption:               m.Caption,
		Width:                 m.WidthPx,
		Height:                m.HeightPx,
		DurationSecs:          duration,
		Status:                status,
		Flagged:               m.Flagged,
		CreatedAt:             m.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		URL:                   url,
		UserGroupID:           userGroupID,
		SourcePlatform:        m.SourcePlatform,
		SourcePhotographer:    m.SourcePhotographer,
		SourcePhotographerURL: m.SourcePhotographerUrl,
		SourceLicense:         m.SourceLicense,
		SourceLicenseURL:      m.SourceLicenseUrl,
	}
}

// toMediaResponses transforms a slice of gen.Medium into API response format.
// BUG-008: resolves the imgproxy config (key/salt/baseURL) once for the
// entire batch instead of re-reading env vars and re-decoding hex on every
// item. The HMAC itself is still computed per-item (unavoidable — each URL
// has a unique signature), but the config resolution overhead is amortized.
func toMediaResponses(items []gen.Medium, svc *Service) []MediaResponse {
	result := make([]MediaResponse, len(items))

	cfg := loadMediaURLConfig()
	for i := range items {
		url := ServeURL(items[i].StorageBucket, items[i].StoragePath)
		if isProxyableImageMIME(items[i].MimeType) {
			url = mediaURLWithConfig(items[i].StorageBucket, items[i].StoragePath, ImageProxyOpts{}, cfg)
		}
		result[i] = toMediaResponse(&items[i], url)
	}

	return result
}

// toQuarantinedMediaResponse transforms a QuarantinedMedia into the API response format.
func toQuarantinedMediaResponse(m *QuarantinedMedia, url string) MediaResponse {
	status := "pending"
	if m.Confirmed {
		status = "approved"
	} else if m.QuarantineReason != nil && *m.QuarantineReason != "" {
		status = "quarantined"
	}

	return MediaResponse{
		ID:        m.ID.String(),
		Filename:  m.OriginalFilename,
		MimeType:  m.MimeType,
		FileSize:  m.FileSizeBytes,
		AltText:   m.AltText,
		Caption:   m.Caption,
		Width:     m.WidthPx,
		Height:    m.HeightPx,
		Status:    status,
		CreatedAt: m.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		URL:       url,
	}
}

// toQuarantinedMediaResponses transforms a slice of QuarantinedMedia into API response format.
func toQuarantinedMediaResponses(items []QuarantinedMedia, svc *Service) []MediaResponse {
	result := make([]MediaResponse, len(items))
	for i := range items {
		result[i] = toQuarantinedMediaResponse(&items[i], svc.GetMediaURL(&gen.Medium{
			ID:            items[i].ID,
			StoragePath:   items[i].StoragePath,
			StorageBucket: items[i].StorageBucket,
			MimeType:      items[i].MimeType,
		}, ImageProxyOpts{}))
	}

	return result
}

// parseMediaID extracts and validates a media UUID from the :id URL parameter.
func parseMediaID(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_media_id", "Invalid media ID format")
		return pgtype.UUID{}, false
	}

	return id, true
}

// requireMediaAuth extracts user, platform, and media ID from the request.
// Returns (userID, ID, mediaID, ok). If ok is false, an error has been written.
func (h *Handlers) requireMediaAuth(w http.ResponseWriter, r *http.Request) (pgtype.UUID, pgtype.UUID, pgtype.UUID, bool) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, false
	}

	mediaID, ok := parseMediaID(w, r)
	if !ok {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, false
	}

	return user.UserID, pgtype.UUID{}, mediaID, true
}

const defaultPageSize int32 = 20

// Maximum request body sizes.
const (
	// MaxMediaUploadBody is the maximum body size for media upload initiation (50MB).
	MaxMediaUploadBody int64 = 50 << 20
	// MaxStandardBody is the maximum body size for standard POST/PATCH requests (10MB).
	MaxStandardBody int64 = 10 << 20
)

// MaxBulkDeleteIDs caps bulk-delete batches so a single request cannot fan
// out into an unbounded number of row writes.
const MaxBulkDeleteIDs = 100

// permissionChecker is the minimal RBAC interface media handlers need to
// check additional permission nodes inside handler bodies (e.g.
// media.video.upload for video MIME types). This avoids importing the full
// rbac package and keeps the handler testable with a stub.
type permissionChecker interface {
	HasPermission(ctx context.Context, userID pgtype.UUID, permKey string) bool
}

// Handlers holds dependencies for media HTTP handlers.
type Handlers struct {
	service     *Service
	AsynqClient *asynq.Client
	// perms is optional — when set, InitiateUpload checks media.video.upload
	// for video MIME types per Section 34.
	perms permissionChecker
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

// SetPermissionChecker injects the RBAC resolver for in-handler permission
// checks (media.video.upload). Called once during server wiring.
func (h *Handlers) SetPermissionChecker(pc permissionChecker) {
	h.perms = pc
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
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

	// #nosec G109 // value bounded by MaxPageSize check
	return int32(n)
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

	// #nosec G109 // value bounded by MaxPageSize check
	// #nosec G115 // value bounded by caller
	return int32(n)
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

	var input struct {
		Filename string `json:"filename"`
		MimeType string `json:"mime_type"`
		FileSize int64  `json:"file_size"`
		Context  string `json:"context"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		// MaxBytesError is returned when the body exceeds the limit.
		maxBytesError := &http.MaxBytesError{}
		if errors.As(err, &maxBytesError) {
			errpkg.WriteError(w, http.StatusRequestEntityTooLarge, "body_too_large", "Request body exceeds 50MB limit")
			return
		}

		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")

		return
	}

	if input.Filename == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_filename", "filename is required")
		return
	}

	if input.MimeType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_mime_type", "mime_type is required")
		return
	}

	if input.FileSize <= 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_file_size", "file_size must be positive")
		return
	}

	// Upload context selects the allowed MIME set (§34). Defaults to "media";
	// "document" permits CV-style attachments used by job applications (§63).
	uploadCtx := UploadContextMedia
	if input.Context != "" {
		uploadCtx = UploadContext(input.Context)
		if MIMETypeForContext(uploadCtx) == nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_upload_context",
				"Unknown upload context '"+input.Context+"'")
			return
		}
	}

	// MIME type validation: ensure declared type is in the allowed set.
	// Uses canonical comparison so alias types (image/jpg, image/pjpeg)
	// are accepted. The actual content inspection via http.DetectContentType
	// happens at ConfirmUpload time when the file bytes are available.
	if !IsAllowedMIMETypeForContext(input.MimeType, uploadCtx) {
		errpkg.WriteError(w, http.StatusBadRequest, "unsupported_mime_type",
			"MIME type '"+input.MimeType+"' is not allowed for media uploads")
		return
	}

	// Cross-check: if the filename extension suggests a different type than
	// declared, flag it. This catches obvious mismatches like declaring
	// "image/png" for a file named "virus.exe".
	if extMIME := MIMETypeFromExtension(input.Filename); extMIME != "" {
		if CanonicalMIMEType(extMIME) != CanonicalMIMEType(input.MimeType) {
			errpkg.WriteError(w, http.StatusBadRequest, "mime_extension_mismatch",
				"Declared MIME type '"+input.MimeType+"' does not match file extension '"+filepath.Ext(input.Filename)+"'")
			return
		}
	}

	// Video uploads require the additional media.video.upload permission
	// node (Section 34 — "MP4 and GIF uploads gated by media.video.upload").
	// The route middleware already checks media.upload; this is a secondary
	// gate for video content specifically (MEDIA-007).
	if isVideoMIME(CanonicalMIMEType(input.MimeType)) && h.perms != nil {
		if !h.perms.HasPermission(r.Context(), user.UserID, "media.video.upload") {
			errpkg.WriteError(w, http.StatusForbidden, "video_upload_forbidden",
				"You do not have permission to upload video files")
			return
		}
	}

	result, err := h.service.InitiateUpload(r.Context(), user.UserID, input.Filename, input.MimeType, input.FileSize, uploadCtx)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "upload_error", err.Error())
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// ConfirmUpload handles POST /api/v1/media/confirm/{id}.
// Response: { "data": { "confirmed": true } }.
func (h *Handlers) ConfirmUpload(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	mediaID, ok := parseMediaID(w, r)
	if !ok {
		return
	}

	// BUG-004: pass the uploader ID so ConfirmUpload can verify only the
	// user who initiated the upload can confirm it (§34 two-step upload).
	if err := h.service.ConfirmUpload(r.Context(), mediaID, user.UserID); err != nil {
		switch {
		case errors.Is(err, ErrMediaNotFound):
			// Covers wrong-owner too — same response either way so media
			// existence never leaks across ownership boundaries.
			errpkg.WriteError(w, http.StatusNotFound, "media_not_found", "Media not found")
		case errors.Is(err, errObjectMissing):
			errpkg.WriteError(w, http.StatusBadRequest, "upload_incomplete", "Upload did not complete — no stored object found")
		case errors.Is(err, errUploadInvalid):
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_upload", "Uploaded object failed validation")
		case errors.Is(err, errStorageUnavailable):
			errpkg.WriteError(w, http.StatusServiceUnavailable, "storage_unavailable", "Storage temporarily unavailable — retry the confirmation")
		default:
			errpkg.WriteError(w, http.StatusInternalServerError, "confirm_error", "Failed to confirm upload")
		}
		return
	}

	// Enqueue background processing tasks — fire-and-forget.
	h.enqueueMediaTasks(r.Context(), mediaID)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"confirmed": true}})
}

// enqueueMediaTasks enqueues image/video processing and virus scan tasks
// based on the media's MIME type. Fire-and-forget: errors are logged only.
func (h *Handlers) enqueueMediaTasks(ctx context.Context, mediaID pgtype.UUID) {
	if h.AsynqClient == nil {
		return
	}

	// Fetch media record to get MIME type for routing.
	m, err := h.service.GetMedia(ctx, mediaID)
	if err != nil {
		slog.Warn("media: failed to fetch media for task enqueue", "media_id", mediaID.String(), "error", err)
		return
	}

	mediaIDStr := mediaID.String()

	// Always enqueue virus scan for all confirmed uploads.
	scanPayload, _ := json.Marshal(map[string]string{
		"media_id": mediaIDStr,
	})
	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeMediaScan, scanPayload),
		asynq.Queue(worker.QueueForTask(worker.TypeMediaScan)),
	); err != nil {
		slog.Warn("media: failed to enqueue scan task", "media_id", mediaIDStr, "error", err)
	}

	// Route by MIME type for processing.
	mimeType := m.MimeType

	switch {
	case strings.HasPrefix(mimeType, "image/"):
		processPayload, _ := json.Marshal(map[string]string{
			"media_id": mediaIDStr,

			"operation": "optimize",
		})
		if _, err := h.AsynqClient.Enqueue(
			asynq.NewTask(worker.TypeImageProcessing, processPayload),
			asynq.Queue(worker.QueueForTask(worker.TypeImageProcessing)),
		); err != nil {
			slog.Warn("media: failed to enqueue image processing", "media_id", mediaIDStr, "error", err)
		}
	case strings.HasPrefix(CanonicalMIMEType(mimeType), "video/"):
		processPayload, _ := json.Marshal(map[string]string{
			"media_id": mediaIDStr,

			"operation": "transcode",
		})
		if _, err := h.AsynqClient.Enqueue(
			asynq.NewTask(worker.TypeVideoProcessing, processPayload),
			asynq.Queue(worker.QueueForTask(worker.TypeVideoProcessing)),
		); err != nil {
			slog.Warn("media: failed to enqueue video processing", "media_id", mediaIDStr, "error", err)
		}
	}
}

// isProxyableImageMIME reports whether imgproxy can transform the type:
// any image/* except the still-image ISO-BMFF types (HEIC/HEIF), which
// need libheif that many imgproxy builds lack — those get the raw
// ServeURL so at least Safari can render them natively.
func isProxyableImageMIME(mimeType string) bool {
	switch CanonicalMIMEType(mimeType) {
	case "image/heic", "image/heif", "image/heic-sequence", "image/heif-sequence":
		return false
	}

	return strings.HasPrefix(CanonicalMIMEType(mimeType), "image/")
}

// isVideoMIME reports whether the type is any video/* — the
// media.video.upload permission gate and task routing must cover every
// video container, not just the ones in the upload allowlist.
func isVideoMIME(mimeType string) bool {
	return strings.HasPrefix(mimeType, "video/")
}

// GetMedia handles GET /api/v1/media/{id}.
// Response: { "data": { ... } }.
func (h *Handlers) GetMedia(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	idStr := chi.URLParam(r, "id")

	var mediaID pgtype.UUID

	if err := mediaID.Scan(idStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_media_id", "Invalid media ID format")
		return
	}

	m, err := h.service.GetMedia(r.Context(), mediaID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "media_not_found", "Media not found")
		return
	}

	// BUG-006: the route is gated on media.library.view_own, which implies
	// own-library scope. Enforce that the caller is the uploader unless they
	// hold media.view_all (admin/moderator). Return 404 (not 403) so
	// existence is not leaked to unauthorized users, consistent with the
	// store's ownership-scoped methods.
	if m.UploaderUserID != user.UserID {
		canViewAll := h.perms != nil &&
			h.perms.HasPermission(r.Context(), user.UserID, "media.view_all")
		if !canViewAll {
			errpkg.WriteError(w, http.StatusNotFound, "media_not_found", "Media not found")
			return
		}
	}

	// Generate imgproxy URL on serialization (never cached).
	url := h.service.GetMediaURL(&m, ImageProxyOpts{})
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": toMediaResponse(&m, url),
	})
}

// metadataInput is the decoded JSON body for UpdateMetadata and AdminUpdateMetadata.
type metadataInput struct {
	AltText     *string  `json:"alt_text"`
	Caption     *string  `json:"caption"`
	Width       *int32   `json:"width"`
	Height      *int32   `json:"height"`
	Duration    *float64 `json:"duration_secs"`
	UserGroupID *string  `json:"user_group_id"`
}

// updateMetadataHandler is a shared helper for PATCH /api/v1/media/{id} and
// PATCH /admin/media/{id}. Both parse the same input. The ownOnly flag
// selects between the ownership-scoped and admin-unscoped service methods.
func (h *Handlers) updateMetadataHandler(w http.ResponseWriter, r *http.Request, ownOnly bool) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxStandardBody)

	user, ok := h.requireUser(w, r)
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
			errpkg.WriteError(w, http.StatusRequestEntityTooLarge, "body_too_large", "Request body exceeds 10MB limit")
			return
		}

		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")

		return
	}

	var userGroupID pgtype.UUID
	if input.UserGroupID != nil && *input.UserGroupID != "" {
		if err := userGroupID.Scan(*input.UserGroupID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_group_id", "Invalid user_group_id format")
			return
		}
	}

	if ownOnly {
		// User-facing update — scoped to the uploader (MEDIA-003).
		if err := h.service.UpdateMetadataOwn(r.Context(), user.UserID, mediaID, input.AltText, input.Caption, input.Width, input.Height, input.Duration, userGroupID); err != nil {
			errpkg.WriteError(w, http.StatusNotFound, "media_not_found", "Media not found")
			return
		}
	} else {
		// Admin update — no ownership filter (gated by media.edit_any).
		if err := h.service.UpdateMetadata(r.Context(), mediaID, input.AltText, input.Caption, input.Width, input.Height, input.Duration, userGroupID); err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "update_error", "Failed to update media")
			return
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"updated": true}})
}

// UpdateMetadata handles PATCH /api/v1/media/{id}.
// User-facing update — scoped to the uploader via media.edit_own.
// Request body: { "alt_text": "...", "caption": "...", "width": 100, "height": 200 }
// Response: { "data": { "updated": true } }.
func (h *Handlers) UpdateMetadata(w http.ResponseWriter, r *http.Request) {
	h.updateMetadataHandler(w, r, true)
}

// DeleteMedia handles DELETE /api/v1/media/{id}.
// User-facing delete — scoped to the uploader via media.delete_own.
// The admin route (DELETE /admin/media/{id}) uses the unscoped DeleteMedia
// service method, gated by media.delete_any.
// Response: { "data": { "deleted": true } }.
func (h *Handlers) DeleteMedia(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	mediaID, ok := parseMediaID(w, r)
	if !ok {
		return
	}

	// Use the ownership-scoped delete so a user with media.delete_own can
	// only delete their own uploads (MEDIA-001).
	if err := h.service.DeleteMediaOwn(r.Context(), user.UserID, mediaID); err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "media_not_found", "Media not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"deleted": true}})
}

// AdminDeleteMedia handles DELETE /admin/media/{id}.
// Admin delete — no ownership filter, gated by media.delete_any.
// ?force=true permanently removes an already-soft-deleted row, gated by
// admin.hard_delete via rbac.RequireForceDelete.
func (h *Handlers) AdminDeleteMedia(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	mediaID, ok := parseMediaID(w, r)
	if !ok {
		return
	}

	if r.URL.Query().Get("force") == "true" {
		err := h.service.HardDeleteMedia(r.Context(), mediaID)
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Media not found")
			return
		}
		if err != nil {
			errpkg.HandleServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if err := h.service.DeleteMedia(r.Context(), mediaID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_error", "Failed to delete media")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"deleted": true}})
}

// ListMedia handles GET /api/v1/media.
// Query params: page_size, page_offset, type, q (search), uploader_id
// Response: { "data": [...] }.
func (h *Handlers) ListMedia(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	var uploaderID *pgtype.UUID

	if uidStr := r.URL.Query().Get("uploader_id"); uidStr != "" {
		var uid pgtype.UUID
		if err := uid.Scan(uidStr); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_uploader_id", "Invalid uploader_id format")
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

	var status *string
	if st := r.URL.Query().Get("status"); st != "" {
		status = &st
	}

	var flagged *string
	if fl := r.URL.Query().Get("flagged"); fl != "" {
		flagged = &fl
	}

	deleted := r.URL.Query().Get("deleted")
	if deleted != "" && deleted != "only" && deleted != "all" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_filter", "deleted must be 'only' or 'all'")
		return
	}

	items, total, err := h.service.ListMedia(r.Context(), uploaderID, mimeType, query, status, flagged, deleted, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "list_error", "Failed to load media")
		return
	}

	totalPages := int32(1)
	if limit > 0 {
		totalPages = (total + limit - 1) / limit
		if totalPages < 1 {
			totalPages = 1
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        toMediaResponses(items, h.service),
		"total":       total,
		"total_pages": totalPages,
	})
}

// ListMyMedia handles GET /api/v1/me/media — the caller's own uploaded media,
// reusing ListMedia's uploader_id filter with the authenticated user's ID.
func (h *Handlers) ListMyMedia(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
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

	var status *string
	if st := r.URL.Query().Get("status"); st != "" {
		status = &st
	}

	var flagged *string
	if fl := r.URL.Query().Get("flagged"); fl != "" {
		flagged = &fl
	}

	deleted := r.URL.Query().Get("deleted")
	if deleted != "" && deleted != "only" && deleted != "all" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_filter", "deleted must be 'only' or 'all'")
		return
	}

	items, total, err := h.service.ListMedia(r.Context(), &user.UserID, mimeType, query, status, flagged, deleted, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "list_error", "Failed to load media")
		return
	}

	totalPages := int32(1)
	if limit > 0 {
		totalPages = (total + limit - 1) / limit
		if totalPages < 1 {
			totalPages = 1
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        toMediaResponses(items, h.service),
		"total":       total,
		"total_pages": totalPages,
	})
}

// ListByUser handles GET /users/{id}/media — the public profile Media tab.
func (h *Handlers) ListByUser(w http.ResponseWriter, r *http.Request) {

	var uploaderID pgtype.UUID
	if err := uploaderID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID format")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	items, total, err := h.service.ListByUser(r.Context(), uploaderID, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "list_error", "Failed to load media")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": toMediaResponses(items, h.service),
		"meta": map[string]any{"total": total, "page_size": limit, "page_offset": offset},
	})
}

// ListByOrganization handles GET /organizations/{id}/media — the org profile
// Media tab. Non-active orgs are already 404'd by the requireActiveOrgByID gate.
func (h *Handlers) ListByOrganization(w http.ResponseWriter, r *http.Request) {

	var orgID pgtype.UUID
	if err := orgID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_org_id", "Invalid organization ID format")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	items, total, err := h.service.ListByOrganization(r.Context(), orgID, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "list_error", "Failed to load media")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": toMediaResponses(items, h.service),
		"meta": map[string]any{"total": total, "page_size": limit, "page_offset": offset},
	})
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

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	items, err := h.service.ListQuarantined(r.Context(), limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "list_quarantined_error", "Failed to load quarantined media")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": toQuarantinedMediaResponses(items, h.service)})
}

// ApproveQuarantined handles POST /admin/media/{id}/approve.
// Clears quarantine and confirms the media item as safe.
func (h *Handlers) ApproveQuarantined(w http.ResponseWriter, r *http.Request) {
	userID, _, mediaID, ok := h.requireMediaAuth(w, r)
	if !ok {
		return
	}

	if err := h.service.ApproveMedia(r.Context(), mediaID, userID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "approve_error", "Failed to approve media")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "approved"}})
}

// RejectQuarantined handles POST /admin/media/{id}/reject.
// Soft-deletes the quarantined media after admin review.
func (h *Handlers) RejectQuarantined(w http.ResponseWriter, r *http.Request) {
	userID, _, mediaID, ok := h.requireMediaAuth(w, r)
	if !ok {
		return
	}

	if err := h.service.RejectMedia(r.Context(), mediaID, userID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "reject_error", "Failed to reject media")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "rejected"}})
}

// AdminUpdateMetadata handles PATCH /admin/media/{id}.
// Admin-level metadata update that does not enforce ownership.
func (h *Handlers) AdminUpdateMetadata(w http.ResponseWriter, r *http.Request) {
	h.updateMetadataHandler(w, r, false)
}

// AdminUpdateMediaStatus handles POST /admin/media/{id}/status.
// Allows admin to change media status (approve/reject) for any item.
func (h *Handlers) AdminUpdateMediaStatus(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxStandardBody)

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	mediaID, ok := parseMediaID(w, r)
	if !ok {
		return
	}

	var input struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Status != "approved" && input.Status != "rejected" && input.Status != "quarantined" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", "Status must be approved, rejected, or quarantined")
		return
	}

	confirmed := input.Status == "approved"

	var quarantineReason *string

	if input.Status == "quarantined" {
		reason := "quarantined by admin"
		quarantineReason = &reason
	}

	if err := h.service.AdminUpdateStatus(r.Context(), mediaID, user.UserID, confirmed, quarantineReason); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "status_update_error", "Failed to update media status")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": input.Status}})
}

// AdminUpload handles POST /admin/media/upload — direct multipart upload for admin.
// Stores the file locally under data/media/{bucket}/{path} so ServeMedia can serve it.
func (h *Handlers) AdminUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxMediaUploadBody)

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	// #nosec G120 // total body size is bounded by MaxBytesReader above.
	if err := r.ParseMultipartForm(32 << 10); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_form", "Invalid multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_file", "file is required")
		return
	}
	defer file.Close()

	if header.Size > h.service.MaxUploadBytes(r.Context()) {
		errpkg.WriteError(w, http.StatusRequestEntityTooLarge, "file_too_large", "File exceeds the configured upload size limit")
		return
	}

	// Sniff MIME from content, not from client-supplied Content-Type header
	// (SEC-008). DetectStorageMIME reads the file's first bytes and falls
	// back to the filename extension only for formats the sniffer cannot
	// fingerprint (svg, containers) — consistent with ConfirmUpload and
	// StoreFromBytes. The media-context allowlist applies to admin uploads
	// exactly as it does to user uploads: an admin trimming
	// content.allowed_upload_types restricts this endpoint too.
	sniffBuf := make([]byte, 512)
	n, _ := io.ReadFull(file, sniffBuf)
	contentType := DetectStorageMIME(sniffBuf[:n], header.Filename)
	if !h.service.isAllowedMIME(r.Context(), contentType, UploadContextMedia) {
		errpkg.WriteError(w, http.StatusBadRequest, "unsupported_media_type", "File type is not allowed for upload")
		return
	}
	// Rewind and buffer the full file (bounded by MaxBytesReader above) so
	// oversized raster images can be compressed before storage.
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "seek_error", "Failed to read file")
		return
	}
	body, err := io.ReadAll(file)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "read_error", "Failed to read file")
		return
	}
	body, contentType = optimizeImage(body, contentType)

	ext := extensionFromMIME(contentType)
	if ext == ".bin" {
		// Sanitized filename extension fallback for formats the sniffer
		// cannot fingerprint (svg, containers) — same as StoreFromBytes.
		ext = strings.ToLower(filepath.Ext(header.Filename))
		// Sanitize ext to prevent path traversal in the storage path (SEC-012).
		ext = strings.TrimPrefix(ext, ".")
		ext = strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				return r
			}

			return -1 // drop dots, slashes, backslashes, etc.
		}, ext)

		if ext != "" {
			ext = "." + ext
		}
	}

	storagePath := fmt.Sprintf("%s/%s%s", StoragePathTemp,

		uuid.New().String(),
		ext)

	bucket := h.service.storageBucket(r.Context())

	localPath := filepath.Join(mediaRoot, bucket, storagePath)
	// #nosec G703 // path is scoped to mediaRoot/{bucket}; file extension is sanitized elsewhere.
	if err := os.MkdirAll(filepath.Dir(localPath), 0o750); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "dir_error", "Failed to create upload directory")
		return
	}

	dst, err := os.Create(localPath) // #nosec
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "file_error", "Failed to create file")
		return
	}
	defer dst.Close()

	if _, err := dst.Write(body); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "write_error", "Failed to write file")
		return
	}

	mediaID, err := h.service.DirectUpload(r.Context(), user.UserID, header.Filename, contentType, int64(len(body)), localPath, storagePath)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to save media record")
		return
	}

	h.enqueueMediaTasks(r.Context(), mediaID)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"media_id": mediaID.String(),
			"url":      "/api/v1/media/serve/" + bucket + "/" + storagePath,
		},
	})
}

// AdminReoptimize handles POST /admin/media/reoptimize — scans media rows
// holding optimizable raster content above OptimizeSkipBytes and enqueues
// content:process_image (operation "optimize") for each, so originals stored
// before storage optimization existed get re-encoded to WebP. Optional body:
// {"limit": N} (default 500, max 1000). Idempotent — already-optimized rows
// are skipped by the worker.
func (h *Handlers) AdminReoptimize(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxStandardBody)

	if _, ok := h.requireUser(w, r); !ok {
		return
	}
	if h.AsynqClient == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "queue_unavailable", "Background task queue is not configured")
		return
	}

	limit := int32(500)
	if r.ContentLength > 0 {
		var input struct {
			Limit int32 `json:"limit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
			return
		}
		if input.Limit > 0 {
			limit = input.Limit
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	rows, err := h.service.store.ListMediaForOptimization(r.Context(), gen.ListMediaForOptimizationParams{
		MinSizeBytes: OptimizeSkipBytes,
		RowLimit:     limit,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to list media for optimization")
		return
	}

	enqueued := 0
	for _, row := range rows {
		payload, _ := json.Marshal(map[string]string{
			"media_id":  row.ID.String(),
			"operation": "optimize",
		})
		if _, err := h.AsynqClient.Enqueue(
			asynq.NewTask(worker.TypeImageProcessing, payload),
			asynq.Queue(worker.QueueForTask(worker.TypeImageProcessing)),
		); err != nil {
			slog.Warn("media: failed to enqueue reoptimize task", "media_id", row.ID.String(), "error", err)
			continue
		}
		enqueued++
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"candidates": len(rows),
			"enqueued":   enqueued,
		},
	})
}

// BulkDelete handles POST /api/v1/media/bulk-delete.
// Request body: { "media_ids": ["...", "..."] }
// Response: { "data": { "deleted": 2 } }.
func (h *Handlers) BulkDelete(w http.ResponseWriter, r *http.Request) {
	// Enforce 10MB body limit on standard POST endpoints.
	r.Body = http.MaxBytesReader(w, r.Body, MaxStandardBody)

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		MediaIDs []string `json:"media_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		maxBytesError := &http.MaxBytesError{}
		if errors.As(err, &maxBytesError) {
			errpkg.WriteError(w, http.StatusRequestEntityTooLarge, "body_too_large", "Request body exceeds 10MB limit")
			return
		}

		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")

		return
	}

	if len(input.MediaIDs) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_media_ids", "media_ids is required")
		return
	}

	// Cap bulk batches so a single request cannot fan out into an unbounded
	// number of row writes.
	if len(input.MediaIDs) > MaxBulkDeleteIDs {
		errpkg.WriteError(w, http.StatusBadRequest, "too_many_media_ids", "media_ids is limited to 100 items per request")
		return
	}

	ids := make([]pgtype.UUID, 0, len(input.MediaIDs))

	for _, idStr := range input.MediaIDs {
		var id pgtype.UUID
		if err := id.Scan(idStr); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_media_id", "Invalid media ID format: "+idStr)
			return
		}

		ids = append(ids, id)
	}

	// Use the ownership-scoped bulk delete so a user with media.delete_own
	// can only delete their own uploads (MEDIA-002).
	if err := h.service.BulkDeleteOwn(r.Context(), user.UserID, ids); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "bulk_delete_error", "Failed to delete media")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]int{"deleted": len(ids)},
	})
}

// ServeMedia handles GET /api/v1/media/serve/{bucket}/{path}.
// Serves media files from the local filesystem for development environments
// where imgproxy and S3/SeaweedFS are not configured.
func (h *Handlers) ServeMedia(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	storagePath := chi.URLParam(r, "*") // catch-all for path

	if bucket == "" || storagePath == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_path", "Invalid media path")
		return
	}

	localPath, err := resolveMediaPath(bucket, storagePath)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_path", "Invalid media path")
		return
	}

	info, err := os.Stat(localPath)
	if err != nil || info.IsDir() {
		writePlaceholderSVG(w)
		return
	}

	ext := strings.ToLower(filepath.Ext(storagePath))

	contentType := mime.TypeByExtension(ext)
	if contentType == "" || contentType == "application/octet-stream" {
		// Older uploads were stored with a .bin extension before the
		// extension map covered video/audio — sniff the real type so
		// they serve (and play) correctly.
		if sniffed, sErr := sniffStoredContentType(localPath); sErr == nil && sniffed != "" {
			contentType = sniffed
		} else {
			contentType = "application/octet-stream"
		}
	}

	// Stored files are attacker-supplied: never let the browser execute them
	// as active content on this origin (SVG/HTML script payloads).
	if isActiveContentType(contentType) {
		contentType = "application/octet-stream"
	}

	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	ctype := contentType
	// Never serve SVG inline: browsers execute embedded script on the API
	// origin (stored XSS). Force a download with a sandboxed CSP.
	if CanonicalMIMEType(ctype) == "image/svg+xml" {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Content-Disposition", `attachment; filename="media.svg"`)
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
		http.ServeFile(w, r, localPath)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	http.ServeFile(w, r, localPath)
}

// sniffStoredContentType reads the first 512 bytes of a stored file and
// returns the sniffed MIME type. Used when the storage path extension does
// not map to a type (legacy .bin uploads).
func sniffStoredContentType(localPath string) (string, error) {
	f, err := os.Open(localPath) // #nosec G304 -- localPath is resolveMediaPath-validated (bucket allowlist + traversal guard) at every call site
	if err != nil {
		return "", err
	}
	defer f.Close()

	head := make([]byte, 512)
	n, err := f.Read(head)
	if err != nil && n == 0 {
		return "", err
	}

	return http.DetectContentType(head[:n]), nil
}

// mediaRoot is the base directory for locally stored media files. It is
// resolved to an absolute path at process start so the storage location is
// pinned to the directory that existed when the process launched — a
// CWD-relative constant would silently split the media store whenever the
// binary is started from a different working directory (objects written
// under one root become "not found" under another). Operators can override
// the root with JC_MEDIA_ROOT (see config.Config.MediaRoot).
var mediaRoot = func() string {
	abs, err := filepath.Abs("data/media")
	if err != nil {
		return "data/media"
	}
	return abs
}()

// SetMediaRoot pins the local media storage root to dir (JC_MEDIA_ROOT).
// Relative paths resolve against the startup working directory; absolute
// paths are used as-is. Called once from server wiring before any handler
// or Asynq task runs — no-op for empty input.
func SetMediaRoot(dir string) {
	if dir == "" {
		return
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		slog.Warn("media: ignoring invalid JC_MEDIA_ROOT", "dir", dir, "error", err)
		return
	}
	mediaRoot = abs
	slog.Info("media: local storage root configured", "dir", mediaRoot)
}

// resolveMediaPath maps a bucket and request path to a file inside mediaRoot.
// It rejects anything that escapes the bucket directory (path traversal) or
// that carries a path separator in the bucket name.
func resolveMediaPath(bucket, storagePath string) (string, error) {
	if strings.ContainsAny(bucket, `/\`) || bucket == "." || bucket == ".." {
		return "", errors.New("invalid bucket")
	}

	base, err := filepath.Abs(filepath.Join(mediaRoot, bucket))
	if err != nil {
		return "", err
	}

	full, err := filepath.Abs(filepath.Join(base, filepath.FromSlash(storagePath)))
	if err != nil {
		return "", err
	}

	if full != base && !strings.HasPrefix(full, base+string(os.PathSeparator)) {
		return "", errors.New("path escapes media root")
	}

	return full, nil
}

// activeContentTypes are MIME types a browser executes as active content.
// Serving them inline from the API origin would turn any uploaded file into
// stored XSS, so they are downgraded to an opaque download.
var activeContentTypes = map[string]bool{
	"image/svg+xml":          true,
	"text/html":              true,
	"text/xml":               true,
	"application/xml":        true,
	"application/xhtml+xml":  true,
	"application/javascript": true,
	"text/javascript":        true,
}

func isActiveContentType(contentType string) bool {
	base, _, _ := strings.Cut(contentType, ";")

	return activeContentTypes[strings.TrimSpace(strings.ToLower(base))]
}

// SeaweedFSUpload handles PUT /seaweedfs/upload/{bucket}/*.
// This is the dev-only fallback for presigned URL uploads when S3/SeaweedFS is
// not configured. The client PUTs raw file bytes to the presigned URL; this
// handler writes them to the local filesystem so ServeMedia can retrieve them.
//
// Auth note: the route is gated by auth.RequireAuth (server.go) — only an
// authenticated session holder reaches this handler. The PUT must correspond
// to a pending upload row created via InitiateUpload (which binds the owner
// and stores the claimed MIME/size): direct PUTs without a valid pending row
// are rejected with 404. Sniffed content is allowlist-gated exactly like
// InitiateUpload and a virus-scan task is enqueued for the pending row once
// bytes land; confirmation still runs the full verification in ConfirmUpload.
func (h *Handlers) SeaweedFSUpload(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	storagePath := chi.URLParam(r, "*") // catch-all

	if bucket == "" || storagePath == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_path", "Invalid upload path")
		return
	}

	localPath, err := resolveMediaPath(bucket, storagePath)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_path", "Invalid upload path")
		return
	}

	// Require the Initiate→Confirm flow: look up the pending row by storage
	// path so owner binding, claimed MIME/size, and expiry (presigned URL
	// ttl) are enforced by lookup. When the store is unavailable we fail
	// closed — without a row the write cannot be attributed to an owner.
	pending, err := h.service.store.GetMediaByStoragePath(r.Context(), storagePath)
	if err != nil || pending.UploaderUserID == (pgtype.UUID{}) {
		slog.Warn("seaweedfs upload: no pending upload row for path", "path", localPath)
		errpkg.WriteError(w, http.StatusNotFound, "upload_not_initiated", "No pending upload found for this path — initiate the upload first")
		return
	}

	// Owner binding: the authenticated caller must be the row's uploader.
	if user := auth.FromContext(r.Context()); user == nil || user.UserID != pending.UploaderUserID {
		slog.Warn("seaweedfs upload: owner mismatch", "path", localPath)
		errpkg.WriteError(w, http.StatusNotFound, "upload_not_initiated", "No pending upload found for this path — initiate the upload first")
		return
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil { // #nosec G301,G703 // bounded; pending path validation
		slog.Warn("seaweedfs upload: failed to create directory", "path", localPath, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "dir_error", "Failed to create upload directory")

		return
	}

	// Enforce 50MB body limit.
	r.Body = http.MaxBytesReader(w, r.Body, MaxFileSizeBytes)

	dst, err := os.Create(localPath) // #nosec
	if err != nil {
		slog.Warn("seaweedfs upload: failed to create file", "path", localPath, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "file_error", "Failed to create file")

		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, r.Body)
	if err != nil {
		slog.Warn("seaweedfs upload: failed to write file", "path", localPath, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "write_error", "Failed to write file")

		return
	}

	// Same MIME gates as InitiateUpload: sniff the stored bytes
	// (DetectStorageMIME — never a client header), run content inspection
	// (VerifyMIMEType against the row's declared type), and enforce the
	// media-context allowlist. A mismatch is removed without confirmation.
	if sniffErr := h.verifyPendingUploadBytes(localPath, pending.OriginalFilename, pending.MimeType); sniffErr != nil {
		slog.Warn("seaweedfs upload: content failed MIME gates", "path", localPath, "error", sniffErr)
		_ = os.Remove(localPath)
		errpkg.WriteError(w, http.StatusBadRequest, "unsupported_media_type", "Uploaded content failed file-type validation")
		return
	}

	// Enqueue the virus scan for the pending row now that bytes have landed
	// (ConfirmUpload enqueues again; the task is idempotent on media id).
	h.enqueueMediaTasks(r.Context(), pending.ID)

	slog.Info("seaweedfs upload: file stored locally", "path", localPath, "bytes", written)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// verifyPendingUploadBytes applies the InitiateUpload MIME gates to bytes
// already written by SeaweedFSUpload: DetectStorageMIME sniffing, the
// service allowlist for the media context, and VerifyMIMEType content
// inspection against the row's declared MIME.
func (h *Handlers) verifyPendingUploadBytes(localPath, filename, declaredMIME string) error {
	f, err := os.Open(localPath) // #nosec G304 -- path resolved via resolveMediaPath
	if err != nil {
		return err
	}
	defer f.Close()

	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	sniffed := DetectStorageMIME(head[:n], filename)
	if !h.service.isAllowedMIME(context.Background(), sniffed, UploadContextMedia) {
		return errors.New("media: sniffed type not allowed for upload")
	}
	if _, matches, verr := VerifyMIMEType(head[:n], declaredMIME); verr != nil || !matches {
		return errors.New("media: content does not match declared MIME type")
	}
	return nil
}

// ProxyRemoteMedia handles GET /api/v1/media/proxy?url={encoded}.
// Fetches a remote image same-origin so hosts that block cross-origin embeds
// (CORP/ORB) still render. Any failure (bad URL, SSRF-guarded target,
// non-image response, oversize body) falls back to the placeholder SVG with
// 200 so <img> consumers never see a console error. The route is gated by
// auth.RequireAuth plus a tight rate limit (server.go) — an anonymous caller
// cannot use the endpoint as an open fetch oracle.
// Ref: conventions/api-routes-catalog.md (media), sections/34 (media system).
func (h *Handlers) ProxyRemoteMedia(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		writePlaceholderSVG(w)
		return
	}

	res, err := h.service.ProxyRemote(r.Context(), rawURL)
	if err != nil {
		// Keep the placeholder-200 UX but surface abuse as an Info log with
		// the caller IP and a hash of the requested URL (never the raw URL,
		// which may carry credentials or session tokens in the query).
		sum := sha256.Sum256([]byte(rawURL))
		slog.Info("media proxy: fetch failed", "ip", clientip.FromRequest(r),
			"url_sha256", hex.EncodeToString(sum[:]), "error", err)
		writePlaceholderSVG(w)
		return
	}

	// Same hardening as ServeMedia — proxied bytes are attacker-influenced.
	// Proxied SVG is never served inline: force a download so embedded
	// script cannot run on this origin.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	if CanonicalMIMEType(res.ContentType) == "image/svg+xml" {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Content-Disposition", `attachment; filename="proxy.svg"`)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Set("Content-Length", strconv.Itoa(len(res.Body)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(res.Body)
		return
	}
	w.Header().Set("Content-Type", res.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Content-Length", strconv.Itoa(len(res.Body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(res.Body)
}

// writePlaceholderSVG returns a static placeholder image for missing media.
// The requested filename is deliberately not echoed back — reflecting it into
// an SVG served from this origin is an XSS sink.
func writePlaceholderSVG(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<svg width="200" height="200" xmlns="http://www.w3.org/2000/svg">
  <rect width="100%" height="100%" fill="#1a1a2e"/>
  <text x="50%" y="50%" font-family="sans-serif" font-size="14" fill="#888" text-anchor="middle" dy=".3em">Japa Connect</text>
</svg>`))
}
