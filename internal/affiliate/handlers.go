package affiliate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/worker"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// TaskEnqueueClient is the interface for enqueueing background tasks.
type TaskEnqueueClient interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// CacheClient is the subset of the Valkey (go-redis compatible) API used by
// the affiliate redirect handler for click dedup (jc:aff_dedup) and the
// attribution cookie (jc:aff_click). valkeycompat.Cmdable satisfies this.
type CacheClient interface {
	SetNX(ctx context.Context, key string, value any, expiration time.Duration) *valkeycompat.BoolCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *valkeycompat.StatusCmd
}

// Handlers provides HTTP handlers for the affiliate program.
type Handlers struct {
	Service     *Service
	AsynqClient TaskEnqueueClient
	Cache       CacheClient // jc:aff_dedup + jc:aff_click (Section 56)
}

// NewHandlers creates a Handlers backed by the given service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// enqueueRecordClick enqueues an async affiliate click recording task.
// Fire-and-forget: errors are logged but never propagated.
//
// The payload carries pre-hashed ip_hash and ua_hash (SHA-256 hex) so raw
// PII never transits Asynq/Valkey (Section 56: "never store raw IP for
// privacy"). The worker uses these directly — it does NOT re-hash.
func (h *Handlers) enqueueRecordClick(linkID pgtype.UUID, ipHash, uaHash, referrerURL string) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"link_id":      linkID.String(),
		"ip_hash":      ipHash,
		"ua_hash":      uaHash,
		"referrer_url": referrerURL,
	})
	if err != nil {
		slog.Warn("affiliate: failed to marshal record click payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeAffiliateRecordClick, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeAffiliateRecordClick)),
	); err != nil {
		slog.Warn("affiliate: failed to enqueue record click task",
			"link_id", linkID.String(), "error", err)
	}
}

// ── Helpers ─────────────────────────────────────────────────────────

func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return user, true
}

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "Tenant context not available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

func parseUUIDParam(r *http.Request, name string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		return pgtype.UUID{}, err
	}
	return id, nil
}

func parseQueryInt(r *http.Request, key string, defaultVal int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return defaultVal
	}
	return n
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		json.NewEncoder(w).Encode(v)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrMemberNotFound), errors.Is(err, ErrLinkNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrAlreadyEnrolled):
		writeError(w, http.StatusConflict, "already_enrolled", err.Error())
	case errors.Is(err, ErrNotEnrolled), errors.Is(err, ErrMemberSuspended):
		writeError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrTooManyLinks):
		writeError(w, http.StatusConflict, "too_many_links", err.Error())
	case errors.Is(err, ErrFeatureDisabled):
		writeError(w, http.StatusForbidden, "feature_disabled", err.Error())
	case errors.Is(err, ErrAccountTooNew):
		writeError(w, http.StatusForbidden, "account_too_new", err.Error())
	case errors.Is(err, ErrLinkNotActive):
		writeError(w, http.StatusNotFound, "link_not_active", err.Error())
	case errors.Is(err, ErrInvalidProductType):
		writeError(w, http.StatusBadRequest, "invalid_product_type", err.Error())
	case errors.Is(err, errpkg.ErrRequired):
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
	default:
		errpkg.HandleServiceError(w, err)
	}
}

// ── Program Info ────────────────────────────────────────────────────

// GetProgramInfo handles GET /api/v1/affiliate-program
func (h *Handlers) GetProgramInfo(w http.ResponseWriter, r *http.Request) {
	info := h.Service.GetProgramInfo(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"data": info})
}

