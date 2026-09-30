// Package university implements the Japa University creator economy backend
// (ADR-063): instructor profiles, marketplace track publishing, paid/Pro
// enrollment revenue splits, and instructor payouts.
// // The package follows the layered architecture: Handlers parse/validate input
// and render responses; Service holds business logic and transaction
// boundaries; Store wraps the sqlc-generated queries with read/write pool
// separation. Asynq tasks (enrollment processing, payout calculation/execution,
// notifications) are defined in the worker package and delegated here via the
// Processor interface.
// // Ref: japaconnect-engineering/sections/36-learning-platform.md (Japa University)
// //	japaconnect-engineering/adr/ADR-063-japa-university-creator-economy.md
package university

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/media"
)

// Store wraps the sqlc-generated queries with read/write pool separation.
// All writes go through the write pool; all reads through the read pool.
// Platform scoping is enforced at the query level (every query filters on
// ).
type Store struct {
	read  *gen.Queries
	write *gen.Queries
	db    *db.DB
}

// NewStore creates a university store bound to the provided database pools.
func NewStore(database *db.DB) *Store {
	return &Store{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
		db:    database,
	}
}

// DB exposes the underlying pool for transactional service methods.
func (s *Store) DB() *db.DB { return s.db }

// ---------------------------------------------------------------------------
// Instructor profile DTOs
// ---------------------------------------------------------------------------

