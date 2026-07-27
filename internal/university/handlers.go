package university

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/go-chi/chi/v5"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("university: encode response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return u, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
		return pgtype.UUID{}, false
	}
	return id, true
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
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
	default:
		slog.Error("university: service error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
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
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	prof, err := h.service.Apply(r.Context(), tenantID, user.UserID, in)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": prof})
}

// GetMyInstructorProfile handles GET /learn/university/instructor/me.
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
	writeJSON(w, http.StatusOK, map[string]any{"data": prof})
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
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
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
	writeJSON(w, http.StatusOK, map[string]any{"data": prof})
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

	// Also list the instructor's live marketplace tracks.
	tracks, _ := h.service.store.ListTracksByInstructor(r.Context(), tenantID, instructorID)
	live := make([]Track, 0, len(tracks))
	for _, t := range tracks {
		if t.IsPublishedToUniversity {
			live = append(live, trackFromGetRow(t))
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
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
	writeJSON(w, http.StatusOK, map[string]any{"data": tracks})
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
		writeError(w, http.StatusForbidden, "not_approved_instructor", "Only approved instructors may create tracks")
		return
	}

	var body struct {
		Title        string  `json:"title"`
		Slug         string  `json:"slug"`
		Description  *string `json:"description"`
		Difficulty   *string `json:"difficulty"`
		CoverMediaID *string `json:"cover_media_id"`
		PriceKobo    *int64  `json:"price_kobo"`
		Currency     *string `json:"currency"`
		Category     *string `json:"category"`
		Language     *string `json:"language"`
		GeoSpaceID   *string `json:"geo_space_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	// BL-016: validate price_kobo is non-negative when set.
	if body.PriceKobo != nil && *body.PriceKobo < 0 {
		writeError(w, http.StatusBadRequest, "invalid_price", "price_kobo must be non-negative")
		return
	}

	in := CreateTrackInput{
		Title: body.Title, Slug: body.Slug, Description: body.Description,
		Category: body.Category, Currency: body.Currency, Language: body.Language,
		PriceKobo: body.PriceKobo,
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
	writeJSON(w, http.StatusCreated, map[string]any{"data": track})
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
		Title        *string `json:"title"`
		Description  *string `json:"description"`
		Difficulty   *string `json:"difficulty"`
		CoverMediaID *string `json:"cover_media_id"`
		PriceKobo    *int64  `json:"price_kobo"`
		Currency     *string `json:"currency"`
		Category     *string `json:"category"`
		Language     *string `json:"language"`
		GeoSpaceID   *string `json:"geo_space_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	// BL-016: validate price_kobo is non-negative when set.
	if body.PriceKobo != nil && *body.PriceKobo < 0 {
		writeError(w, http.StatusBadRequest, "invalid_price", "price_kobo must be non-negative")
		return
	}

	in := UpdateTrackInput{
		Title: body.Title, Description: body.Description, Difficulty: body.Difficulty,
		Category: body.Category, Currency: body.Currency, Language: body.Language,
		PriceKobo: body.PriceKobo,
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
	writeJSON(w, http.StatusOK, map[string]any{"data": track})
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
	writeJSON(w, http.StatusOK, map[string]any{"data": track})
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
		writeError(w, http.StatusForbidden, "not_track_owner", "You do not own this track")
		return
	}

	summary, err := h.service.store.EarningsSummary(r.Context(), tenantID, prof.ID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"track_id":       trackID.String(),
			"total_enrolled": track.TotalEnrolled,
			"open_kobo":      summary.OpenKobo,
			"paid_kobo":      summary.PaidKobo,
			"total_splits":   summary.TotalSplits,
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
	writeJSON(w, http.StatusOK, map[string]any{
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
		writeError(w, http.StatusBadRequest, "missing_slug", "Track slug is required")
		return
	}

	track, err := h.service.store.GetCatalogTrackBySlug(r.Context(), tenantID, slug)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": track})
}

// ---------------------------------------------------------------------------
// Creator earnings (self)
// ---------------------------------------------------------------------------

// GetMyEarnings handles GET /learn/university/my/earnings.
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
	writeJSON(w, http.StatusOK, map[string]any{"data": summary})
}

// GetMyPayouts handles GET /learn/university/my/payouts.
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
	writeJSON(w, http.StatusOK, map[string]any{"data": payouts})
}

// ---------------------------------------------------------------------------
// Admin — instructors
// ---------------------------------------------------------------------------

// AdminListInstructors handles GET /admin/university/instructors.
func (h *Handlers) AdminListInstructors(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	limit, offset := parsePagination(r)
	status := r.URL.Query().Get("status")

	rows, err := h.service.store.ListByStatus(r.Context(), tenantID, status, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	total, err := h.service.store.CountByStatus(r.Context(), tenantID, status)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	profiles := make([]InstructorProfile, 0, len(rows))
	for _, row := range rows {
		profiles = append(profiles, profileFromRow(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{
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
		Tier               string  `json:"tier"`
		CommissionOverride *float64 `json:"commission_pct_override"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	prof, err := h.service.ApproveInstructor(r.Context(), tenantID, instructorID, user.UserID, body.Tier, body.CommissionOverride)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": prof})
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
	writeJSON(w, http.StatusOK, map[string]any{"data": prof})
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
	writeJSON(w, http.StatusOK, map[string]any{"data": prof})
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
	writeJSON(w, http.StatusOK, map[string]any{"data": prof})
}

// ---------------------------------------------------------------------------
// Admin — courses & payouts
// ---------------------------------------------------------------------------

// AdminListPendingTracks handles GET /admin/university/tracks/pending.
func (h *Handlers) AdminListPendingTracks(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	limit, offset := parsePagination(r)

	rows, err := h.service.store.ListPendingTracks(r.Context(), tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
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
	writeJSON(w, http.StatusOK, map[string]any{"data": track})
}

// AdminListPayouts handles GET /admin/university/payouts.
func (h *Handlers) AdminListPayouts(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	limit, offset := parsePagination(r)
	status := r.URL.Query().Get("status")

	payouts, err := h.service.store.ListPayoutsByStatus(r.Context(), tenantID, status, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": payouts})
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
	writeJSON(w, http.StatusOK, map[string]any{"data": payout})
}

// trackFromGetRow maps a ListUniversityTracksByInstructorRow into the Track DTO.
func trackFromGetRow(r gen.ListUniversityTracksByInstructorRow) Track {
	t := Track{
		ID: r.ID.String(), Title: r.Title, Slug: r.Slug, Description: r.Description,
		Difficulty: r.Difficulty, Currency: r.Currency, Language: r.Language,
		TotalEnrolled: r.TotalEnrolled, IsPublished: r.IsPublished,
		IsPublishedToUniversity: r.IsPublishedToUniversity,
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
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
	return t
}