// Enroll handles POST /api/v1/affiliate-program/enroll
func (h *Handlers) Enroll(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	member, err := h.Service.Enroll(r.Context(), EnrollInput{
		TenantID: tenantID,
		UserID:   user.UserID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": member})
}

// ── User Link Routes ────────────────────────────────────────────────

// ListMyLinks handles GET /api/v1/me/affiliate-links
func (h *Handlers) ListMyLinks(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	page := int32(parseQueryInt(r, "page", 1))
	pageSize := int32(parseQueryInt(r, "page_size", DefaultPageSize))

	links, err := h.Service.ListUserLinks(r.Context(), user.UserID, tenantID, page, pageSize)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": links})
}

// CreateMyLink handles POST /api/v1/me/affiliate-links
func (h *Handlers) CreateMyLink(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	var input CreateLinkInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if input.ProductType == "" {
		writeError(w, http.StatusBadRequest, "missing_field", "product_type is required")
		return
	}
	if input.DestinationURL == "" {
		writeError(w, http.StatusBadRequest, "missing_field", "destination_url is required")
		return
	}

	link, err := h.Service.CreateLink(r.Context(), tenantID, user.UserID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": link})
}

// GetMyLink handles GET /api/v1/me/affiliate-links/:id
func (h *Handlers) GetMyLink(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid link ID")
		return
	}
	link, err := h.Service.GetLink(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if link.UserID != user.UserID {
		writeError(w, http.StatusForbidden, "forbidden", "You can only view your own links")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": link})
}

// UpdateMyLink handles PATCH /api/v1/me/affiliate-links/:id
func (h *Handlers) UpdateMyLink(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid link ID")
		return
	}
	var input UpdateLinkInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	link, err := h.Service.UpdateLink(r.Context(), id, user.UserID, tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": link})
}

// DeleteMyLink handles DELETE /api/v1/me/affiliate-links/:id
func (h *Handlers) DeleteMyLink(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid link ID")
		return
	}
	if err := h.Service.DeleteLink(r.Context(), id, user.UserID, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// GetMyEarnings handles GET /api/v1/me/affiliate-earnings
func (h *Handlers) GetMyEarnings(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	earnings, err := h.Service.GetEarnings(r.Context(), user.UserID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": earnings})
}

// ── Redirect Handler ────────────────────────────────────────────────

// Redirect handles GET /r/:code — public, no auth required.
//
// Ref: japaconnect-engineering/sections/56-user-affiliate-program.md
// (Redirect Endpoint). Click dedup uses Valkey SET NX (jc:aff_dedup) and the
// attribution cookie is stored in jc:aff_click for the configured
// affiliate.attribution_window_days window. Click recording is fire-and-forget
// via the affiliate:record_click Asynq task (Rule 8) — never inline.
func (h *Handlers) Redirect(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	link, err := h.Service.GetLinkByCode(r.Context(), code)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	// Deduplicate clicks: same IP within 1 hour = same click.
	// Uses Valkey SET NX (not GET-then-SET) per Section 56 / Rule 9 spirit.
	ipHash := sha256Hex(clientip.FromRequest(r))

	// jc:aff_dedup:{link_id}:{ip_hash}:{hour_bucket} — 1h TTL.
	dedupKey := valkey.MustKey("aff_dedup:%s:%s:%s",
		link.ID.String(), ipHash, time.Now().Format("2006-01-02-15"))
	if h.Cache != nil {
		if set, derr := h.Cache.SetNX(r.Context(), dedupKey, "1", time.Hour).Result(); derr == nil && !set {
			// Duplicate click within the hour — still redirect, but do not
			// record or overwrite attribution.
			dest := sanitizeRedirectURL(link.DestinationUrl)
			http.Redirect(w, r, dest, http.StatusFound)
			return
		}
	}

	// Record click async (fire-and-forget) — affiliate:record_click. The
	// worker persists the event and increments counters. Never record inline
	// (BUG-001: previously both an inline goroutine and the Asynq task wrote,
	// double-counting every click).
	uaHash := sha256Hex(r.Header.Get("User-Agent"))
	referrer := r.Referer()
	h.enqueueRecordClick(link.ID, ipHash, uaHash, referrer)

	// Store attribution cookie in Valkey so a future purchase by this viewer
	// can be attributed to this affiliate link (Section 56 Attribution Model).
	// Only authenticated viewers can be attributed; anonymous visitors have no
	// stable id to key on in V1.
	if viewer := auth.FromContext(r.Context()); viewer != nil && viewer.UserID.Valid && h.Cache != nil {
		productID := "null"
		if link.ProductID.Valid {
			productID = link.ProductID.String()
		}
		attrKey := valkey.MustKey("aff_click:%s:%s:%s",
			viewer.UserID.String(), link.ProductType, productID)
		ttl := time.Duration(h.Service.AttributionWindowDays(r.Context())) * 24 * time.Hour
		if ttl <= 0 {
			ttl = 30 * 24 * time.Hour
		}
		if _, aerr := h.Cache.Set(r.Context(), attrKey, link.Code, ttl).Result(); aerr != nil {
			slog.Warn("affiliate: failed to set attribution key",
				"key", attrKey, "error", aerr)
		}
	}

	dest := sanitizeRedirectURL(link.DestinationUrl)
	http.Redirect(w, r, dest, http.StatusFound)
}

// sanitizeRedirectURL validates the destination scheme before redirecting to
// block javascript:/data: open-redirect vectors (SEC-004). Falls back to "/"
// for unsafe or unparseable destinations.
func sanitizeRedirectURL(dest string) string {
	if u, err := url.Parse(dest); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "/"
	}
	return dest
}

// ── Admin Routes ────────────────────────────────────────────────────

// ListMembersAdmin handles GET /api/v1/admin/affiliate-program/members
//
// Ref: japaconnect-engineering/sections/56-user-affiliate-program.md.
// Tenant-scoped, paginated, with an optional ?status filter (catalog). Uses
// the sqlc ListAffiliateMembersFiltered query (no raw SQL, no hardcoded
// limit). Money is rendered via pgtype.Numeric → string (never float).
func (h *Handlers) ListMembersAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	page := int32(parseQueryInt(r, "page", 1))
	pageSize := int32(parseQueryInt(r, "page_size", DefaultPageSize))
	limit, offset := paginate(page, pageSize)

	var statusFilter *string
	if s := r.URL.Query().Get("status"); s != "" {
		statusFilter = &s
	}

	members, err := h.Service.store.listMembersFiltered(r.Context(), tenantID, statusFilter, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query_error", "Failed to query members")
		return
	}
	results := make([]map[string]any, 0, len(members))
	for _, m := range members {
		username, _ := h.Service.lookupUsername(r.Context(), m.UserID, tenantID)
		results = append(results, map[string]any{
			"id": m.UserID.String(), "user_id": m.UserID.String(), "username": username,
			"status": m.Status, "total_clicks": m.TotalClicks,
			"total_conversions": m.TotalConversions,
			"total_earned_credits": formatNumeric(m.TotalEarnedCredits),
			"enrolled_at": m.EnrolledAt.Time,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": results})
}

// UpdateMemberStatusAdmin handles PATCH /api/v1/admin/affiliate-program/members/:user_id
func (h *Handlers) UpdateMemberStatusAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	userID, err := parseUUIDParam(r, "user_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}
	var input UpdateMemberStatusInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if input.Status == "" {
		writeError(w, http.StatusBadRequest, "missing_field", "status is required")
		return
	}

	member, err := h.Service.UpdateMemberStatus(r.Context(), userID, tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": member})
}

// ListAllLinksAdmin handles GET /api/v1/admin/affiliate-links
//
// Ref: japaconnect-engineering/sections/56-user-affiliate-program.md.
// Tenant-scoped, paginated, with optional ?user_id&product_type&status filters
// (catalog). Uses the sqlc ListAllAffiliateLinksFiltered query (joins users
// for the username column). Money is rendered via pgtype.Numeric → string.
func (h *Handlers) ListAllLinksAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	page := int32(parseQueryInt(r, "page", 1))
	pageSize := int32(parseQueryInt(r, "page_size", DefaultPageSize))
	limit, offset := paginate(page, pageSize)

	params := gen.ListAllAffiliateLinksFilteredParams{
		TenantID: tenantID, PageLimit: limit, PageOffset: offset,
	}
	if uid := r.URL.Query().Get("user_id"); uid != "" {
		var u pgtype.UUID
		if err := u.Scan(uid); err == nil {
			params.UserID = u
		}
	}
	if pt := r.URL.Query().Get("product_type"); pt != "" {
		params.ProductType = &pt
	}
	if st := r.URL.Query().Get("status"); st != "" {
		isActive := st == "active"
		params.IsActive = &isActive
	}

	rows, err := h.Service.store.listAllLinksFiltered(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query_error", "Failed to query links")
		return
	}
	results := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		results = append(results, map[string]any{
			"id": row.ID.String(), "user_id": row.UserID.String(), "username": row.Username,
			"product_type": row.ProductType, "code": row.Code, "description": row.Description,
			"destination_url": row.DestinationUrl, "click_count": row.ClickCount,
			"conversion_count": row.ConversionCount,
			"total_earned": formatNumeric(row.TotalEarned),
			"is_active": row.IsActive, "created_at": row.CreatedAt.Time,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": results})
}

// DeleteLinkAdmin handles DELETE /api/v1/admin/affiliate-links/:id
//
// Ref: japaconnect-engineering/sections/56-user-affiliate-program.md (admin
// management). Admin-scoped soft-delete (deactivate) — the row is retained for
// audit. Guarded by affiliate.admin.manage.
func (h *Handlers) DeleteLinkAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid link ID")
		return
	}
	if err := h.Service.store.deactivateLinkAdmin(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// ── Internal helpers ────────────────────────────────────────────────

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// Unused but referenced to avoid compiler errors.
var _ = time.Now
var _ = fmt.Sprintf

// ─── P5.6 Affiliate Commission Rates ─────────────────────────────────────

// GetCommissionRates handles GET /admin/affiliate-program/commission-rates.
func (h *Handlers) GetCommissionRates(w http.ResponseWriter, r *http.Request) {
	info := h.Service.GetProgramInfo(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"rates": info.CommissionRates,
	}})
}

// UpdateCommissionRates handles PATCH /admin/affiliate-program/commission-rates.
//
// Ref: japaconnect-engineering/sections/56-user-affiliate-program.md.
// Commission rates are stored in system_settings (affiliate.commission.*) —
// the single source of truth. The previous implementation wrote to the
// affiliate_program_settings JSONB column, which the user-facing
// GetProgramInfo never read, so admin changes never reached users and the
// worker credited at a different rate than displayed (BUG-004).
func (h *Handlers) UpdateCommissionRates(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	var input struct {
		ProMonthly *float64 `json:"pro_monthly"`
		ProAnnual  *float64 `json:"pro_annual"`
		Course     *float64 `json:"course"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if h.Service.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "settings_unavailable", "Settings provider not configured")
		return
	}
	ctx := r.Context()
	if input.ProMonthly != nil {
		if err := h.Service.settings.SetSetting(ctx, tenantID, "affiliate.commission.pro_monthly", strconv.FormatFloat(*input.ProMonthly, 'f', -1, 64)); err != nil {
			handleServiceError(w, err)
			return
		}
	}
	if input.ProAnnual != nil {
		if err := h.Service.settings.SetSetting(ctx, tenantID, "affiliate.commission.pro_annual", strconv.FormatFloat(*input.ProAnnual, 'f', -1, 64)); err != nil {
			handleServiceError(w, err)
			return
		}
	}
	if input.Course != nil {
		if err := h.Service.settings.SetSetting(ctx, tenantID, "affiliate.commission.course", strconv.FormatFloat(*input.Course, 'f', -1, 64)); err != nil {
			handleServiceError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"updated": true}})
}
