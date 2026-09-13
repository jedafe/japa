package video

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
	// maxMultipartMemory is the in-memory budget for parsing a multipart upload.
	// Files larger than this are spooled to temporary disk files by net/http.
	maxMultipartMemory = 32 << 20
	// hardMaxUploadBytes is the absolute cap on a single video upload, regardless
	// of the tenant-configured media upload limit, to prevent memory exhaustion.
	hardMaxUploadBytes = 1 << 30
)

// Handlers exposes HTTP endpoints for the video import subsystem.
type Handlers struct {
	Service *Service
	authMW  func(http.Handler) http.Handler
}

// NewHandlers returns video HTTP handlers.
func NewHandlers(svc *Service) *Handlers {
	return &Handlers{Service: svc}
}

// SetAuthMiddleware sets the authentication middleware for routes that require
// an authenticated user.
func (h *Handlers) SetAuthMiddleware(mw func(http.Handler) http.Handler) {
	h.authMW = mw
}

// RegisterRoutes wires the video routes into the router. Callers should call
// this inside an /api/v1 route group.
func (h *Handlers) RegisterRoutes(r chi.Router) {
	h.postAuth(r, "/videos/resolve", h.Resolve)
	h.postAuth(r, "/videos/upload", h.Upload)
	r.Get("/videos/{id}", h.GetByID)
	h.postAuth(r, "/videos/{id}/summarize", h.Summarize)
	r.Get("/u/{username}/videos", h.ListChannelVideos)
	h.postAuth(r, "/video-channels/{username}/claims", h.SubmitChannelClaim)
	h.getAuth(r, "/me/video-claims", h.ListMyVideoClaims)
	h.postAuth(r, "/me/video-imports/preview", h.PreviewBulkImport)
	h.postAuth(r, "/me/video-imports", h.CreateImportJob)
	h.getAuth(r, "/me/video-imports", h.ListImportJobs)
	h.getAuth(r, "/me/video-imports/{id}", h.GetImportJob)
	h.postAuth(r, "/me/video-imports/{id}/retry", h.RetryMyImportJob)
}

func (h *Handlers) postAuth(r chi.Router, pattern string, fn http.HandlerFunc) {
	if h.authMW != nil {
		r.With(h.authMW).Post(pattern, fn)
		return
	}

	r.Post(pattern, fn)
}

func (h *Handlers) getAuth(r chi.Router, pattern string, fn http.HandlerFunc) {
	if h.authMW != nil {
		r.With(h.authMW).Get(pattern, fn)
		return
	}

	r.Get(pattern, fn)
}

// Resolve handles POST /api/v1/videos/resolve.
func (h *Handlers) Resolve(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		URL string `json:"url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.URL == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_url", "URL is required")
		return
	}

	result, err := h.Service.ResolveURL(r.Context(), tenantID, input.URL)
	if err != nil {
		if isVideoClientError(err) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_url", err.Error())
			return
		}

		errpkg.HandleServiceError(w, err)

		return
	}

	_ = user // authenticated user is the actor, but author is the synthetic channel

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// uploadFormHeadroom is the extra body allowance on top of the configured
// media upload cap, covering multipart framing plus the title/description
// form fields.
const uploadFormHeadroom int64 = 1 << 20

// Upload handles POST /api/v1/videos/upload — a direct multipart upload for
// source_type='upload' videos. The request is multipart/form-data with a
// `video` file field and optional `title`/`description` text fields.
// The uploader is the real (non-synthetic) author of the video source.
func (h *Handlers) Upload(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	maxBytes := h.Service.MaxUploadBytes(r.Context(), tenantID)
	if maxBytes <= 0 || maxBytes > hardMaxUploadBytes {
		maxBytes = hardMaxUploadBytes
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+uploadFormHeadroom)

	// #nosec G120 — maxMultipartMemory is a package constant (32 MiB) that caps
	// in-memory form parsing; the request body itself is bounded by MaxBytesReader.
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_form", "Invalid multipart form or file too large")
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_file", "A 'video' file field is required")
		return
	}

	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(file)
	if err != nil {
		errpkg.WriteError(w, http.StatusRequestEntityTooLarge, "file_too_large", "File exceeds the configured upload size limit")
		return
	}

	if len(data) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "empty_file", "The uploaded file is empty")
		return
	}

	if int64(len(data)) > maxBytes {
		errpkg.WriteError(w, http.StatusRequestEntityTooLarge, "file_too_large", "File exceeds the configured upload size limit")
		return
	}

	// Sniff the MIME type from content, not the client-supplied
	// Content-Type header — only real video files may become video sources.
	contentType := http.DetectContentType(data)
	if !strings.HasPrefix(contentType, "video/") {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_file_type", "The uploaded file is not a supported video")
		return
	}

	result, err := h.Service.UploadVideo(
		r.Context(),
		tenantID,
		user.UserID,
		header.Filename,
		data,
		strings.TrimSpace(r.FormValue("title")),
		strings.TrimSpace(r.FormValue("description")),
	)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// GetByID handles GET /api/v1/videos/{id} — public read of a single video
// source in the tenant so the video content single page can render the
// embed. Non-existent IDs return 404.
func (h *Handlers) GetByID(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil || !id.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid video source ID")
		return
	}

	source, err := h.Service.GetSourceByID(r.Context(), id)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	if source.Status == "removed" {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Video source has been removed")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": source})
}

// Summarize handles POST /api/v1/videos/{id}/summarize. It enqueues an
// asynchronous video:generate_summary Asynq task that uses the AI Gateway to
// draft a title/body for the video and store it as an in_review contents row.
// Requires authentication. Gated by the video.ai_summary.enabled setting.
func (h *Handlers) Summarize(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil || !id.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid video source ID")
		return
	}

	if err := h.Service.EnqueueGenerateSummary(r.Context(), tenantID, user.UserID, id); err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]any{
			"status": "queued",
		},
	})
}

// ListChannelVideos handles GET /api/v1/u/{username}/videos.
func (h *Handlers) ListChannelVideos(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	username := chi.URLParam(r, "username")
	if username == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_username", "Username is required")
		return
	}

	limit, offset := parsePage(r, "limit", "offset")

	sources, err := h.Service.ListChannelVideosByUsername(r.Context(), tenantID, username, limit, offset)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": sources})
}

// CreateImportJob handles POST /api/v1/me/video-imports.
func (h *Handlers) CreateImportJob(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		SourceType     string  `json:"source_type"`
		SourceURL      string  `json:"source_url"`
		Query          string  `json:"query"`
		TargetUsername *string `json:"target_username"`
		AutoPublish    bool    `json:"auto_publish"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.SourceType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_source_type", "source_type is required")
		return
	}

	job, err := h.Service.StartBulkImport(r.Context(), tenantID, user.UserID, BulkImportRequest{
		SourceType:     input.SourceType,
		SourceURL:      input.SourceURL,
		Query:          input.Query,
		TargetUsername: input.TargetUsername,
		AutoPublish:    input.AutoPublish,
	})
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": job})
}

