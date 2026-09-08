package video

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
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
	r.Get("/u/{username}/videos", h.ListChannelVideos)
	h.postAuth(r, "/me/video-imports", h.CreateImportJob)
	h.getAuth(r, "/me/video-imports", h.ListImportJobs)
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
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+uploadFormHeadroom)

	if err := r.ParseMultipartForm(maxBytes + uploadFormHeadroom); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_form", "Invalid multipart form or file too large")
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_file", "A 'video' file field is required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		errpkg.WriteError(w, http.StatusRequestEntityTooLarge, "file_too_large", "File exceeds the configured upload size limit")
		return
	}
	if len(data) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "empty_file", "The uploaded file is empty")
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
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil || !id.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid video source ID")
		return
	}

	source, err := h.Service.GetSourceByID(r.Context(), tenantID, id)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": source})
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

	limit, offset := parsePage(r)

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
		SourceType string  `json:"source_type"`
		SourceURL  *string `json:"source_url"`
		Query      *string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.SourceType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_source_type", "source_type is required")
		return
	}

	job, err := h.Service.CreateImportJob(r.Context(), gen.CreateVideoImportJobParams{
		TenantID:   tenantID,
		UserID:     user.UserID,
		SourceType: input.SourceType,
		SourceUrl:  input.SourceURL,
		Query:      input.Query,
	})
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": job})
}

// ListImportJobs handles GET /api/v1/me/video-imports.
func (h *Handlers) ListImportJobs(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	limit, offset := parsePage(r)

	jobs, err := h.Service.ListImportJobsByUser(r.Context(), tenantID, user.UserID, limit, offset)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": jobs})
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

func parsePage(r *http.Request) (limit, offset int32) {
	limit = defaultPageSize
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.ParseInt(l, 10, 32); err == nil && n > 0 {
			if n > maxPageSize {
				n = maxPageSize
			}
			limit = int32(n)
		}
	}

	if o := r.URL.Query().Get("offset"); o != "" {
		if n, err := strconv.ParseInt(o, 10, 32); err == nil && n >= 0 {
			offset = int32(n)
		}
	}
	return
}

func isVideoClientError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrUnsupportedPlatform) ||
		errors.Is(err, ErrInvalidVideoURL) ||
		errors.Is(err, ErrNoYouTubeID)
}
