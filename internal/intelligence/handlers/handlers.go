// Package handlers implements the 12 HTTP handlers for the Intelligence
// System API surface (/api/v1/intel/*). The handlers are thin: read
// operations call the intelligence stores directly, while write operations
// (follow/unfollow, save/unsave) delegate to the existing follows and
// scholarships services so the radar reuses the established polymorphic
// follow table and saved-scholarship tracker rather than introducing
// parallel tables.
//
// Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/follows"
	"github.com/jedafe/japa/internal/intelligence/sources"
	"github.com/jedafe/japa/internal/intelligence/stores"
	"github.com/jedafe/japa/internal/scholarships"
	"github.com/jedafe/japa/internal/tenant"
)

// TaskIntelPromote is the Asynq task name the review-approve handler
// enqueues to promote an approved record (spec §647, route catalog §2237).
// Mirrors worker.TypeIntelPromote without importing the worker package
// (which would create a cycle).
const TaskIntelPromote = "intel:promote"

// Handlers exposes the /api/v1/intel/* HTTP routes.
type Handlers struct {
	intelStores   *stores.Store
	intelSources  *sources.Service
	followsSvc    *follows.Service
	scholarSvc    *scholarships.Service
	asynqClient   *asynq.Client // enqueues intel:promote on review approve (BUG-003)
}

// defaultIntelTenantID is the system tenant UUID used as a fallback for
// public routes that do not have an authenticated tenant in the context.
// Per ADR-112 Amendment 2, all intelligence data is tenant-scoped; in a
// single-tenant deployment all data belongs to the default tenant
// 00000000-0000-0000-0000-000000000001.
var defaultIntelTenantID = func() pgtype.UUID {
	var id pgtype.UUID
	id.Bytes[15] = 1 // 00000000-0000-0000-0000-000000000001
	id.Valid = true
	return id
}()

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
) *Handlers {
	return &Handlers{
		intelStores:  intelStores,
		intelSources: intelSources,
		followsSvc:   followsSvc,
		scholarSvc:   scholarSvc,
		asynqClient:  asynqClient,
	}
}

// enqueueIntelPromote enqueues the intel:promote task for the given record
// (fire-and-forget per Rule 8). BUG-003 fix: the spec §647 requires approve
// to enqueue promotion; without it approved records are never published.
func (h *Handlers) enqueueIntelPromote(ctx context.Context, tenantID, recordID pgtype.UUID) error {
	if h.asynqClient == nil {
		return fmt.Errorf("intel: asynq client not configured")
	}
	payload, err := json.Marshal(map[string]string{
		"record_id": recordID.String(),
		"tenant_id": tenantID.String(),
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

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]interface{}{
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
			"status":  status,
		},
	})
}

func requireUser(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	u := auth.FromContext(r.Context())
	if u == nil || !u.UserID.Valid {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return pgtype.UUID{}, false
	}
	return u.UserID, true
}

func optionalUser(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	u := auth.FromContext(r.Context())
	if u == nil || !u.UserID.Valid {
		return pgtype.UUID{}, false
	}
	return u.UserID, true
}

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	id := tenant.IDFromContext(r.Context())
	if !id.Valid {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return id, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, param string) (pgtype.UUID, bool) {
	raw := chi.URLParam(r, param)
	if raw == "" {
		writeError(w, http.StatusBadRequest, "missing_param", param+" is required")
		return pgtype.UUID{}, false
	}
	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_uuid", "Invalid "+param)
		return pgtype.UUID{}, false
	}
	return id, true
}

// parsePagination reads the offset-pagination convention params
// (`page_size` + `page_offset`) per conventions/api-response-format.md, with
// back-compat for the legacy `limit` + `page` (1-based) pair. Returns a
// 1-based page and a clamped page size so callers can compute
// `offset = (page-1) * limit` unchanged.
func parsePagination(r *http.Request) (int, int) {
	q := r.URL.Query()
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize <= 0 {
		pageSize, _ = strconv.Atoi(q.Get("limit"))
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
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
