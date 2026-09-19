// Package handlers implements the 12 HTTP handlers for the Intelligence
// System API surface (/api/v1/intel/*). The handlers are thin: read
// operations call the intelligence stores directly, while write operations
// (follow/unfollow, save/unsave) delegate to the existing follows and
// scholarships services so the radar reuses the established polymorphic
// follow table and saved-scholarship tracker rather than introducing
// parallel tables.
// // Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/follows"
	"github.com/jedafe/japa/internal/intelligence/sources"
	"github.com/jedafe/japa/internal/intelligence/stores"
	"github.com/jedafe/japa/internal/reports"
	"github.com/jedafe/japa/internal/scholarships"
	"github.com/jedafe/japa/internal/tags"
)

// TaskIntelPromote is the Asynq task name the review-approve handler
// enqueues to promote an approved record (spec §647, route catalog §2237).
// Mirrors worker.TypeIntelPromote without importing the worker package
// (which would create a cycle).
const TaskIntelPromote = "intel:promote"

// TaskIntelFetchSource is the Asynq task name the crawl-now handler
// enqueues to trigger an immediate fetch of a source (INTEL-011 fix).
// Mirrors worker.TypeIntelFetchSource without importing the worker package.
const TaskIntelFetchSource = "intel:fetch_source"

// TaskReportNotifyAdmin is the Asynq task name the source-report handler
// enqueues so moderators with reports.review are notified of a new report —
// the same fan-out the content/comment/user report handlers run (Section 51).
// Mirrors worker.TypeReportNotifyAdmin without importing the worker package.
const TaskReportNotifyAdmin = "report:notify_admin"

// SettingsReader is the minimal interface the handlers need to read
// system_settings at runtime (Rule 3, INTEL-016). Defining it locally
// avoids an import cycle on internal/config.
type SettingsReader interface {
	GetIntOr(ctx context.Context, key string, fallback int) int
}

// videoStore is the small surface of the video store needed for source item
// feeds that include linked video sources (v2 source directory, Task 3).
type videoStore interface {
	ListBySourceID(ctx context.Context, sourceID pgtype.UUID, limit, offset int32) ([]gen.VideoSource, int64, error)
	// ListItemsBySourceID returns the same rows enriched with the author's
	// username and the published content slug, so item cards link to the
	// in-app video page (/video/{slug} or /video/{id}) instead of the raw
	// external video URL (§106).
	ListItemsBySourceID(ctx context.Context, sourceID pgtype.UUID, limit, offset int32) ([]gen.ListVideoSourceItemsBySourceIDRow, int64, error)
	// GetChannelUsernameBySourceID returns the channel account username
	// behind a video_channel source (used to link the claim form, §106).
	GetChannelUsernameBySourceID(ctx context.Context, sourceID pgtype.UUID) (string, error)
}

// contentStore is the small surface of the content service needed for source
// item feeds that include linked comparison contents.
type contentStore interface {
	ListComparisonContentsBySourceID(ctx context.Context, sourceID pgtype.UUID, limit, offset int32) ([]gen.ListComparisonContentsBySourceIDRow, int64, error)
}

// reportService creates a user report for an entity (content, source, etc).
// *reports.Service satisfies this interface.
type reportService interface {
	CreateReport(ctx context.Context, reporterUserID pgtype.UUID, input reports.CreateReportInput) (gen.ContentReport, error)
}

// tagService resolves and creates directory tags from source keywords so
// keyword chips can link to /tags/{slug} without 404ing.
type tagService interface {
	ResolveTagSlugs(ctx context.Context, slugs []string) ([]gen.GetTagsBySlugsRow, error)
	CreateTag(ctx context.Context, input tags.CreateTagInput) (gen.CreateTagRow, error)
}

// Handlers exposes the /api/v1/intel/* HTTP routes.
type Handlers struct {
	intelStores  *stores.Store
	intelSources *sources.Service
	followsSvc   *follows.Service
	scholarSvc   *scholarships.Service
	asynqClient  *asynq.Client  // enqueues intel:promote on review approve (BUG-003)
	settings     SettingsReader // system_settings reader (INTEL-016); nil → hardcoded defaults
	videos       videoStore     // optional; enables video source item feeds
	contents     contentStore   // optional; enables comparison source item feeds
	reportSvc    reportService  // optional; creates source reports from the detail page
	tagSvc       tagService     // optional; ensures source keyword tags exist
}

