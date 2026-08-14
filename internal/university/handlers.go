package university

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes HTTP endpoints for the Japa University creator economy.
// Each handler parses input, calls one service method, and renders a JSON
// response following the JapaConnect API envelope.
type Handlers struct {
	service *Service
}

// NewHandlers creates university handlers bound to the provided service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return u, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
		return pgtype.UUID{}, false
	}
	return id, true
}

// scanOptionalUUIDParam parses a query-string UUID that may be absent. Returns
// (zero, false) when the param is missing or empty (meaning "no filter"); on a
// malformed value it returns (zero, false) as well — the caller treats both as
// "skip this filter" rather than 400-ing, since these are optional filters.
func scanOptionalUUIDParam(r *http.Request, name string) (pgtype.UUID, bool) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return pgtype.UUID{}, false
	}
	var id pgtype.UUID
	if err := id.Scan(v); err != nil {
		return pgtype.UUID{}, false
	}
	return id, true
}

// parseOptionalTime parses an RFC 3339 query-string timestamp that may be
// absent. Returns (zero, false) when missing/empty/malformed.
func parseOptionalTime(r *http.Request, name string) (pgtype.Timestamptz, bool) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return pgtype.Timestamptz{}, false
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return pgtype.Timestamptz{}, false
	}
	return pgtype.Timestamptz{Time: t, Valid: true}, true
}

func parsePagination(r *http.Request) (int32, int32) {
	limit, offset := int32(20), int32(0)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n)
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = int32(n)
		}
	}
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			offset = (int32(n) - 1) * limit
		}
	}
	return limit, offset
}

func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
}

// ---------------------------------------------------------------------------
// Instructor (creator) — self
// ---------------------------------------------------------------------------

// Apply handles POST /learn/university/instructor/apply.
func (h *Handlers) Apply(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var in ApplyInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	prof, err := h.service.Apply(r.Context(), tenantID, user.UserID, in)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": prof})
}

// GetMyInstructorProfile handles GET /learn/university/instructor/me.
// Returns the profile plus aggregate dashboard stats (total_enrolled,
// total_revenue_kobo, courses_count) so the creator dashboard can render
// without a second round-trip (BUG-003).
func (h *Handlers) GetMyInstructorProfile(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	prof, err := h.service.GetMyProfile(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Enrich with aggregate dashboard stats.
	var profID pgtype.UUID
	_ = profID.Scan(prof.ID)
	summary, _ := h.service.store.DashboardSummary(r.Context(), tenantID, profID)
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"id":                      prof.ID,
		"tenant_id":               prof.TenantID,
		"user_id":                 prof.UserID,
		"display_name":            prof.DisplayName,
		"headline":                prof.Headline,
		"bio":                     prof.Bio,
		"social_links":            prof.SocialLinks,
		"tier":                    prof.Tier,
		"status":                  prof.Status,
		"commission_pct_override": prof.CommissionPct,
		"kyc_status":              prof.KycStatus,
		"application_note":        prof.ApplicationNote,
		"reviewed_at":             prof.ReviewedAt,
		"created_at":              prof.CreatedAt,
		"updated_at":              prof.UpdatedAt,
		"total_enrolled":          summary.TotalEnrolled,
		"total_revenue_kobo":      summary.TotalRevenueKobo,
		"courses_count":           summary.CoursesCount,
	}})
}

