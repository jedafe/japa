package video

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

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