// GetImportJob handles GET /api/v1/me/video-imports/{id}.
func (h *Handlers) GetImportJob(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	idParam := chi.URLParam(r, "id")

	var id pgtype.UUID

	if err := id.Scan(idParam); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid import job ID")
		return
	}

	job, err := h.Service.GetImportJob(r.Context(), tenantID, user.UserID, id)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": job})
}

// userListQueryFunc loads a paginated list scoped to the authenticated user.
type userListQueryFunc func(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) (any, error)

// runUserList is the shared boilerplate for user-scoped list endpoints.
func (h *Handlers) runUserList(w http.ResponseWriter, r *http.Request, fetch userListQueryFunc) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	limit, offset := parsePage(r, "limit", "offset")

	items, err := fetch(r.Context(), tenantID, user.UserID, limit, offset)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// ListImportJobs handles GET /api/v1/me/video-imports.
func (h *Handlers) ListImportJobs(w http.ResponseWriter, r *http.Request) {
	h.runUserList(w, r, func(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) (any, error) {
		return h.Service.ListImportJobsByUser(ctx, tenantID, userID, limit, offset)
	})
}

// SubmitChannelClaim handles POST /api/v1/video-channels/{username}/claims.
// Authenticated users can file a channel ownership claim or a takedown request.
func (h *Handlers) SubmitChannelClaim(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	username := chi.URLParam(r, "username")
	if username == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_username", "Username is required")
		return
	}

	var input struct {
		ClaimType     string  `json:"claim_type"`
		EvidenceURL   *string `json:"evidence_url"`
		Notes         *string `json:"notes"`
		VideoSourceID *string `json:"video_source_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.ClaimType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_claim_type", "claim_type is required")
		return
	}

	evidenceURL := ""
	if input.EvidenceURL != nil {
		evidenceURL = *input.EvidenceURL
	}

	notes := ""
	if input.Notes != nil {
		notes = *input.Notes
	}

	videoSourceID := ""
	if input.VideoSourceID != nil {
		videoSourceID = *input.VideoSourceID
	}

	claim, err := h.Service.SubmitClaim(r.Context(), tenantID, user.UserID, username, input.ClaimType, evidenceURL, notes, videoSourceID)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": claim})
}

// PreviewBulkImport handles POST /api/v1/me/video-imports/preview.
func (h *Handlers) PreviewBulkImport(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var req BulkImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	preview, err := h.Service.PreviewBulkImport(r.Context(), tenantID, user.UserID, req)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": preview})
}

// RetryMyImportJob handles POST /api/v1/me/video-imports/{id}/retry.
func (h *Handlers) RetryMyImportJob(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil || !id.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid import job ID")
		return
	}

	job, err := h.Service.RetryMyImportJob(r.Context(), tenantID, user.UserID, id)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": job})
}

// ListMyVideoClaims handles GET /api/v1/me/video-claims.
func (h *Handlers) ListMyVideoClaims(w http.ResponseWriter, r *http.Request) {
	h.runUserList(w, r, func(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) (any, error) {
		return h.Service.ListClaimsByUser(ctx, tenantID, userID, limit, offset)
	})
}

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

func isVideoClientError(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, ErrUnsupportedPlatform) ||
		errors.Is(err, ErrInvalidVideoURL) ||
		errors.Is(err, ErrNoYouTubeID)
}