// UpdateMyInstructorProfile handles PATCH /learn/university/instructor/me.
func (h *Handlers) UpdateMyInstructorProfile(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var body struct {
		DisplayName  *string         `json:"display_name"`
		Headline     *string         `json:"headline"`
		Bio          *string         `json:"bio"`
		SocialLinks  json.RawMessage `json:"social_links"`
		PayoutMethod json.RawMessage `json:"payout_method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	in := UpdateSelfInput{Headline: body.Headline, Bio: body.Bio}
	if body.DisplayName != nil {
		in.DisplayName = *body.DisplayName
	}
	if len(body.SocialLinks) > 0 {
		in.SocialLinks = body.SocialLinks
	}
	if len(body.PayoutMethod) > 0 {
		in.PayoutMethod = body.PayoutMethod
	}

	prof, err := h.service.UpdateMyProfile(r.Context(), tenantID, user.UserID, in)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": prof})
}

// ---------------------------------------------------------------------------
// Instructor — public
// ---------------------------------------------------------------------------

// GetPublicInstructor handles GET /learn/university/instructors/{id}.
func (h *Handlers) GetPublicInstructor(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	instructorID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	prof, err := h.service.store.GetPublic(r.Context(), tenantID, instructorID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// List only live marketplace tracks, filtered at the DB level (BUG-017).
	pubTracks, _ := h.service.store.ListPublishedTracksByInstructor(r.Context(), tenantID, instructorID)
	live := make([]Track, 0, len(pubTracks))
	for _, t := range pubTracks {
		live = append(live, trackFromPublishedRow(t))
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"instructor": prof,
			"tracks":     live,
		},
	})
}

// ---------------------------------------------------------------------------
// Creator track management
// ---------------------------------------------------------------------------

// ListMyTracks handles GET /learn/university/my/tracks.
func (h *Handlers) ListMyTracks(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	prof, err := h.service.store.GetByUser(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	rows, err := h.service.store.ListTracksByInstructor(r.Context(), tenantID, prof.ID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	tracks := make([]Track, 0, len(rows))
	for _, row := range rows {
		tracks = append(tracks, trackFromGetRow(row))
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tracks})
}

// CreateMyTrack handles POST /learn/university/my/tracks.
func (h *Handlers) CreateMyTrack(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	prof, err := h.service.store.GetByUser(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if prof.Status != "approved" {
		errpkg.WriteError(w, http.StatusForbidden, "not_approved_instructor", "Only approved instructors may create tracks")
		return
	}

	var body struct {
		Title                string  `json:"title"`
		Slug                 string  `json:"slug"`
		Description          *string `json:"description"`
		Difficulty           *string `json:"difficulty"`
		CoverMediaID         *string `json:"cover_media_id"`
		PriceKobo            *int64  `json:"price_kobo"`
		Currency             *string `json:"currency"`
		Category             *string `json:"category"`
		Language             *string `json:"language"`
		GeoSpaceID           *string `json:"geo_space_id"`
		SubscriptionEligible *bool   `json:"subscription_eligible"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	// BL-016: validate price_kobo is non-negative when set.
	if body.PriceKobo != nil && *body.PriceKobo < 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_price", "price_kobo must be non-negative")
		return
	}

	in := CreateTrackInput{
		Title: body.Title, Slug: body.Slug, Description: body.Description,
		Category: body.Category, Currency: body.Currency, Language: body.Language,
		PriceKobo: body.PriceKobo, SubscriptionEligible: body.SubscriptionEligible,
	}
	if body.Difficulty != nil {
		in.Difficulty = *body.Difficulty
	}
	if body.CoverMediaID != nil {
		var cm pgtype.UUID
		if err := cm.Scan(*body.CoverMediaID); err == nil {
			in.CoverMediaID = &cm
		}
	}
	if body.GeoSpaceID != nil && *body.GeoSpaceID != "" {
		var gs pgtype.UUID
		if err := gs.Scan(*body.GeoSpaceID); err == nil {
			in.GeoSpaceID = &gs
		}
	}

	track, err := h.service.CreateTrack(r.Context(), tenantID, prof.ID, in)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": track})
}