// InstructorProfile is the API-facing instructor profile record.
type InstructorProfile struct {
	ID string `json:"id"`

	UserID          string          `json:"user_id"`
	DisplayName     string          `json:"display_name"`
	Headline        *string         `json:"headline,omitempty"`
	Bio             *string         `json:"bio,omitempty"`
	SocialLinks     json.RawMessage `json:"social_links"`
	Tier            string          `json:"tier"`
	Status          string          `json:"status"`
	CommissionPct   *float64        `json:"commission_pct_override,omitempty"`
	KycStatus       string          `json:"kyc_status"`
	ApplicationNote *string         `json:"application_note,omitempty"`
	ReviewedAt      *time.Time      `json:"reviewed_at,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

func profileFromRow(r gen.InstructorProfile) InstructorProfile {
	p := InstructorProfile{
		ID: r.ID.String(),

		UserID:          r.UserID.String(),
		DisplayName:     r.DisplayName,
		Headline:        r.Headline,
		Bio:             r.Bio,
		SocialLinks:     rawJSON(r.SocialLinks),
		Tier:            r.Tier,
		Status:          r.Status,
		KycStatus:       r.KycStatus,
		ApplicationNote: r.ApplicationNote,
		CreatedAt:       r.CreatedAt.Time,
		UpdatedAt:       r.UpdatedAt.Time,
	}

	if r.ReviewedAt.Valid {
		t := r.ReviewedAt.Time
		p.ReviewedAt = &t
	}

	if r.CommissionPctOverride.Valid {
		if f, err := r.CommissionPctOverride.Float64Value(); err == nil && f.Valid {
			v := f.Float64
			p.CommissionPct = &v
		}
	}

	return p
}

// rawJSON normalizes a sqlc jsonb []byte into a non-nil json.RawMessage so
// the API always emits a JSON object (never null) for jsonb columns.
func rawJSON(b []byte) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("{}")
	}

	return json.RawMessage(b)
}

// ApplyInput is the JSON body for POST /learn/university/instructor/apply.
type ApplyInput struct {
	DisplayName     string          `json:"display_name"`
	Headline        *string         `json:"headline,omitempty"`
	Bio             *string         `json:"bio,omitempty"`
	SocialLinks     json.RawMessage `json:"social_links,omitempty"`
	ApplicationNote *string         `json:"application_note,omitempty"`
}

// Apply submits (or re-submits while pending/rejected) an instructor
// application. Returns the resulting profile row.
func (s *Store) Apply(ctx context.Context, userID pgtype.UUID, in ApplyInput) (gen.InstructorProfile, error) {
	var socialLinks []byte
	if len(in.SocialLinks) > 0 {
		socialLinks = in.SocialLinks
	}

	row, err := s.write.UpsertInstructorApplication(ctx, gen.UpsertInstructorApplicationParams{

		UserID:          userID,
		DisplayName:     in.DisplayName,
		Headline:        in.Headline,
		Bio:             in.Bio,
		SocialLinks:     socialLinks,
		ApplicationNote: in.ApplicationNote,
	})
	if err != nil {
		return gen.InstructorProfile{}, fmt.Errorf("upsert instructor application: %w", err)
	}

	return row, nil
}

// GetByUser returns the instructor profile for a user (any status).
func (s *Store) GetByUser(ctx context.Context, userID pgtype.UUID) (gen.InstructorProfile, error) {
	return s.read.GetInstructorProfileByUser(ctx,
		userID,
	)
}

// GetByID returns the instructor profile by ID (any status).
func (s *Store) GetByID(ctx context.Context, id pgtype.UUID) (gen.InstructorProfile, error) {
	return s.read.GetInstructorProfileByID(ctx,
		id,
	)
}

// GetPublic returns the public-facing instructor profile (approved only).
func (s *Store) GetPublic(ctx context.Context, id pgtype.UUID) (gen.GetPublicInstructorProfileRow, error) {
	return s.read.GetPublicInstructorProfile(ctx,
		id,
	)
}

// ListByStatus lists instructor profiles filtered by status (empty = all).
// spaceID, when Valid, restricts to instructors with a track targeted at that
// space (ADR-063 admin per-university view).
func (s *Store) ListByStatus(ctx context.Context, status string, spaceID pgtype.UUID, limit, offset int32) ([]gen.InstructorProfile, error) {
	return s.read.ListInstructorProfilesByStatus(ctx, gen.ListInstructorProfilesByStatusParams{
		Status: status, SpaceID: spaceID, PageLimit: limit, PageOffset: offset,
	})
}

// CountByStatus counts instructor profiles filtered by status (empty = all).
// spaceID, when Valid, restricts to instructors with a track targeted at that
// space (ADR-063 admin per-university view).
func (s *Store) CountByStatus(ctx context.Context, status string, spaceID pgtype.UUID) (int64, error) {
	return s.read.CountInstructorProfilesByStatus(ctx, gen.CountInstructorProfilesByStatusParams{
		Status: status, SpaceID: spaceID,
	})
}

// UpdateSelf updates creator-editable fields on the caller's own profile.
func (s *Store) UpdateSelf(ctx context.Context, userID pgtype.UUID, in UpdateSelfInput) (gen.InstructorProfile, error) {
	return s.write.UpdateInstructorProfileSelf(ctx, gen.UpdateInstructorProfileSelfParams{

		UserID:       userID,
		DisplayName:  in.DisplayName,
		Headline:     in.Headline,
		Bio:          in.Bio,
		SocialLinks:  in.SocialLinks,
		PayoutMethod: in.PayoutMethod,
	})
}

// UpdateSelfInput holds creator-editable profile fields.
type UpdateSelfInput struct {
	DisplayName  string
	Headline     *string
	Bio          *string
	SocialLinks  []byte
	PayoutMethod []byte
}

// Approve transitions a pending application to approved.
func (s *Store) Approve(ctx context.Context, id, reviewer pgtype.UUID, tier string, commissionOverride *float64) (gen.InstructorProfile, error) {
	params := gen.ApproveInstructorApplicationParams{
		ID: id, ReviewedBy: reviewer,
	}

	if tier != "" {
		t := tier
		params.Tier = &t
	}

	if commissionOverride != nil {
		var n pgtype.Numeric
		if err := n.Scan(fmt.Sprintf("%f", *commissionOverride)); err == nil {
			params.CommissionPctOverride = n
		}
	}

	return s.write.ApproveInstructorApplication(ctx, params)
}

// Reject transitions a pending application to rejected.
func (s *Store) Reject(ctx context.Context, id, reviewer pgtype.UUID) (gen.InstructorProfile, error) {
	return s.write.RejectInstructorApplication(ctx, gen.RejectInstructorApplicationParams{
		ID: id, ReviewedBy: reviewer,
	})
}

// Suspend transitions an approved instructor to suspended.
func (s *Store) Suspend(ctx context.Context, id, reviewer pgtype.UUID) (gen.InstructorProfile, error) {
	return s.write.SuspendInstructor(ctx, gen.SuspendInstructorParams{
		ID: id, ReviewedBy: reviewer,
	})
}

// Reinstate transitions a suspended instructor back to approved.
func (s *Store) Reinstate(ctx context.Context, id, reviewer pgtype.UUID) (gen.InstructorProfile, error) {
	return s.write.ReinstateInstructor(ctx, gen.ReinstateInstructorParams{
		ID: id, ReviewedBy: reviewer,
	})
}

// Delete permanently removes an instructor profile/application. Blocked
// while the instructor is approved (reject or suspend first).
func (s *Store) Delete(ctx context.Context, id pgtype.UUID) (gen.DeleteInstructorProfileRow, error) {
	return s.write.DeleteInstructorProfile(ctx,
		id,
	)
}

// ---------------------------------------------------------------------------
// Marketplace track DTOs + queries
// ---------------------------------------------------------------------------

// Track is the API-facing marketplace track record.
type Track struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Slug        string  `json:"slug"`
	Description *string `json:"description,omitempty"`
	Difficulty  string  `json:"difficulty"`
	/** Level is an alias for Difficulty, matching the frontend TrackSummary.level
	 * contract (ADR-060). Always equal to Difficulty. */
	Level                   string     `json:"level"`
	CoverMediaID            *string    `json:"cover_media_id,omitempty"`
	InstructorID            *string    `json:"instructor_id,omitempty"`
	PriceKobo               *int64     `json:"price_kobo,omitempty"`
	Currency                string     `json:"currency"`
	Category                *string    `json:"category,omitempty"`
	Language                string     `json:"language"`
	GeoSpaceID              *string    `json:"geo_space_id,omitempty"`
	TotalEnrolled           int32      `json:"total_enrolled"`
	IsPublished             bool       `json:"is_published"`
	IsPublishedToUniversity bool       `json:"is_published_to_university"`
	SubmissionStatus        string     `json:"submission_status"`
	SubmittedAt             *time.Time `json:"submitted_at,omitempty"`
	ReviewedAt              *time.Time `json:"reviewed_at,omitempty"`
	RejectionReason         *string    `json:"rejection_reason,omitempty"`
	TotalRevenueKobo        int64      `json:"total_revenue_kobo"`
	ThumbnailURL            *string    `json:"thumbnail_url,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

func trackFromRow(r gen.GetUniversityTrackByIDRow) Track {
	t := Track{
		ID: r.ID.String(), Title: r.Title, Slug: r.Slug, Description: r.Description,
		Difficulty: r.Difficulty, Level: r.Difficulty, Currency: r.Currency, Language: r.Language,
		TotalEnrolled: r.TotalEnrolled, IsPublished: r.IsPublished,
		IsPublishedToUniversity: r.IsPublishedToUniversity,
		SubmissionStatus:        r.UniversitySubmissionStatus,
		RejectionReason:         r.UniversityRejectionReason,
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

	if r.GeoSpaceID.Valid {
		s := r.GeoSpaceID.String()
		t.GeoSpaceID = &s
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

// CatalogTrack is a marketplace catalog row with instructor + rating.
type CatalogTrack struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Slug        string  `json:"slug"`
	Description *string `json:"description,omitempty"`
	Difficulty  string  `json:"difficulty"`
	/** Level is an alias for Difficulty, matching the frontend TrackSummary.level
	 * contract (ADR-060). Always equal to Difficulty. */
	Level                string  `json:"level"`
	PriceKobo            *int64  `json:"price_kobo,omitempty"`
	Currency             string  `json:"currency"`
	Category             *string `json:"category,omitempty"`
	Language             string  `json:"language"`
	GeoSpaceID           *string `json:"geo_space_id,omitempty"`
	ThumbnailURL         *string `json:"thumbnail_url,omitempty"`
	TotalEnrolled        int32   `json:"total_enrolled"`
	SubscriptionEligible bool    `json:"subscription_eligible"`
	InstructorID         *string `json:"instructor_id,omitempty"`
	InstructorName       *string `json:"instructor_name,omitempty"`
	AvgRating            float64 `json:"avg_rating"`
	ReviewCount          int32   `json:"review_count"`
}

// MarshalJSON emits the canonical CatalogTrack fields plus Learn-app aliases
// (rating_avg, rating_count, enrolled_count, subtitle) so the same DTO works
// for both the Main university page and the Learn marketplace surfaces.
func (c CatalogTrack) MarshalJSON() ([]byte, error) {
	// noMethod is a defined copy of CatalogTrack with the same fields/tags but
	// without this MarshalJSON method, which avoids infinite recursion.
	type noMethod CatalogTrack

	return json.Marshal(&struct {
		noMethod
		RatingAvg     float64 `json:"rating_avg,omitempty"`
		RatingCount   int32   `json:"rating_count,omitempty"`
		EnrolledCount int32   `json:"enrolled_count,omitempty"`
		Subtitle      *string `json:"subtitle,omitempty"`
	}{
		noMethod:      noMethod(c),
		RatingAvg:     c.AvgRating,
		RatingCount:   c.ReviewCount,
		EnrolledCount: c.TotalEnrolled,
		Subtitle:      c.Description,
	})
}

func catalogTrackFromRow(r gen.ListUniversityCatalogRow) CatalogTrack {
	c := CatalogTrack{
		ID: r.ID.String(), Title: r.Title, Slug: r.Slug, Description: r.Description,
		Difficulty: r.Difficulty, Level: r.Difficulty, Currency: r.Currency, Language: r.Language,
		TotalEnrolled: r.TotalEnrolled, SubscriptionEligible: r.SubscriptionEligible,
	}

	if r.InstructorID.Valid {
		s := r.InstructorID.String()
		c.InstructorID = &s
	}

	if r.PriceKobo != nil {
		v := *r.PriceKobo
		c.PriceKobo = &v
	}

	if r.Category != nil {
		c.Category = r.Category
	}

	if r.GeoSpaceID.Valid {
		s := r.GeoSpaceID.String()
		c.GeoSpaceID = &s
	}

	if u := media.URLFromParts(r.ThumbnailStorageBucket, r.ThumbnailStoragePath, media.ImageProxyOpts{}); u != nil {
		c.ThumbnailURL = u
	}

	if r.InstructorName != nil {
		c.InstructorName = r.InstructorName
	}

	if r.AvgRating.Valid {
		if f, err := r.AvgRating.Float64Value(); err == nil && f.Valid {
			c.AvgRating = f.Float64
		}
	}

	c.ReviewCount = r.ReviewCount

	return c
}

// CatalogFilter holds the marketplace catalog query filters.
type CatalogFilter struct {
	Category string
	Language string
	IsFree   bool
	Level    string      // difficulty filter (frontend sends "level" per ADR-060)
	Q        string      // title search (ILIKE)
	Sort     string      // popular|newest|rating (empty = popular by default)
	SpaceID  pgtype.UUID // optional geo_space_id filter (used by university Courses tab)
}

// ListCatalog returns marketplace tracks matching the filter.
func (s *Store) ListCatalog(ctx context.Context, f CatalogFilter, limit, offset int32) ([]gen.ListUniversityCatalogRow, error) {
	return s.read.ListUniversityCatalog(ctx, gen.ListUniversityCatalogParams{
		Category: f.Category, Language: f.Language,
		IsFree: f.IsFree, Level: f.Level, Q: f.Q, Sort: f.Sort, SpaceID: f.SpaceID,
		PageLimit: limit, PageOffset: offset,
	})
}

// CountCatalog returns the total count of marketplace tracks matching the filter.
func (s *Store) CountCatalog(ctx context.Context, f CatalogFilter) (int64, error) {
	return s.read.CountUniversityCatalog(ctx, gen.CountUniversityCatalogParams{
		Category: f.Category, Language: f.Language,
		IsFree: f.IsFree, Level: f.Level, Q: f.Q, SpaceID: f.SpaceID,
	})
}

// ListCatalogLanguages returns the distinct languages present in the live
// marketplace catalog — drives the language filter dropdown (Rule 16).
func (s *Store) ListCatalogLanguages(ctx context.Context) ([]string, error) {
	return s.read.ListUniversityCatalogLanguages(ctx)
}

// GetCatalogTrackBySlug returns a single marketplace track with instructor + rating.
func (s *Store) GetCatalogTrackBySlug(ctx context.Context, slug string) (gen.GetUniversityTrackBySlugRow, error) {
	return s.read.GetUniversityTrackBySlug(ctx,
		slug,
	)
}

// CatalogTrackDetail is the flat DTO returned by GET /learn/university/catalog/{slug}.
// It mirrors gen.GetUniversityTrackBySlugRow but adds a `level` JSON field as an
// alias for `difficulty`, matching the frontend TrackDetail.level contract
// (ADR-060). The sqlc-generated row type cannot carry extra fields, so we map
// it through this struct in the handler.
type CatalogTrackDetail struct {
	ID pgtype.UUID `json:"id"`

	Title                   string             `json:"title"`
	Slug                    string             `json:"slug"`
	Description             *string            `json:"description,omitempty"`
	Difficulty              string             `json:"difficulty"`
	Level                   string             `json:"level"`
	CoverMediaID            pgtype.UUID        `json:"cover_media_id,omitempty"`
	InstructorID            pgtype.UUID        `json:"instructor_id,omitempty"`
	PriceKobo               *int64             `json:"price_kobo,omitempty"`
	Currency                string             `json:"currency"`
	Category                *string            `json:"category,omitempty"`
	Language                string             `json:"language"`
	GeoSpaceID              *string            `json:"geo_space_id,omitempty"`
	TotalEnrolled           int32              `json:"total_enrolled"`
	SubscriptionEligible    bool               `json:"subscription_eligible"`
	IsPublished             bool               `json:"is_published"`
	IsPublishedToUniversity bool               `json:"is_published_to_university"`
	CreatedAt               pgtype.Timestamptz `json:"created_at"`
	UpdatedAt               pgtype.Timestamptz `json:"updated_at"`
	InstructorName          *string            `json:"instructor_name,omitempty"`
	InstructorHeadline      *string            `json:"instructor_headline,omitempty"`
	InstructorTier          *string            `json:"instructor_tier,omitempty"`
	AvgRating               pgtype.Numeric     `json:"avg_rating"`
	ReviewCount             int32              `json:"review_count"`
}

// CatalogTrackDetailFromRow maps the sqlc row to the response DTO, setting
// Level = Difficulty for the frontend contract.
func CatalogTrackDetailFromRow(r gen.GetUniversityTrackBySlugRow) CatalogTrackDetail {
	c := CatalogTrackDetail{
		ID: r.ID, Title: r.Title, Slug: r.Slug,
		Description: r.Description, Difficulty: r.Difficulty, Level: r.Difficulty,
		CoverMediaID: r.CoverMediaID, InstructorID: r.InstructorID,
		PriceKobo: r.PriceKobo, Currency: r.Currency, Category: r.Category,
		Language: r.Language, TotalEnrolled: r.TotalEnrolled,
		SubscriptionEligible: r.SubscriptionEligible,
		IsPublished:          r.IsPublished, IsPublishedToUniversity: r.IsPublishedToUniversity,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		InstructorName: r.InstructorName, InstructorHeadline: r.InstructorHeadline,
		InstructorTier: r.InstructorTier, AvgRating: r.AvgRating, ReviewCount: r.ReviewCount,
	}

	if r.GeoSpaceID.Valid {
		s := r.GeoSpaceID.String()
		c.GeoSpaceID = &s
	}

	return c
}

// GetTrackByID returns a track by ID (any publish state).
func (s *Store) GetTrackByID(ctx context.Context, id pgtype.UUID) (gen.GetUniversityTrackByIDRow, error) {
	return s.read.GetUniversityTrackByID(ctx,
		id,
	)
}

// CreateTrackInput holds the body for POST /learn/university/my/tracks.
type CreateTrackInput struct {
	Title                string
	Slug                 string
	Description          *string
	Difficulty           string
	CoverMediaID         *pgtype.UUID
	InstructorID         *pgtype.UUID
	PriceKobo            *int64
	Currency             *string
	Category             *string
	Language             *string
	GeoSpaceID           *pgtype.UUID
	SubscriptionEligible *bool
}

// CreateTrack creates a new marketplace track owned by the instructor.
func (s *Store) CreateTrack(ctx context.Context, in CreateTrackInput) (gen.CreateUniversityTrackRow, error) {
	params := gen.CreateUniversityTrackParams{
		Title: in.Title, Slug: in.Slug,
		Description: in.Description, Difficulty: in.Difficulty,
		Category: in.Category,
	}
	if in.CoverMediaID != nil {
		params.CoverMediaID = *in.CoverMediaID
	}

	if in.InstructorID != nil {
		params.InstructorID = *in.InstructorID
	}

	if in.PriceKobo != nil {
		v := *in.PriceKobo
		params.PriceKobo = &v
	}

	if in.Currency != nil {
		params.Currency = *in.Currency
	}

	if in.Language != nil {
		params.Language = *in.Language
	}

	if in.GeoSpaceID != nil {
		params.GeoSpaceID = *in.GeoSpaceID
	}

	if in.SubscriptionEligible != nil {
		params.SubscriptionEligible = in.SubscriptionEligible
	}

	return s.write.CreateUniversityTrack(ctx, params)
}

// UpdateTrackInput holds the body for PATCH /learn/university/my/tracks/{id}.
type UpdateTrackInput struct {
	Title                *string
	Description          *string
	Difficulty           *string
	CoverMediaID         *pgtype.UUID
	PriceKobo            *int64
	Currency             *string
	Category             *string
	Language             *string
	GeoSpaceID           *pgtype.UUID
	SubscriptionEligible *bool
}

// UpdateTrack updates a creator's own track.
func (s *Store) UpdateTrack(ctx context.Context, trackID, instructorID pgtype.UUID, in UpdateTrackInput) (gen.UpdateUniversityTrackRow, error) {
	params := gen.UpdateUniversityTrackParams{
		ID: trackID, InstructorID: instructorID,
	}
	if in.Title != nil {
		params.Title = *in.Title
	}

	if in.Description != nil {
		params.Description = in.Description
	}

	if in.Difficulty != nil {
		params.Difficulty = *in.Difficulty
	}

	if in.CoverMediaID != nil {
		params.CoverMediaID = *in.CoverMediaID
	}

	if in.PriceKobo != nil {
		v := *in.PriceKobo
		params.PriceKobo = &v
	}

	params.Currency = in.Currency
	params.Category = in.Category
	params.Language = in.Language

	if in.GeoSpaceID != nil {
		params.GeoSpaceID = *in.GeoSpaceID
	}

	if in.SubscriptionEligible != nil {
		params.SubscriptionEligible = in.SubscriptionEligible
	}

	return s.write.UpdateUniversityTrack(ctx, params)
}

// SubmitTrack marks a creator's track as submitted for marketplace approval.
func (s *Store) SubmitTrack(ctx context.Context, trackID, instructorID pgtype.UUID) (gen.SubmitUniversityTrackRow, error) {
	return s.write.SubmitUniversityTrack(ctx, gen.SubmitUniversityTrackParams{
		ID: trackID, InstructorID: instructorID,
	})
}

// ListTracksByInstructor returns all tracks owned by an instructor.
func (s *Store) ListTracksByInstructor(ctx context.Context, instructorID pgtype.UUID) ([]gen.ListUniversityTracksByInstructorRow, error) {
	return s.read.ListUniversityTracksByInstructor(ctx,
		instructorID,
	)
}

// ListPendingTracks returns tracks awaiting marketplace approval.
// spaceID, when Valid, restricts to tracks targeted at that space.
func (s *Store) ListPendingTracks(ctx context.Context, spaceID pgtype.UUID, limit, offset int32) ([]gen.ListPendingUniversityTracksRow, error) {
	return s.read.ListPendingUniversityTracks(ctx, gen.ListPendingUniversityTracksParams{
		SpaceID: spaceID, PageLimit: limit, PageOffset: offset,
	})
}

// CountPendingTracks returns the total count of tracks awaiting marketplace
// approval (mirrors ListPendingTracks). Used by the admin hub and the
// pending-tracks list meta envelope.
func (s *Store) CountPendingTracks(ctx context.Context, spaceID pgtype.UUID) (int64, error) {
	return s.read.CountPendingUniversityTracks(ctx,
		spaceID,
	)
}

// ApproveTrack publishes a submitted track to the marketplace.
func (s *Store) ApproveTrack(ctx context.Context, trackID pgtype.UUID) (gen.ApproveUniversityTrackRow, error) {
	return s.write.ApproveUniversityTrack(ctx,
		trackID,
	)
}

// IncrementTotalEnrolled bumps the cached enrollment counter.
func (s *Store) IncrementTotalEnrolled(ctx context.Context, trackID pgtype.UUID) error {
	return s.write.IncrementTrackTotalEnrolled(ctx,
		trackID,
	)
}

// ---------------------------------------------------------------------------
// Enrollment + revenue split queries
// ---------------------------------------------------------------------------

// UpsertEnrollment creates or updates an enrollment, linking the payment.
func (s *Store) UpsertEnrollment(ctx context.Context, userID, trackID pgtype.UUID, accessType string, paymentID *pgtype.UUID) (gen.Enrollment, error) {
	params := gen.UpsertEnrollmentWithPaymentParams{
		UserID: userID, TrackID: trackID, AccessType: accessType,
	}
	if paymentID != nil {
		params.PaymentID = *paymentID
	}

	return s.write.UpsertEnrollmentWithPayment(ctx, params)
}

// HasEnrollment reports whether a user is enrolled in a track.
func (s *Store) HasEnrollment(ctx context.Context, userID, trackID pgtype.UUID) (bool, error) {
	return s.read.HasEnrollmentForTrack(ctx, gen.HasEnrollmentForTrackParams{
		UserID: userID, TrackID: trackID,
	})
}

// HasSplitForPayment is the idempotency guard for enrollment processing.
func (s *Store) HasSplitForPayment(ctx context.Context, paymentID pgtype.UUID) (bool, error) {
	return s.read.HasRevenueSplitForPayment(ctx,
		paymentID,
	)
}

// InsertRevenueSplit records an immutable per-event revenue split.
func (s *Store) InsertRevenueSplit(ctx context.Context, params gen.InsertRevenueSplitParams) (gen.RevenueSplit, error) {
	return s.write.InsertRevenueSplit(ctx, params)
}

// ListOpenSplits returns revenue splits not yet rolled into a payout.
func (s *Store) ListOpenSplits(ctx context.Context, instructorID pgtype.UUID) ([]gen.RevenueSplit, error) {
	return s.read.ListOpenRevenueSplitsByInstructor(ctx,
		instructorID,
	)
}

// ListSplitsByInstructor returns paginated splits for an instructor.
func (s *Store) ListSplitsByInstructor(ctx context.Context, instructorID pgtype.UUID, limit, offset int32) ([]gen.RevenueSplit, error) {
	return s.read.ListRevenueSplitsByInstructor(ctx, gen.ListRevenueSplitsByInstructorParams{
		InstructorID: instructorID, PageLimit: limit, PageOffset: offset,
	})
}

// GetRevenueSplitByID returns a single immutable revenue-split record by ID,
// platform-scoped. Used by the admin audit viewer (ADR-063 §9).
func (s *Store) GetRevenueSplitByID(ctx context.Context, id pgtype.UUID) (gen.RevenueSplit, error) {
	return s.read.GetRevenueSplitByID(ctx,
		id,
	)
}

// ListRevenueSplitsFilter holds the optional filters for the admin audit
// viewer list endpoint (ADR-063 §9). A nil/zero value on any field means
// "no filter on this column".
type ListRevenueSplitsFilter struct {
	InstructorID pgtype.UUID        // empty = all instructors
	TrackID      pgtype.UUID        // empty = all tracks
	Source       string             // "" = both purchase and pro_stream
	PayoutID     pgtype.UUID        // empty = both open and stamped splits
	FromTs       pgtype.Timestamptz // zero = no lower bound
	ToTs         pgtype.Timestamptz // zero = no upper bound
	SpaceID      pgtype.UUID        // empty = all spaces (ADR-063 per-university view)
}

// ListRevenueSplits returns a platform-wide paginated list of immutable
// revenue-split audit records with optional filters (ADR-063 §9).
func (s *Store) ListRevenueSplits(ctx context.Context, f ListRevenueSplitsFilter, limit, offset int32) ([]gen.ListRevenueSplitsRow, error) {
	return s.read.ListRevenueSplits(ctx, gen.ListRevenueSplitsParams{

		InstructorID: f.InstructorID,
		TrackID:      f.TrackID,
		Source:       f.Source,
		PayoutID:     f.PayoutID,
		FromTs:       f.FromTs,
		ToTs:         f.ToTs,
		SpaceID:      f.SpaceID,
		PageLimit:    limit,
		PageOffset:   offset,
	})
}

// CountRevenueSplits returns the total row count for the same filter set as
// ListRevenueSplits, for the pagination meta envelope (ADR-063 §9).
func (s *Store) CountRevenueSplits(ctx context.Context, f ListRevenueSplitsFilter) (int64, error) {
	return s.read.CountRevenueSplits(ctx, gen.CountRevenueSplitsParams{

		InstructorID: f.InstructorID,
		TrackID:      f.TrackID,
		Source:       f.Source,
		PayoutID:     f.PayoutID,
		FromTs:       f.FromTs,
		ToTs:         f.ToTs,
		SpaceID:      f.SpaceID,
	})
}

// SumOpenSplits returns gross/creator/platform totals for open splits.
func (s *Store) SumOpenSplits(ctx context.Context, instructorID pgtype.UUID) (gen.SumOpenSplitsByInstructorRow, error) {
	return s.read.SumOpenSplitsByInstructor(ctx,
		instructorID,
	)
}

// SumOpenSplitsByAllInstructors returns gross/creator/platform totals for
// open splits grouped by instructor_id in a single query. Use this instead
// of looping SumOpenSplits per instructor to avoid an N+1 (AUDIT-BE-DB-005).
func (s *Store) SumOpenSplitsByAllInstructors(ctx context.Context) ([]gen.SumOpenSplitsByAllInstructorsRow, error) {
	return s.read.SumOpenSplitsByAllInstructors(ctx)
}

// StampSplitsWithPayout marks open splits in a period as rolled into a payout.
// The period boundaries are passed as pgtype.Date (from the payout row) and
// converted to timestamptz boundaries for the created_at range filter.
func (s *Store) StampSplitsWithPayout(ctx context.Context, instructorID, payoutID pgtype.UUID, periodStart, periodEnd pgtype.Date) (int64, error) {
	return s.write.StampRevenueSplitsWithPayout(ctx, gen.StampRevenueSplitsWithPayoutParams{
		InstructorID: instructorID, PayoutID: payoutID,
		PeriodStart: pgtype.Timestamptz{Time: periodStart.Time, Valid: true},
		PeriodEnd:   pgtype.Timestamptz{Time: periodEnd.Time, Valid: true},
	})
}

// ---------------------------------------------------------------------------
// Payout queries
// ---------------------------------------------------------------------------

// CreatePayout creates a payout row for an instructor/period (idempotent).
func (s *Store) CreatePayout(ctx context.Context, params gen.CreateInstructorPayoutParams) (gen.InstructorPayout, error) {
	return s.write.CreateInstructorPayout(ctx, params)
}

// GetPayout returns a payout by ID.
func (s *Store) GetPayout(ctx context.Context, id pgtype.UUID) (gen.InstructorPayout, error) {
	return s.read.GetInstructorPayoutByID(ctx,
		id,
	)
}

// ListPayoutsByInstructor returns an instructor's payout history.
func (s *Store) ListPayoutsByInstructor(ctx context.Context, instructorID pgtype.UUID, limit, offset int32) ([]gen.InstructorPayout, error) {
	return s.read.ListInstructorPayoutsByInstructor(ctx, gen.ListInstructorPayoutsByInstructorParams{
		InstructorID: instructorID, PageLimit: limit, PageOffset: offset,
	})
}

// ListPayoutsByStatus returns the payout queue (optionally filtered by status).
// spaceID, when Valid, restricts to payouts for instructors with a track
// targeted at that space.
func (s *Store) ListPayoutsByStatus(ctx context.Context, spaceID pgtype.UUID, status string, limit, offset int32) ([]gen.ListInstructorPayoutsByStatusRow, error) {
	return s.read.ListInstructorPayoutsByStatus(ctx, gen.ListInstructorPayoutsByStatusParams{
		Status: status, SpaceID: spaceID, PageLimit: limit, PageOffset: offset,
	})
}

// CountPayoutsByStatus returns the total count of payouts matching the
// status/space filter (mirrors ListPayoutsByStatus). Used by the admin hub
// and the payout-queue list meta envelope.
func (s *Store) CountPayoutsByStatus(ctx context.Context, spaceID pgtype.UUID, status string) (int64, error) {
	return s.read.CountInstructorPayoutsByStatus(ctx, gen.CountInstructorPayoutsByStatusParams{
		Status: status, SpaceID: spaceID,
	})
}

// UpdatePayoutStatus transitions a payout's status.
func (s *Store) UpdatePayoutStatus(ctx context.Context, id pgtype.UUID, status string, ref, failureReason *string) (gen.InstructorPayout, error) {
	return s.write.UpdatePayoutStatus(ctx, gen.UpdatePayoutStatusParams{
		ID: id, Status: status,
		PayoutReference: ref, FailureReason: failureReason,
	})
}

// EarningsSummary returns open/paid totals for an instructor.
func (s *Store) EarningsSummary(ctx context.Context, instructorID pgtype.UUID) (gen.GetInstructorEarningsSummaryRow, error) {
	return s.read.GetInstructorEarningsSummary(ctx,
		instructorID,
	)
}

// DashboardSummary returns aggregate stats (total_enrolled, total_revenue,
// courses_count) for the instructor dashboard (BUG-003).
func (s *Store) DashboardSummary(ctx context.Context, instructorID pgtype.UUID) (gen.GetInstructorDashboardSummaryRow, error) {
	return s.read.GetInstructorDashboardSummary(ctx,
		instructorID,
	)
}

// RevenueSeries returns the daily creator revenue series for an instructor (BUG-005).
func (s *Store) RevenueSeries(ctx context.Context, instructorID pgtype.UUID) ([]gen.GetInstructorRevenueSeriesRow, error) {
	return s.read.GetInstructorRevenueSeries(ctx,
		instructorID,
	)
}

// TrackRevenueSeries returns the daily revenue series for a single track (BUG-007).
func (s *Store) TrackRevenueSeries(ctx context.Context, trackID pgtype.UUID) ([]gen.GetTrackRevenueSeriesRow, error) {
	return s.read.GetTrackRevenueSeries(ctx,
		trackID,
	)
}

// TrackEnrollmentSeries returns the daily enrollment series for a track (BUG-007).
func (s *Store) TrackEnrollmentSeries(ctx context.Context, trackID pgtype.UUID) ([]gen.GetTrackEnrollmentSeriesRow, error) {
	return s.read.GetTrackEnrollmentSeries(ctx,
		trackID,
	)
}

// TrackCompletionRate returns total/completed enrollment counts for a track (BUG-007).
func (s *Store) TrackCompletionRate(ctx context.Context, trackID pgtype.UUID) (gen.GetTrackCompletionRateRow, error) {
	return s.read.GetTrackCompletionRate(ctx,
		trackID,
	)
}

// HasProStreamSplit checks whether a pro_stream split already exists for the
// given instructor/track/user combination (BUG-008 idempotency guard).
func (s *Store) HasProStreamSplit(ctx context.Context, instructorID, trackID, userID pgtype.UUID) (bool, error) {
	return s.read.HasProStreamSplitForPeriod(ctx, gen.HasProStreamSplitForPeriodParams{
		InstructorID: instructorID, TrackID: trackID, UserID: userID,
	})
}

// ListAllTracks returns all marketplace tracks (admin view, BUG-002).
func (s *Store) ListAllTracks(ctx context.Context, spaceID pgtype.UUID, status string, limit, offset int32) ([]gen.ListAllUniversityTracksRow, error) {
	return s.read.ListAllUniversityTracks(ctx, gen.ListAllUniversityTracksParams{
		Status: status, SpaceID: spaceID, PageLimit: limit, PageOffset: offset,
	})
}

// CountAllTracks returns the total count of marketplace tracks (BUG-002).
// spaceID, when Valid, restricts to tracks targeted at that space.
func (s *Store) CountAllTracks(ctx context.Context, spaceID pgtype.UUID, status string) (int64, error) {
	return s.read.CountAllUniversityTracks(ctx, gen.CountAllUniversityTracksParams{
		Status: status, SpaceID: spaceID,
	})
}

// RejectTrack rejects a submitted track with an optional reason (BUG-002, BUG-004).
func (s *Store) RejectTrack(ctx context.Context, trackID pgtype.UUID, reason *string) (gen.RejectUniversityTrackRow, error) {
	return s.write.RejectUniversityTrack(ctx, gen.RejectUniversityTrackParams{
		ID: trackID, RejectionReason: reason,
	})
}

// UpdateInstructorAdmin updates an instructor's tier and/or commission override (BUG-002).
func (s *Store) UpdateInstructorAdmin(ctx context.Context, id pgtype.UUID, tier *string, commissionOverride *pgtype.Numeric) (gen.InstructorProfile, error) {
	params := gen.UpdateInstructorProfileAdminParams{
		ID: id, Tier: tier,
	}
	if commissionOverride != nil {
		params.CommissionPctOverride = *commissionOverride
	}

	return s.write.UpdateInstructorProfileAdmin(ctx, params)
}

// ListPublishedTracksByInstructor returns only live marketplace tracks for
// an instructor, filtered at the DB level (BUG-017).
func (s *Store) ListPublishedTracksByInstructor(ctx context.Context, instructorID pgtype.UUID) ([]gen.ListPublishedTracksByInstructorRow, error) {
	return s.read.ListPublishedTracksByInstructor(ctx,
		instructorID,
	)
}

// SetTrackFeatured features or unfeatures a marketplace track (BUG-012).
func (s *Store) SetTrackFeatured(ctx context.Context, trackID pgtype.UUID, featured bool) (gen.SetUniversityTrackFeaturedRow, error) {
	return s.write.SetUniversityTrackFeatured(ctx, gen.SetUniversityTrackFeaturedParams{
		ID: trackID, Featured: featured,
	})
}

// ---------------------------------------------------------------------------
// Track categories (ADR-063 §9.1)
// Ref: japaconnect-engineering/sections/36-learning-platform.md (Track categories)
// ---------------------------------------------------------------------------

// CreateTrackCategory inserts a new university_track_categories row.
func (s *Store) CreateTrackCategory(ctx context.Context, slug, name string, sortOrder int32, isActive bool) (gen.UniversityTrackCategory, error) {
	return s.write.CreateUniversityTrackCategory(ctx, gen.CreateUniversityTrackCategoryParams{
		Slug: slug, Name: name, SortOrder: sortOrder, IsActive: isActive,
	})
}

// GetTrackCategory fetches a single category by ID (platform-scoped).
func (s *Store) GetTrackCategory(ctx context.Context, id pgtype.UUID) (gen.UniversityTrackCategory, error) {
	return s.read.GetUniversityTrackCategory(ctx,
		id,
	)
}

// ListTrackCategories returns paginated categories with optional active/search filters.
func (s *Store) ListTrackCategories(ctx context.Context, active, search string, limit, offset int32) ([]gen.UniversityTrackCategory, error) {
	return s.read.ListUniversityTrackCategories(ctx, gen.ListUniversityTrackCategoriesParams{
		Active: active, Search: search, PageLimit: limit, PageOffset: offset,
	})
}

// CountTrackCategories returns the total count for the same filter set.
func (s *Store) CountTrackCategories(ctx context.Context, active, search string) (int64, error) {
	return s.read.CountUniversityTrackCategories(ctx, gen.CountUniversityTrackCategoriesParams{
		Active: active, Search: search,
	})
}

// ListActiveTrackCategories returns only active categories for the public catalog dropdown.
func (s *Store) ListActiveTrackCategories(ctx context.Context) ([]gen.ListActiveUniversityTrackCategoriesRow, error) {
	return s.read.ListActiveUniversityTrackCategories(ctx)
}

// UpdateTrackCategory partially updates a category (nil fields are preserved via COALESCE).
func (s *Store) UpdateTrackCategory(ctx context.Context, id pgtype.UUID, name, slug *string, sortOrder *int32, isActive *bool) (gen.UniversityTrackCategory, error) {
	return s.write.UpdateUniversityTrackCategory(ctx, gen.UpdateUniversityTrackCategoryParams{
		ID: id, Name: name, Slug: slug, SortOrder: sortOrder, IsActive: isActive,
	})
}

// DeleteTrackCategory removes a category row (caller must check CountTracksByCategory first).
func (s *Store) DeleteTrackCategory(ctx context.Context, id pgtype.UUID) error {
	return s.write.DeleteUniversityTrackCategory(ctx,
		id,
	)
}

// CountTracksByCategory returns the number of learning_tracks whose category matches
// the given slug. Used by the delete handler to block removal when tracks reference it.
func (s *Store) CountTracksByCategory(ctx context.Context, category string) (int64, error) {
	return s.read.CountTracksByCategory(ctx,
		&category,
	)
}