// NewHandlers creates intelligence handlers. The followsSvc and scholarSvc
// are required for the radar follow/unfollow and save/unsave routes; the
// radar feed, counts, and source directory read directly from the
// intelligence stores. asynqClient is used by HandleReviewApprove to enqueue
// intel:promote (spec §647); a nil client degrades to a logged warning
// (BUG-003 fix).
func NewHandlers(
	intelStores *stores.Store,
	intelSources *sources.Service,
	followsSvc *follows.Service,
	scholarSvc *scholarships.Service,
	asynqClient *asynq.Client,
	settings SettingsReader,
) *Handlers {
	return &Handlers{
		intelStores:  intelStores,
		intelSources: intelSources,
		followsSvc:   followsSvc,
		scholarSvc:   scholarSvc,
		asynqClient:  asynqClient,
		settings:     settings,
	}
}

// enqueueIntelPromote enqueues the intel:promote task for the given record
// (fire-and-forget per Rule 8). BUG-003 fix: the spec §647 requires approve
// to enqueue promotion; without it approved records are never published.
// INTEL-026 fix: the reviewer user ID is passed in the payload so the
// promoter can stamp hitl_reviewed_by on the contents row.
// SetVideoStore links the source directory item feed to the video store so
// linked video sources can be surfaced as source items (v2 Task 3).
func (h *Handlers) SetVideoStore(v videoStore) { h.videos = v }

// SetContentStore links the source directory item feed to the content service
// so linked comparison contents can be surfaced as source items.
func (h *Handlers) SetContentStore(c contentStore) { h.contents = c }

// SetReportService wires the report service so source detail can accept
// user reports.
func (h *Handlers) SetReportService(rs reportService) { h.reportSvc = rs }

// SetTagService wires the tag service so source keywords can be auto-created
// as directory tags and linked from the source header.
func (h *Handlers) SetTagService(ts tagService) { h.tagSvc = ts }

func (h *Handlers) enqueueIntelPromote(ctx context.Context, recordID, reviewerUserID pgtype.UUID) error {
	if h.asynqClient == nil {
		return errors.New("intel: asynq client not configured")
	}

	payload, err := json.Marshal(map[string]string{
		"record_id": recordID.String(),

		"reviewer_user_id": reviewerUserID.String(),
	})
	if err != nil {
		return fmt.Errorf("intel: marshal promote payload: %w", err)
	}

	if _, err := h.asynqClient.EnqueueContext(ctx, asynq.NewTask(TaskIntelPromote, payload)); err != nil {
		return fmt.Errorf("intel: enqueue promote: %w", err)
	}

	return nil
}

// --- shared helpers ---

func requireUser(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	u := auth.FromContext(r.Context())
	if u == nil || !u.UserID.Valid {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return pgtype.UUID{}, false
	}

	return u.UserID, true
}

func optionalUser(_ http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	u := auth.FromContext(r.Context())
	if u == nil || !u.UserID.Valid {
		return pgtype.UUID{}, false
	}

	return u.UserID, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, param string) (pgtype.UUID, bool) {
	raw := chi.URLParam(r, param)
	if raw == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_param", param+" is required")
		return pgtype.UUID{}, false
	}

	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_uuid", "Invalid "+param)
		return pgtype.UUID{}, false
	}

	return id, true
}

// parsePagination reads the offset-pagination convention params
// (`page_size` + `page_offset`) per conventions/api-response-format.md, with
// back-compat for the legacy `limit` + `page` (1-based) pair. Returns a
// 1-based page and a clamped page size so callers can compute
// `offset = (page-1) * limit` unchanged.
// // INTEL-016 fix: default and max page sizes are read from system_settings
// (intel.radar.default_page_size, intel.radar.max_page_size) when the
// handler has a settings reader; otherwise falls back to 20/100.
func (h *Handlers) parsePagination(r *http.Request) (int, int) {
	defaultSize := 20
	maxSize := 100

	if h.settings != nil {
		defaultSize = h.settings.GetIntOr(r.Context(), "intel.radar.default_page_size", 20)
		maxSize = h.settings.GetIntOr(r.Context(), "intel.radar.max_page_size", 100)
	}

	q := r.URL.Query()
	pageSize, _ := strconv.Atoi(q.Get("page_size"))

	if pageSize <= 0 {
		pageSize, _ = strconv.Atoi(q.Get("limit"))
	}

	if pageSize <= 0 || pageSize > maxSize {
		pageSize = defaultSize
	}

	pageOffset, _ := strconv.Atoi(q.Get("page_offset"))
	if pageOffset < 0 {
		page, _ := strconv.Atoi(q.Get("page"))
		if page <= 0 {
			page = 1
		}

		pageOffset = (page - 1) * pageSize
	}

	page := (pageOffset / pageSize) + 1

	return page, pageSize
}

// clampInt32 safely converts an int to int32, clamping to math.MaxInt32
// to prevent integer overflow (gosec G115). All pagination values from
// parsePagination are already bounded (pageSize <= 100, page >= 1), but
// the product (page-1)*pageSize can still exceed int32 on pathological
// inputs — clamp to be safe.
func clampInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}

	if v < 0 {
		return 0
	}

	return int32(v)
}
