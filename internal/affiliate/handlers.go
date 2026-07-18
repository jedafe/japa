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
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/worker"
)

// TaskEnqueueClient is the interface for enqueueing background tasks.
type TaskEnqueueClient interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Handlers provides HTTP handlers for the affiliate program.
type Handlers struct {
	Service     *Service
	AsynqClient TaskEnqueueClient
}

// NewHandlers creates a Handlers backed by the given service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// enqueueRecordClick enqueues an async affiliate click recording task.
// Fire-and-forget: errors are logged but never propagated.
func (h *Handlers) enqueueRecordClick(tenantID, linkID pgtype.UUID, userID string, ipHash string) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"tenant_id": tenantID.String(),
		"link_id":   linkID.String(),
		"user_id":   userID,
		"ip_hash":   ipHash,
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
	case errors.Is(err, ErrInvalidProductType):
		writeError(w, http.StatusBadRequest, "invalid_product_type", err.Error())
	case errors.Is(err, errpkg.ErrRequired):
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
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
func (h *Handlers) Redirect(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	link, err := h.Service.GetLinkByCode(r.Context(), code)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	// Deduplicate clicks: same IP + user_agent within 1 hour = same click
	ipHash := sha256Hex(realIP(r))
	uaHash := sha256Hex(r.Header.Get("User-Agent"))

	// Record click async (fire-and-forget)
	tenantID := link.TenantID
	referrer := r.Header.Get("Referer")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		referrerURL := referrer
		_ = h.Service.RecordClick(ctx, RecordClickInput{
			LinkID:      link.ID,
			IPHash:      ipHash,
			UAHash:      uaHash,
			ReferrerURL: &referrerURL,
			TenantID:    tenantID,
		})
	}()

	// Enqueue async affiliate click recording task — fire-and-forget.
	h.enqueueRecordClick(tenantID, link.ID, "", ipHash)

	http.Redirect(w, r, link.DestinationUrl, http.StatusFound)
}

// ── Admin Routes ────────────────────────────────────────────────────

// ListMembersAdmin handles GET /api/v1/admin/affiliate-program/members
func (h *Handlers) ListMembersAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	status := parseQueryString(r, "status")
	page := int32(parseQueryInt(r, "page", 1))
	pageSize := int32(parseQueryInt(r, "page_size", DefaultPageSize))

	members, err := h.Service.ListMembers(r.Context(), tenantID, status, page, pageSize)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": members})
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
func (h *Handlers) ListAllLinksAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	page := int32(parseQueryInt(r, "page", 1))
	pageSize := int32(parseQueryInt(r, "page_size", DefaultPageSize))

	links, err := h.Service.ListAllLinks(r.Context(), tenantID, page, pageSize)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": links})
}

// ── Internal helpers ────────────────────────────────────────────────

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func realIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return r.RemoteAddr
}

func referrerPtr(r *http.Request) *string {
	ref := r.Referer()
	if ref == "" {
		return nil
	}
	return &ref
}

func parseQueryString(r *http.Request, key string) *string {
	v := r.URL.Query().Get(key)
	if v == "" {
		return nil
	}
	return &v
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
func (h *Handlers) UpdateCommissionRates(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	_ = tenantID
	var input struct {
		ProMonthly *int `json:"pro_monthly"`
		ProAnnual  *int `json:"pro_annual"`
		Course     *int `json:"course"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	// Commission rates are stored in program settings — update via raw SQL
	if input.ProMonthly != nil || input.ProAnnual != nil || input.Course != nil {
		_, _ = h.Service.db.Writer.Exec(r.Context(),
			`INSERT INTO affiliate_program_settings (tenant_id, commission_rates, updated_at)
			 VALUES ($1, jsonb_build_object('pro_monthly', COALESCE($2,0), 'pro_annual', COALESCE($3,0), 'course', COALESCE($4,0)), now())
			 ON CONFLICT (tenant_id) DO UPDATE SET
			   commission_rates = jsonb_build_object('pro_monthly', COALESCE($2, (affiliate_program_settings.commission_rates->>'pro_monthly')::int),
			     'pro_annual', COALESCE($3, (affiliate_program_settings.commission_rates->>'pro_annual')::int),
			     'course', COALESCE($4, (affiliate_program_settings.commission_rates->>'course')::int)),
			   updated_at = now()`,
			tenantID, input.ProMonthly, input.ProAnnual, input.Course)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"updated": true}})
}