// UpdateMyTrack handles PATCH /learn/university/my/tracks/{id}.
func (h *Handlers) UpdateMyTrack(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	trackID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	prof, err := h.service.store.GetByUser(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	var body struct {
		Title                *string `json:"title"`
		Description          *string `json:"description"`
		Difficulty           *string `json:"difficulty"`
		CoverMediaID         *string `json:"cover_media_id"`
		PriceKobo            *int64  `json:"price_kobo"`
		Currency             *string `json:"currency"`
		Category             *string `json:"category"`
		Language             *string `json:"language"`
		GeoSpaceID           *string `json:"geo_space_id"`
		SubscriptionEligible *bool   `json:"subscription_eligible"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	// BL-016: validate price_kobo is non-negative when set.
	if body.PriceKobo != nil && *body.PriceKobo < 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_price", "price_kobo must be non-negative")
		return
	}

	in := UpdateTrackInput{
		Title: body.Title, Description: body.Description, Difficulty: body.Difficulty,
		Category: body.Category, Currency: body.Currency, Language: body.Language,
		PriceKobo: body.PriceKobo, SubscriptionEligible: body.SubscriptionEligible,
	}
	if body.CoverMediaID != nil {
		var cm pgtype.UUID
		if err := cm.Scan(*body.CoverMediaID); err == nil {
			in.CoverMediaID = &cm
		}
	}
	if body.GeoSpaceID != nil && *body.GeoSpaceID != "" {
		var gs pgtype.UUID
		if err := gs.Scan(*body.GeoSpaceID); err == nil {
			in.GeoSpaceID = &gs
		}
	}

	track, err := h.service.UpdateTrack(r.Context(), tenantID, trackID, prof.ID, in)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": track})
}

// SubmitMyTrack handles POST /learn/university/my/tracks/{id}/submit.
func (h *Handlers) SubmitMyTrack(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	trackID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	prof, err := h.service.store.GetByUser(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	track, err := h.service.SubmitTrack(r.Context(), tenantID, trackID, prof.ID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": track})
}

// GetMyTrackStats handles GET /learn/university/my/tracks/{id}/stats.
func (h *Handlers) GetMyTrackStats(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	trackID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	prof, err := h.service.store.GetByUser(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	track, err := h.service.store.GetTrackByID(r.Context(), tenantID, trackID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if !track.InstructorID.Valid || track.InstructorID.String() != prof.ID.String() {
		errpkg.WriteError(w, http.StatusForbidden, "not_track_owner", "You do not own this track")
		return
	}

	summary, err := h.service.store.EarningsSummary(r.Context(), tenantID, prof.ID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Track-specific series + completion rate (BUG-007).
	revSeries, _ := h.service.store.TrackRevenueSeries(r.Context(), tenantID, trackID)
	enrollSeries, _ := h.service.store.TrackEnrollmentSeries(r.Context(), tenantID, trackID)
	completion, _ := h.service.store.TrackCompletionRate(r.Context(), tenantID, trackID)

	var completionRate float64
	if completion.TotalEnrolled > 0 {
		completionRate = float64(completion.Completed) / float64(completion.TotalEnrolled) * 100
	}

	revData := make([]map[string]any, 0, len(revSeries))
	for _, s := range revSeries {
		if s.RevDate.Valid {
			revData = append(revData, map[string]any{
				"date": s.RevDate.Time.Format("2006-01-02"),
				"kobo": s.Kobo,
			})
		}
	}

	enrollData := make([]map[string]any, 0, len(enrollSeries))
	for _, s := range enrollSeries {
		if s.EnrollDate.Valid {
			enrollData = append(enrollData, map[string]any{
				"date":  s.EnrollDate.Time.Format("2006-01-02"),
				"count": s.Count,
			})
		}
	}

	totalRevenue := summary.OpenKobo + summary.PaidKobo
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"track_id":            trackID.String(),
			"total_enrolled":      track.TotalEnrolled,
			"completion_rate_pct": completionRate,
			"total_revenue_kobo":  totalRevenue,
			"revenue_series":      revData,
			"enrollment_series":   enrollData,
		},
	})
}

// ---------------------------------------------------------------------------
// Marketplace catalog (public)
// ---------------------------------------------------------------------------

// ListCatalog handles GET /learn/university/catalog.
func (h *Handlers) ListCatalog(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	limit, offset := parsePagination(r)

	f := CatalogFilter{
		Category: r.URL.Query().Get("category"),
		Language: r.URL.Query().Get("language"),
		IsFree:   r.URL.Query().Get("is_free") == "true",
	}

	rows, err := h.service.store.ListCatalog(r.Context(), tenantID, f, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	total, err := h.service.store.CountCatalog(r.Context(), tenantID, f)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	items := make([]CatalogTrack, 0, len(rows))
	for _, row := range rows {
		items = append(items, catalogTrackFromRow(row))
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": items,
		"meta": map[string]int64{"total": total, "limit": int64(limit), "offset": int64(offset)},
	})
}

// GetCatalogTrack handles GET /learn/university/catalog/{slug}.
func (h *Handlers) GetCatalogTrack(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Track slug is required")
		return
	}

	track, err := h.service.store.GetCatalogTrackBySlug(r.Context(), tenantID, slug)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	// Map through CatalogTrackDetailFromRow to add the `level` JSON alias for
	// the frontend TrackDetail.level contract (ADR-060).
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": CatalogTrackDetailFromRow(track)})
}

// ---------------------------------------------------------------------------
// Creator earnings (self)
// ---------------------------------------------------------------------------

// GetMyEarnings handles GET /learn/university/my/earnings.
// Returns the earnings summary (open/paid/total) plus a daily revenue_series
// for the LayerChart (BUG-005).
func (h *Handlers) GetMyEarnings(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	prof, err := h.service.store.GetByUser(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	summary, err := h.service.store.EarningsSummary(r.Context(), tenantID, prof.ID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	series, _ := h.service.store.RevenueSeries(r.Context(), tenantID, prof.ID)
	revenueSeries := make([]map[string]any, 0, len(series))
	for _, s := range series {
		if s.RevDate.Valid {
			revenueSeries = append(revenueSeries, map[string]any{
				"date": s.RevDate.Time.Format("2006-01-02"),
				"kobo": s.Kobo,
			})
		}
	}

	totalEarned := summary.OpenKobo + summary.PaidKobo
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"total_earned_kobo":   totalEarned,
		"total_paid_out_kobo": summary.PaidKobo,
		"pending_kobo":        summary.OpenKobo,
		"revenue_series":      revenueSeries,
	}})
}

// GetMyPayouts handles GET /learn/university/my/payouts.
// Returns payouts mapped to a clean DTO with the fields the frontend expects
// (amount_kobo, requested_at, processed_at, status) (BUG-006).
func (h *Handlers) GetMyPayouts(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	limit, offset := parsePagination(r)

	prof, err := h.service.store.GetByUser(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	payouts, err := h.service.store.ListPayoutsByInstructor(r.Context(), tenantID, prof.ID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	items := make([]payoutDTO, 0, len(payouts))
	for _, p := range payouts {
		items = append(items, payoutFromRow(p))
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// payoutDTO is the clean API-facing payout record (BUG-006).
type payoutDTO struct {
	ID          string     `json:"id"`
	AmountKobo  int64      `json:"amount_kobo"`
	Status      string     `json:"status"`
	PeriodStart string     `json:"period_start"`
	PeriodEnd   string     `json:"period_end"`
	RequestedAt time.Time  `json:"requested_at"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
	Currency    string     `json:"currency"`
}

func payoutFromRow(p gen.InstructorPayout) payoutDTO {
	d := payoutDTO{
		ID:          p.ID.String(),
		AmountKobo:  p.NetKobo,
		Status:      p.Status,
		PeriodStart: p.PeriodStart.Time.Format("2006-01-02"),
		PeriodEnd:   p.PeriodEnd.Time.Format("2006-01-02"),
		RequestedAt: p.CreatedAt.Time,
		Currency:    p.Currency,
	}
	if p.ProcessedAt.Valid {
		d.ProcessedAt = &p.ProcessedAt.Time
	}
	return d
}

// ---------------------------------------------------------------------------
// Admin — instructors
// ---------------------------------------------------------------------------

// AdminListInstructors handles GET /admin/university/instructors.
// Optional ?space_id= scopes the list to instructors with a track targeted at
// that space (ADR-063 admin per-university view).
func (h *Handlers) AdminListInstructors(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	limit, offset := parsePagination(r)
	status := r.URL.Query().Get("status")
	spaceID, _ := scanOptionalUUIDParam(r, "space_id")

	rows, err := h.service.store.ListByStatus(r.Context(), tenantID, status, spaceID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	total, err := h.service.store.CountByStatus(r.Context(), tenantID, status, spaceID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	profiles := make([]InstructorProfile, 0, len(rows))
	for _, row := range rows {
		profiles = append(profiles, profileFromRow(row))
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": profiles,
		"meta": map[string]int64{"total": total, "limit": int64(limit), "offset": int64(offset)},
	})
}

// AdminApproveInstructor handles POST /admin/university/instructors/{id}/approve.
func (h *Handlers) AdminApproveInstructor(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	instructorID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Tier               string   `json:"tier"`
		CommissionOverride *float64 `json:"commission_pct_override"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	prof, err := h.service.ApproveInstructor(r.Context(), tenantID, instructorID, user.UserID, body.Tier, body.CommissionOverride)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": prof})
}

// AdminRejectInstructor handles POST /admin/university/instructors/{id}/reject.
func (h *Handlers) AdminRejectInstructor(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	instructorID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	prof, err := h.service.RejectInstructor(r.Context(), tenantID, instructorID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": prof})
}

// AdminSuspendInstructor handles POST /admin/university/instructors/{id}/suspend.
func (h *Handlers) AdminSuspendInstructor(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	instructorID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	prof, err := h.service.SuspendInstructor(r.Context(), tenantID, instructorID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": prof})
}

// AdminReinstateInstructor handles POST /admin/university/instructors/{id}/reinstate.
func (h *Handlers) AdminReinstateInstructor(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	instructorID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	prof, err := h.service.ReinstateInstructor(r.Context(), tenantID, instructorID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": prof})
}

// AdminDeleteInstructor handles DELETE /admin/university/instructors/{id}.
// Blocked while the instructor is approved — reject or suspend first.
func (h *Handlers) AdminDeleteInstructor(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	instructorID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.DeleteInstructor(r.Context(), tenantID, instructorID); err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// Admin — courses & payouts
// ---------------------------------------------------------------------------

// AdminListPendingTracks handles GET /admin/university/tracks/pending.
// Optional ?space_id= scopes the list to tracks targeted at that space.
func (h *Handlers) AdminListPendingTracks(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	limit, offset := parsePagination(r)
	spaceID, _ := scanOptionalUUIDParam(r, "space_id")

	rows, err := h.service.store.ListPendingTracks(r.Context(), tenantID, spaceID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if rows == nil {
		rows = []gen.ListPendingUniversityTracksRow{}
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// AdminApproveTrack handles POST /admin/university/tracks/{id}/approve.
func (h *Handlers) AdminApproveTrack(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	trackID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	track, err := h.service.ApproveTrack(r.Context(), tenantID, trackID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": track})
}

// AdminListPayouts handles GET /admin/university/payouts.
// Optional ?space_id= scopes the list to payouts for instructors with a track
// targeted at that space.
func (h *Handlers) AdminListPayouts(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	limit, offset := parsePagination(r)
	status := r.URL.Query().Get("status")
	spaceID, _ := scanOptionalUUIDParam(r, "space_id")

	payouts, err := h.service.store.ListPayoutsByStatus(r.Context(), tenantID, spaceID, status, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if payouts == nil {
		payouts = []gen.ListInstructorPayoutsByStatusRow{}
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": payouts})
}

// AdminGetPayout handles GET /admin/university/payouts/{id}.
func (h *Handlers) AdminGetPayout(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	payoutID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	payout, err := h.service.store.GetPayout(r.Context(), tenantID, payoutID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": payout})
}

// AdminProcessPayout handles POST /admin/university/payouts/{id}/process.
func (h *Handlers) AdminProcessPayout(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	payoutID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.SendInstructorPayout(r.Context(), tenantID, payoutID); err != nil {
		handleServiceError(w, err)
		return
	}
	payout, err := h.service.store.GetPayout(r.Context(), tenantID, payoutID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": payout})
}

// AdminListRevenueSplits handles GET /admin/university/revenue-splits.
// Read-only audit viewer over the immutable revenue_splits ledger (ADR-063 §9).
// Supports optional filters: ?instructor_id= ?track_id= ?source=purchase|pro_stream
// ?payout_id= ?from= ?to= (RFC 3339, half-open [from,to)) and offset pagination
// (?page=&limit=). Returns the standard {data, meta{total,page,limit}} envelope.
func (h *Handlers) AdminListRevenueSplits(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	limit, offset := parsePagination(r)

	f := ListRevenueSplitsFilter{
		Source: r.URL.Query().Get("source"),
	}
	if id, ok := scanOptionalUUIDParam(r, "instructor_id"); ok {
		f.InstructorID = id
	}
	if id, ok := scanOptionalUUIDParam(r, "track_id"); ok {
		f.TrackID = id
	}
	if id, ok := scanOptionalUUIDParam(r, "payout_id"); ok {
		f.PayoutID = id
	}
	if id, ok := scanOptionalUUIDParam(r, "space_id"); ok {
		f.SpaceID = id
	}
	if ts, ok := parseOptionalTime(r, "from"); ok {
		f.FromTs = ts
	}
	if ts, ok := parseOptionalTime(r, "to"); ok {
		f.ToTs = ts
	}

	splits, err := h.service.store.ListRevenueSplits(r.Context(), tenantID, f, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if splits == nil {
		splits = []gen.ListRevenueSplitsRow{}
	}
	total, err := h.service.store.CountRevenueSplits(r.Context(), tenantID, f)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	page := int32(0)
	if limit > 0 {
		page = offset/limit + 1
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": splits,
		"meta": map[string]any{"total": total, "page": page, "limit": limit},
	})
}

// AdminGetRevenueSplit handles GET /admin/university/revenue-splits/{id}.
// Read-only single fetch of an immutable revenue-split audit record (ADR-063 §9).
func (h *Handlers) AdminGetRevenueSplit(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	splitID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	split, err := h.service.store.GetRevenueSplitByID(r.Context(), tenantID, splitID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": split})
}

// trackFromGetRow maps a ListUniversityTracksByInstructorRow into the Track DTO.
func trackFromGetRow(r gen.ListUniversityTracksByInstructorRow) Track {
	t := Track{
		ID: r.ID.String(), Title: r.Title, Slug: r.Slug, Description: r.Description,
		Difficulty: r.Difficulty, Currency: r.Currency, Language: r.Language,
		TotalEnrolled: r.TotalEnrolled, IsPublished: r.IsPublished,
		IsPublishedToUniversity: r.IsPublishedToUniversity,
		SubmissionStatus:        r.UniversitySubmissionStatus,
		TotalRevenueKobo:        r.TotalRevenueKobo,
		RejectionReason:         r.UniversityRejectionReason,
		ThumbnailURL:            r.ThumbnailUrl,
		CreatedAt:               r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.CoverMediaID.Valid {
		s := r.CoverMediaID.String()
		t.CoverMediaID = &s
	}
	if r.InstructorID.Valid {
		s := r.InstructorID.String()
		t.InstructorID = &s
	}
	if r.PriceKobo != nil {
		v := *r.PriceKobo
		t.PriceKobo = &v
	}
	if r.Category != nil {
		t.Category = r.Category
	}
	if r.UniversitySubmittedAt.Valid {
		tm := r.UniversitySubmittedAt.Time
		t.SubmittedAt = &tm
	}
	if r.UniversityReviewedAt.Valid {
		tm := r.UniversityReviewedAt.Time
		t.ReviewedAt = &tm
	}
	return t
}

// trackFromPublishedRow maps a ListPublishedTracksByInstructorRow into the Track DTO.
func trackFromPublishedRow(r gen.ListPublishedTracksByInstructorRow) Track {
	t := Track{
		ID: r.ID.String(), Title: r.Title, Slug: r.Slug, Description: r.Description,
		Difficulty: r.Difficulty, Currency: r.Currency, Language: r.Language,
		TotalEnrolled: r.TotalEnrolled, IsPublished: r.IsPublished,
		IsPublishedToUniversity: r.IsPublishedToUniversity,
		SubmissionStatus:        r.UniversitySubmissionStatus,
		RejectionReason:         r.UniversityRejectionReason,
		ThumbnailURL:            r.ThumbnailUrl,
		CreatedAt:               r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.CoverMediaID.Valid {
		s := r.CoverMediaID.String()
		t.CoverMediaID = &s
	}
	if r.InstructorID.Valid {
		s := r.InstructorID.String()
		t.InstructorID = &s
	}
	if r.PriceKobo != nil {
		v := *r.PriceKobo
		t.PriceKobo = &v
	}
	if r.Category != nil {
		t.Category = r.Category
	}
	if r.UniversitySubmittedAt.Valid {
		tm := r.UniversitySubmittedAt.Time
		t.SubmittedAt = &tm
	}
	if r.UniversityReviewedAt.Valid {
		tm := r.UniversityReviewedAt.Time
		t.ReviewedAt = &tm
	}
	return t
}

// ---------------------------------------------------------------------------
// Admin — all tracks, reject track, fail payout, update instructor (BUG-002)
// ---------------------------------------------------------------------------

// AdminListAllTracks handles GET /admin/university/tracks.
// Returns all marketplace tracks (not just pending), optionally filtered by
// submission_status (BUG-002). Optional ?space_id= scopes to tracks targeted
// at that space.
func (h *Handlers) AdminListAllTracks(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	limit, offset := parsePagination(r)
	status := r.URL.Query().Get("status")
	spaceID, _ := scanOptionalUUIDParam(r, "space_id")

	rows, err := h.service.store.ListAllTracks(r.Context(), tenantID, spaceID, status, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if rows == nil {
		rows = []gen.ListAllUniversityTracksRow{}
	}
	total, err := h.service.store.CountAllTracks(r.Context(), tenantID, spaceID, status)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": rows,
		"meta": map[string]int64{"total": total, "limit": int64(limit), "offset": int64(offset)},
	})
}

// AdminRejectTrack handles POST /admin/university/tracks/{id}/reject.
// Rejects a submitted track with an optional reason (BUG-002, BUG-004).
func (h *Handlers) AdminRejectTrack(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	trackID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Reason *string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	track, err := h.service.store.RejectTrack(r.Context(), tenantID, trackID, body.Reason)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": track})
}

// AdminFailPayout handles POST /admin/university/payouts/{id}/fail.
// Marks a payout as failed with an optional reason (BUG-002).
func (h *Handlers) AdminFailPayout(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	payoutID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Reason *string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	payout, err := h.service.store.UpdatePayoutStatus(r.Context(), tenantID, payoutID, "failed", nil, body.Reason)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": payout})
}

// AdminUpdateInstructor handles PATCH /admin/university/instructors/{id}.
// Updates an instructor's tier and/or commission override (BUG-002).
func (h *Handlers) AdminUpdateInstructor(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	instructorID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Tier               string   `json:"tier"`
		CommissionOverride *float64 `json:"commission_pct_override"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	var tier *string
	if body.Tier != "" {
		t := body.Tier
		tier = &t
	}
	var commission *pgtype.Numeric
	if body.CommissionOverride != nil {
		var n pgtype.Numeric
		if err := n.Scan(fmt.Sprintf("%f", *body.CommissionOverride)); err == nil {
			commission = &n
		}
	}

	prof, err := h.service.store.UpdateInstructorAdmin(r.Context(), tenantID, instructorID, tier, commission)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": profileFromRow(prof)})
}

// AdminFeatureTrack handles POST /admin/university/tracks/{id}/feature.
// Toggles the featured flag on a marketplace track (BUG-012: wires the
// university.course.feature RBAC node).
func (h *Handlers) AdminFeatureTrack(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	trackID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Featured *bool `json:"featured"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	featured := true
	if body.Featured != nil {
		featured = *body.Featured
	}

	track, err := h.service.store.SetTrackFeatured(r.Context(), tenantID, trackID, featured)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": track})
}

// ---------------------------------------------------------------------------
// Track categories (ADR-063 §9.1)
// Ref: japaconnect-engineering/sections/36-learning-platform.md (Track categories)
// ---------------------------------------------------------------------------

// AdminListCategories handles GET /admin/university/categories.
// Supports ?active=true|false and ?search= filters plus ?page=&limit= pagination.
func (h *Handlers) AdminListCategories(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	limit, offset := parsePagination(r)
	active := r.URL.Query().Get("active")
	search := r.URL.Query().Get("search")

	rows, err := h.service.store.ListTrackCategories(r.Context(), tenantID, active, search, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if rows == nil {
		rows = []gen.UniversityTrackCategory{}
	}
	total, err := h.service.store.CountTrackCategories(r.Context(), tenantID, active, search)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	page := int32(0)
	if limit > 0 {
		page = offset/limit + 1
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": rows,
		"meta": map[string]any{"total": total, "page": page, "limit": limit},
	})
}

// AdminCreateCategory handles POST /admin/university/categories.
func (h *Handlers) AdminCreateCategory(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	var body struct {
		Slug      string `json:"slug"`
		Name      string `json:"name"`
		SortOrder *int32 `json:"sort_order"`
		IsActive  *bool  `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if body.Slug == "" || body.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "slug and name are required")
		return
	}
	sortOrder := int32(0)
	if body.SortOrder != nil {
		sortOrder = *body.SortOrder
	}
	isActive := true
	if body.IsActive != nil {
		isActive = *body.IsActive
	}
	cat, err := h.service.store.CreateTrackCategory(r.Context(), tenantID, body.Slug, body.Name, sortOrder, isActive)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": cat})
}

// AdminGetCategory handles GET /admin/university/categories/{id}.
func (h *Handlers) AdminGetCategory(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	cat, err := h.service.store.GetTrackCategory(r.Context(), tenantID, id)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": cat})
}

// AdminUpdateCategory handles PATCH /admin/university/categories/{id}.
func (h *Handlers) AdminUpdateCategory(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Slug      *string `json:"slug"`
		Name      *string `json:"name"`
		SortOrder *int32  `json:"sort_order"`
		IsActive  *bool   `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	cat, err := h.service.store.UpdateTrackCategory(r.Context(), tenantID, id, body.Name, body.Slug, body.SortOrder, body.IsActive)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": cat})
}

// AdminDeleteCategory handles DELETE /admin/university/categories/{id}.
// Blocked if any learning_tracks still reference the category slug.
func (h *Handlers) AdminDeleteCategory(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	// Fetch the category first to get the slug for the track reference check.
	cat, err := h.service.store.GetTrackCategory(r.Context(), tenantID, id)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	count, err := h.service.store.CountTracksByCategory(r.Context(), tenantID, cat.Slug)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if count > 0 {
		errpkg.WriteError(w, http.StatusConflict, "category_in_use",
			fmt.Sprintf("Cannot delete category '%s': %d track(s) still reference it. Reassign or clear those tracks first.", cat.Slug, count))
		return
	}
	if err := h.service.store.DeleteTrackCategory(r.Context(), tenantID, id); err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true, "id": id}})
}

// PublicListActiveCategories handles GET /learn/university/categories.
// No auth required — returns only active categories for the catalog dropdown.
func (h *Handlers) PublicListActiveCategories(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	rows, err := h.service.store.ListActiveTrackCategories(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if rows == nil {
		rows = []gen.ListActiveUniversityTrackCategoriesRow{}
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rows})
}
