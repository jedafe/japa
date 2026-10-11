package academy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	contentpkg "github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/media"
)

// Store provides direct pgx CRUD for learning tracks, modules, lessons,
// progress records, and certificates. All queries are platform-scoped via
// the column and use the shared read/write connection pools.
type Store struct {
	read  *gen.Queries
	write *gen.Queries
	db    *db.DB
}

// NewStore creates an academy store bound to the provided database pools.
func NewStore(database *db.DB) *Store {
	return &Store{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
		db:    database,
	}
}

// ---------------------------------------------------------------------------
// Track operations
// ---------------------------------------------------------------------------

// Track represents track.
type Track struct {
	ID          pgtype.UUID `json:"id"`
	Title       string      `json:"title"`
	Slug        string      `json:"slug"`
	Description *string     `json:"description,omitempty"`
	Difficulty  string      `json:"difficulty"`
	/** Level is an alias for Difficulty, matching the frontend TrackSummary.level
	 * contract (ADR-060). Always equal to Difficulty. */
	Level        string       `json:"level"`
	CoverMediaID *pgtype.UUID `json:"cover_media_id,omitempty"`
	IsPublished  bool         `json:"is_published"`
	SortOrder    int32        `json:"sort_order"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	// Marketplace columns (migration 000166, ADR-063).
	InstructorID         *pgtype.UUID `json:"instructor_id,omitempty"`
	PriceKobo            *int64       `json:"price_kobo,omitempty"`
	Currency             string       `json:"currency,omitempty"`
	IsPublishedToAcademy bool         `json:"is_published_to_academy,omitempty"`
	Category             *string      `json:"category,omitempty"`
	Language             string       `json:"language,omitempty"`
	TotalEnrolled        int32        `json:"total_enrolled,omitempty"`
	SubscriptionEligible bool         `json:"subscription_eligible,omitempty"`
}

// CreateTrackInput represents create track input.
type CreateTrackInput struct {
	Title                string      `json:"title"`
	Slug                 string      `json:"slug"`
	Description          *string     `json:"description,omitempty"`
	Difficulty           string      `json:"difficulty"`
	CoverMediaID         pgtype.UUID `json:"cover_media_id"`
	IsPublished          bool        `json:"is_published"`
	SortOrder            int32       `json:"sort_order"`
	Category             *string     `json:"category,omitempty"`
	SubscriptionEligible bool        `json:"subscription_eligible,omitempty"`
}

// UpdateTrackInput represents update track input. All fields are optional —
// a nil pointer leaves the column untouched so partial PATCH bodies cannot
// wipe values they never set (BUG: previously required params zeroed fields).
type UpdateTrackInput struct {
	Title                *string     `json:"title,omitempty"`
	Description          *string     `json:"description,omitempty"`
	Difficulty           *string     `json:"difficulty,omitempty"`
	CoverMediaID         pgtype.UUID `json:"cover_media_id"`
	IsPublished          *bool       `json:"is_published,omitempty"`
	SortOrder            *int32      `json:"sort_order,omitempty"`
	Category             *string     `json:"category,omitempty"`
	SubscriptionEligible *bool       `json:"subscription_eligible,omitempty"`
}

// trackFromFields builds a Track DTO from the base academy_tracks columns
// plus the marketplace columns added by migration 000166 (ADR-063). Migration
// 000166 caused sqlc to emit per-query row types instead of reusing the
// gen.AcademyTrack model; all these row types share the same columns, so
// callers extract the fields and pass them here.
func trackFromFields(
	id pgtype.UUID,
	title, slug string,
	description *string,
	difficulty string,
	coverMediaID pgtype.UUID,
	isPublished bool,
	sortOrder int32,
	createdAt, updatedAt pgtype.Timestamptz,
	instructorID pgtype.UUID,
	priceKobo *int64,
	currency string,
	isPublishedToAcademy bool,
	category *string,
	language string,
	totalEnrolled int32,
	subscriptionEligible bool,
) Track {
	var cmID *pgtype.UUID
	if coverMediaID.Valid {
		cmID = &coverMediaID
	}

	var instrID *pgtype.UUID
	if instructorID.Valid {
		instrID = &instructorID
	}

	return Track{
		ID: id,

		Title:                title,
		Slug:                 slug,
		Description:          description,
		Difficulty:           difficulty,
		Level:                difficulty,
		CoverMediaID:         cmID,
		IsPublished:          isPublished,
		SortOrder:            sortOrder,
		CreatedAt:            createdAt.Time,
		UpdatedAt:            updatedAt.Time,
		InstructorID:         instrID,
		PriceKobo:            priceKobo,
		Currency:             currency,
		IsPublishedToAcademy: isPublishedToAcademy,
		Category:             category,
		Language:             language,
		TotalEnrolled:        totalEnrolled,
		SubscriptionEligible: subscriptionEligible,
	}
}

// ListPublishedTracks lists all published tracks for a platform.
func (s *Store) ListPublishedTracks(ctx context.Context) ([]Track, error) {
	rows, err := s.read.ListPublishedAcademyTracks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list published tracks: %w", err)
	}

	tracks := make([]Track, 0, len(rows))

	for _, r := range rows {
		tracks = append(tracks, trackFromFields(r.ID, r.Title, r.Slug, r.Description, r.Difficulty, r.CoverMediaID, r.IsPublished, r.SortOrder, r.CreatedAt, r.UpdatedAt, r.InstructorID, r.PriceKobo, r.Currency, r.IsPublishedToAcademy, r.Category, r.Language, r.TotalEnrolled, r.SubscriptionEligible))
	}

	return tracks, nil
}

// SearchPublishedTracks searches published tracks by title or slug, platform-scoped.
func (s *Store) SearchPublishedTracks(ctx context.Context, query string, limit int32) ([]Track, error) {
	const q = `
SELECT id, title, slug, description, difficulty, cover_media_id,
       is_published, sort_order, created_at, updated_at,
       instructor_id, price_kobo, currency, is_published_to_academy,
       category, language, total_enrolled, subscription_eligible
FROM academy_tracks
WHERE is_published = true AND (title ILIKE $1 OR slug ILIKE $1)
ORDER BY sort_order ASC, created_at DESC
LIMIT $2
`

	rows, err := s.db.Reader.Query(ctx, q, "%"+query+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("search published tracks: %w", err)
	}

	defer rows.Close()

	var tracks []Track

	for rows.Next() {
		var r struct {
			ID                   pgtype.UUID
			Title                string
			Slug                 string
			Description          *string
			Difficulty           string
			CoverMediaID         pgtype.UUID
			IsPublished          bool
			SortOrder            int32
			CreatedAt            pgtype.Timestamptz
			UpdatedAt            pgtype.Timestamptz
			InstructorID         pgtype.UUID
			PriceKobo            *int64
			Currency             string
			IsPublishedToAcademy bool
			Category             *string
			Language             string
			TotalEnrolled        int32
			SubscriptionEligible bool
		}

		if err := rows.Scan(
			&r.ID, &r.Title, &r.Slug, &r.Description, &r.Difficulty, &r.CoverMediaID,
			&r.IsPublished, &r.SortOrder, &r.CreatedAt, &r.UpdatedAt, &r.InstructorID, &r.PriceKobo,
			&r.Currency, &r.IsPublishedToAcademy, &r.Category, &r.Language, &r.TotalEnrolled, &r.SubscriptionEligible,
		); err != nil {
			return nil, fmt.Errorf("scan learning track: %w", err)
		}

		tracks = append(tracks, trackFromFields(
			r.ID, r.Title, r.Slug, r.Description, r.Difficulty, r.CoverMediaID,
			r.IsPublished, r.SortOrder, r.CreatedAt, r.UpdatedAt, r.InstructorID, r.PriceKobo,
			r.Currency, r.IsPublishedToAcademy, r.Category, r.Language, r.TotalEnrolled, r.SubscriptionEligible,
		))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate learning tracks: %w", err)
	}

	return tracks, nil
}

// GetTrackBySlug retrieves a track by its slug.
func (s *Store) GetTrackBySlug(ctx context.Context, slug string) (*Track, error) {
	r, err := s.read.GetAcademyTrackBySlug(ctx,

		slug,
	)
	if err != nil {
		return nil, fmt.Errorf("get track by slug: %w", err)
	}

	t := trackFromFields(r.ID, r.Title, r.Slug, r.Description, r.Difficulty, r.CoverMediaID, r.IsPublished, r.SortOrder, r.CreatedAt, r.UpdatedAt, r.InstructorID, r.PriceKobo, r.Currency, r.IsPublishedToAcademy, r.Category, r.Language, r.TotalEnrolled, r.SubscriptionEligible)

	return &t, nil
}

// GetTrackByID retrieves a track by its ID.
func (s *Store) GetTrackByID(ctx context.Context, id pgtype.UUID) (*Track, error) {
	r, err := s.read.GetAcademyTrackByID(ctx,

		id,
	)
	if err != nil {
		return nil, fmt.Errorf("get track by id: %w", err)
	}

	t := trackFromFields(r.ID, r.Title, r.Slug, r.Description, r.Difficulty, r.CoverMediaID, r.IsPublished, r.SortOrder, r.CreatedAt, r.UpdatedAt, r.InstructorID, r.PriceKobo, r.Currency, r.IsPublishedToAcademy, r.Category, r.Language, r.TotalEnrolled, r.SubscriptionEligible)

	return &t, nil
}

// CreateTrack creates a new learning track.
func (s *Store) CreateTrack(ctx context.Context, input CreateTrackInput) (*Track, error) {
	r, err := s.write.CreateAcademyTrack(ctx, gen.CreateAcademyTrackParams{

		Title:                input.Title,
		Slug:                 input.Slug,
		Description:          input.Description,
		Difficulty:           input.Difficulty,
		CoverMediaID:         input.CoverMediaID,
		IsPublished:          input.IsPublished,
		SortOrder:            input.SortOrder,
		Category:             input.Category,
		SubscriptionEligible: input.SubscriptionEligible,
	})
	if err != nil {
		return nil, fmt.Errorf("create track: %w", err)
	}

	t := trackFromFields(r.ID, r.Title, r.Slug, r.Description, r.Difficulty, r.CoverMediaID, r.IsPublished, r.SortOrder, r.CreatedAt, r.UpdatedAt, r.InstructorID, r.PriceKobo, r.Currency, r.IsPublishedToAcademy, r.Category, r.Language, r.TotalEnrolled, r.SubscriptionEligible)

	return &t, nil
}

// UpdateTrack updates an existing learning track.
func (s *Store) UpdateTrack(ctx context.Context, id pgtype.UUID, input UpdateTrackInput) (*Track, error) {
	r, err := s.write.UpdateAcademyTrack(ctx, gen.UpdateAcademyTrackParams{

		ID:                   id,
		Title:                input.Title,
		Description:          input.Description,
		Difficulty:           input.Difficulty,
		CoverMediaID:         input.CoverMediaID,
		IsPublished:          input.IsPublished,
		SortOrder:            input.SortOrder,
		Category:             input.Category,
		SubscriptionEligible: input.SubscriptionEligible,
	})
	if err != nil {
		return nil, fmt.Errorf("update track: %w", err)
	}

	t := trackFromFields(r.ID, r.Title, r.Slug, r.Description, r.Difficulty, r.CoverMediaID, r.IsPublished, r.SortOrder, r.CreatedAt, r.UpdatedAt, r.InstructorID, r.PriceKobo, r.Currency, r.IsPublishedToAcademy, r.Category, r.Language, r.TotalEnrolled, r.SubscriptionEligible)

	return &t, nil
}

// DeleteTrack soft-deletes a learning track.
func (s *Store) DeleteTrack(ctx context.Context, id pgtype.UUID) error {
	tag, err := s.db.Writer.Exec(ctx, `
		DELETE FROM academy_tracks WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("delete track: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// PublishTrack sets the published status of a learning track.
func (s *Store) PublishTrack(ctx context.Context, id pgtype.UUID, isPublished bool) error {
	tag, err := s.db.Writer.Exec(ctx, `
		UPDATE academy_tracks
		SET is_published = $1, updated_at = now()
		WHERE id = $2
	`, isPublished, id)
	if err != nil {
		return fmt.Errorf("publish track: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// ListAllTracks lists all tracks including drafts.
func (s *Store) ListAllTracks(ctx context.Context) ([]Track, error) {
	rows, err := s.read.ListAllAcademyTracks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all tracks: %w", err)
	}

	tracks := make([]Track, 0, len(rows))

	for _, r := range rows {
		tracks = append(tracks, trackFromFields(r.ID, r.Title, r.Slug, r.Description, r.Difficulty, r.CoverMediaID, r.IsPublished, r.SortOrder, r.CreatedAt, r.UpdatedAt, r.InstructorID, r.PriceKobo, r.Currency, r.IsPublishedToAcademy, r.Category, r.Language, r.TotalEnrolled, r.SubscriptionEligible))
	}

	return tracks, nil
}

// ---------------------------------------------------------------------------
// Module operations
// ---------------------------------------------------------------------------

// Module represents module.
type Module struct {
	ID          pgtype.UUID `json:"id"`
	TrackID     pgtype.UUID `json:"track_id"`
	Title       string      `json:"title"`
	Slug        string      `json:"slug"`
	Description *string     `json:"description,omitempty"`
	SortOrder   int32       `json:"sort_order"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// LessonSummary is the lightweight lesson view nested inside ModuleWithLessons
// for the track-detail response (GET /academy/tracks/:slug). Mirrors the frontend
// LessonSummary type (ADR-060). Content is omitted — only metadata.
type LessonSummary struct {
	ID              pgtype.UUID `json:"id"`
	Slug            string      `json:"slug"`
	Title           string      `json:"title"`
	LessonType      string      `json:"lesson_type"`
	DurationMinutes *int32      `json:"duration_minutes,omitempty"`
	SortOrder       int32       `json:"sort_order"`
	IsPreview       bool        `json:"is_preview"`
	// DurationLabel/DurationsISO are derived from DurationMinutes so the
	// frontend curriculum accordion (ADR-060) can render per-lesson durations
	// via the same {#if lesson.duration_label ?? lesson.duration_iso} guard
	// used for the track-level duration. Empty when duration_minutes is null/0.
	DurationLabel string `json:"duration_label,omitempty"`
	DurationISO   string `json:"duration_iso,omitempty"`
}

// ModuleWithLessons is a module with its nested lessons, used in the track-detail
// response (GET /academy/tracks/:slug). Matches the frontend ModuleSummary contract
// (ADR-060): modules carry their lessons so the curriculum accordion renders
// from a single fetch.
type ModuleWithLessons struct {
	ID          pgtype.UUID     `json:"id"`
	Slug        string          `json:"slug"`
	Title       string          `json:"title"`
	Description *string         `json:"description,omitempty"`
	SortOrder   int32           `json:"sort_order"`
	Lessons     []LessonSummary `json:"lessons"`
}

// InstructorRef is the instructor view embedded in TrackDetail for the hero
// stat and the instructor card (ADR-060). Mirrors the frontend InstructorRef
// type. Populated from the joined academy_instructor_profiles + users + user_profiles
// row in GetAcademyTrackDetailBySlug; nil for platform/staff-authored courses.
type InstructorRef struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"displayName"`
	AvatarURL   *string `json:"avatar_url,omitempty"`
	Bio         *string `json:"bio,omitempty"`
}

// TrackDetail is the flat DTO returned by GET /academy/tracks/:slug. Per Section 36
// the route returns "track detail: modules, lessons, instructor" in one response.
// The frontend TrackDetail type (ADR-060) expects a flat object with a `modules`
// array where each module carries its `lessons` — NOT the legacy `{track, modules}`
// envelope. `Level` aliases `Difficulty` for the frontend contract.
// Fields not yet backed by DB columns (subtitle, overview_html, objectives,
// prerequisites) are included as optional JSON keys so the frontend contract is
// stable; they serialize as null/omitted until the corresponding migrations land.
type TrackDetail struct {
	ID           pgtype.UUID         `json:"id"`
	Title        string              `json:"title"`
	Slug         string              `json:"slug"`
	Description  *string             `json:"description,omitempty"`
	Difficulty   string              `json:"difficulty"`
	Level        string              `json:"level"`
	CoverMediaID *pgtype.UUID        `json:"cover_media_id,omitempty"`
	IsPublished  bool                `json:"is_published"`
	SortOrder    int32               `json:"sort_order"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
	Modules      []ModuleWithLessons `json:"modules"`
	// Marketplace / enrollment fields (migration 000166).
	PriceKobo            *int64  `json:"price_kobo,omitempty"`
	Currency             string  `json:"currency,omitempty"`
	EnrolledCount        int32   `json:"enrolled_count,omitempty"`
	IsPublishedToAcademy bool    `json:"is_published_to_academy,omitempty"`
	Category             *string `json:"category,omitempty"`
	Language             string  `json:"language,omitempty"`
	// Duration computed from the sum of lesson duration_minutes (ADR-060).
	DurationLabel string `json:"duration_label,omitempty"`
	DurationISO   string `json:"duration_iso,omitempty"`
	// Optional fields the frontend reads with {#if} guards — omitted from JSON
	// when absent so they're undefined on the client (falsy).
	ThumbnailURL         *string        `json:"thumbnail_url,omitempty"`
	Subtitle             *string        `json:"subtitle,omitempty"`
	InstructorName       *string        `json:"instructor_name,omitempty"`
	Instructor           *InstructorRef `json:"instructor,omitempty"`
	SubscriptionEligible bool           `json:"subscription_eligible"`
}

// CreateModuleInput represents create module input.
type CreateModuleInput struct {
	Title       string  `json:"title"`
	Slug        string  `json:"slug"`
	Description *string `json:"description,omitempty"`
	SortOrder   int32   `json:"sort_order"`
}

// UpdateModuleInput represents update module input. Nil pointers leave the
// column untouched (partial-update safe).
type UpdateModuleInput struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	SortOrder   *int32  `json:"sort_order,omitempty"`
}

func moduleFromRow(r gen.AcademyModule) Module {
	return Module{
		ID: r.ID,

		TrackID:     r.TrackID,
		Title:       r.Title,
		Slug:        r.Slug,
		Description: r.Description,
		SortOrder:   r.SortOrder,
		CreatedAt:   r.CreatedAt.Time,
		UpdatedAt:   r.UpdatedAt.Time,
	}
}

// ListModulesByTrack lists all modules for a track.
func (s *Store) ListModulesByTrack(ctx context.Context, trackID pgtype.UUID) ([]Module, error) {
	rows, err := s.read.ListAcademyModulesByTrack(ctx,

		trackID,
	)
	if err != nil {
		return nil, fmt.Errorf("list modules by track: %w", err)
	}

	modules := make([]Module, 0, len(rows))

	for _, r := range rows {
		modules = append(modules, moduleFromRow(r))
	}

	return modules, nil
}

// GetTrackDetail builds the flat track-detail DTO (Section 36: "track detail:
// modules, lessons, instructor") for GET /academy/tracks/:slug. It fetches the
// track by slug joined to media (thumbnail_url) and academy_instructor_profiles +
// users + user_profiles (instructor name/bio/username/avatar), its modules,
// and each module's lessons, then assembles them into a single TrackDetail
// response with nested ModuleWithLessons. This matches the frontend
// TrackDetail contract (ADR-060) — NOT the legacy `{track, modules}` envelope.
func (s *Store) GetTrackDetail(ctx context.Context, slug string) (*TrackDetail, error) {
	row, err := s.read.GetAcademyTrackDetailBySlug(ctx,

		slug,
	)
	if err != nil {
		return nil, fmt.Errorf("get track detail by slug: %w", err)
	}

	track := trackFromFields(row.ID, row.Title, row.Slug, row.Description,
		row.Difficulty, row.CoverMediaID, row.IsPublished, row.SortOrder, row.CreatedAt, row.UpdatedAt,
		row.InstructorID, row.PriceKobo, row.Currency, row.IsPublishedToAcademy, row.Category,
		row.Language, row.TotalEnrolled, row.SubscriptionEligible)

	modRows, err := s.read.ListAcademyModulesByTrack(ctx,

		track.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("list modules by track: %w", err)
	}

	modules := make([]ModuleWithLessons, 0, len(modRows))

	for _, m := range modRows {
		lessonRows, err := s.read.ListAcademyLessonsByModule(ctx,

			m.ID,
		)
		if err != nil {
			return nil, fmt.Errorf("list lessons by module: %w", err)
		}

		lessons := make([]LessonSummary, 0, len(lessonRows))

		for _, l := range lessonRows {
			// Only include published lessons in the public track-detail view.
			if !l.IsPublished {
				continue
			}

			var lLabel, lISO string
			if l.DurationMinutes != nil {
				lLabel, lISO = formatTrackDuration(int(*l.DurationMinutes))
			}

			lessons = append(lessons, LessonSummary{
				ID:              l.ID,
				Slug:            l.Slug,
				Title:           l.Title,
				LessonType:      l.LessonType,
				DurationMinutes: l.DurationMinutes,
				SortOrder:       l.SortOrder,
				IsPreview:       l.IsPreview,
				DurationLabel:   lLabel,
				DurationISO:     lISO,
			})
		}

		modules = append(modules, ModuleWithLessons{
			ID:          m.ID,
			Slug:        m.Slug,
			Title:       m.Title,
			Description: m.Description,
			SortOrder:   m.SortOrder,
			Lessons:     lessons,
		})
	}

	// Compute total duration from all published lessons' duration_minutes.
	totalMinutes := 0

	for _, m := range modules {
		for _, l := range m.Lessons {
			if l.DurationMinutes != nil {
				totalMinutes += int(*l.DurationMinutes)
			}
		}
	}

	durationLabel, durationISO := formatTrackDuration(totalMinutes)

	detail := &TrackDetail{
		ID: track.ID,

		Title:                track.Title,
		Slug:                 track.Slug,
		Description:          track.Description,
		Difficulty:           track.Difficulty,
		Level:                track.Difficulty,
		CoverMediaID:         track.CoverMediaID,
		IsPublished:          track.IsPublished,
		SortOrder:            track.SortOrder,
		CreatedAt:            track.CreatedAt,
		UpdatedAt:            track.UpdatedAt,
		Modules:              modules,
		PriceKobo:            track.PriceKobo,
		Currency:             track.Currency,
		EnrolledCount:        track.TotalEnrolled,
		IsPublishedToAcademy: track.IsPublishedToAcademy,
		Category:             track.Category,
		Language:             track.Language,
		DurationLabel:        durationLabel,
		DurationISO:          durationISO,
		ThumbnailURL:         media.URLFromParts(row.ThumbnailStorageBucket, row.ThumbnailStoragePath, media.ImageProxyOpts{}),
		InstructorName:       row.InstructorName,
		SubscriptionEligible: track.SubscriptionEligible,
	}

	// Build the InstructorRef from the joined profile/user columns. The join
	// is LEFT, so a platform/staff-authored course (no instructor_id) leaves
	// AcademyInstructorProfileID invalid → Instructor stays nil and the frontend
	// {#if track.instructor} guard hides the instructor card.
	if row.AcademyInstructorProfileID.Valid && row.InstructorName != nil {
		detail.Instructor = &InstructorRef{
			ID:          row.AcademyInstructorProfileID.String(),
			Username:    strOrEmpty(row.InstructorUsername),
			DisplayName: *row.InstructorName,
			AvatarURL:   row.InstructorAvatarUrl,
			Bio:         row.InstructorBio,
		}
	}

	return detail, nil
}

// strOrEmpty dereferences a nullable string pointer, returning "" for nil so
// instructor_username serializes as an empty string (not null) when the join
// found a profile but no user row (defensive — should not normally happen).
func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}

	return *p
}

// formatTrackDuration converts a total minutes value into a human-readable
// label (e.g. "2h 30m") and an ISO 8601 duration (e.g. "PT2H30M"). Returns
// empty strings when totalMinutes is zero so the frontend {#if} guard hides
// the duration stat.
func formatTrackDuration(totalMinutes int) (label, iso string) {
	if totalMinutes <= 0 {
		return "", ""
	}

	hours := totalMinutes / 60
	mins := totalMinutes % 60

	if hours > 0 && mins > 0 {
		label = fmt.Sprintf("%dh %dm", hours, mins)
	} else if hours > 0 {
		label = fmt.Sprintf("%dh", hours)
	} else {
		label = fmt.Sprintf("%dm", mins)
	}

	iso = "PT"
	if hours > 0 {
		iso += fmt.Sprintf("%dH", hours)
	}

	if mins > 0 {
		iso += fmt.Sprintf("%dM", mins)
	}

	return label, iso
}

// GetModuleBySlug retrieves a module by its slug.
func (s *Store) GetModuleBySlug(ctx context.Context, trackID pgtype.UUID, slug string) (*Module, error) {
	r, err := s.read.GetAcademyModuleBySlug(ctx, gen.GetAcademyModuleBySlugParams{

		TrackID: trackID,
		Slug:    slug,
	})
	if err != nil {
		return nil, fmt.Errorf("get module by slug: %w", err)
	}

	m := moduleFromRow(r)

	return &m, nil
}

// GetModuleByID retrieves a module by its ID.
func (s *Store) GetModuleByID(ctx context.Context, id pgtype.UUID) (*Module, error) {
	r, err := s.read.GetAcademyModuleByID(ctx,

		id,
	)
	if err != nil {
		return nil, fmt.Errorf("get module by id: %w", err)
	}

	m := moduleFromRow(r)

	return &m, nil
}

// CreateModule creates a new module in a track.
func (s *Store) CreateModule(ctx context.Context, trackID pgtype.UUID, input CreateModuleInput) (*Module, error) {
	r, err := s.write.CreateAcademyModule(ctx, gen.CreateAcademyModuleParams{

		TrackID:     trackID,
		Title:       input.Title,
		Slug:        input.Slug,
		Description: input.Description,
		SortOrder:   input.SortOrder,
	})
	if err != nil {
		return nil, fmt.Errorf("create module: %w", err)
	}

	m := moduleFromRow(r)

	return &m, nil
}

// UpdateModule updates an existing module.
func (s *Store) UpdateModule(ctx context.Context, id pgtype.UUID, input UpdateModuleInput) (*Module, error) {
	r, err := s.write.UpdateAcademyModule(ctx, gen.UpdateAcademyModuleParams{

		ID:          id,
		Title:       input.Title,
		Description: input.Description,
		SortOrder:   input.SortOrder,
	})
	if err != nil {
		return nil, fmt.Errorf("update module: %w", err)
	}

	m := moduleFromRow(r)

	return &m, nil
}

// DeleteModule soft-deletes a module.
func (s *Store) DeleteModule(ctx context.Context, id pgtype.UUID) error {
	tag, err := s.db.Writer.Exec(ctx, `
		DELETE FROM academy_modules WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("delete module: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// ListAllModules returns all modules for a platform (admin view).
func (s *Store) ListAllModules(ctx context.Context) ([]Module, error) {
	rows, err := s.read.ListAllAcademyModules(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all modules: %w", err)
	}

	modules := make([]Module, 0, len(rows))

	for _, r := range rows {
		modules = append(modules, moduleFromRow(r))
	}

	return modules, nil
}

// ListAllLessons returns all lessons for a platform (admin view).
func (s *Store) ListAllLessons(ctx context.Context) ([]Lesson, error) {
	rows, err := s.read.ListAllAcademyLessons(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all lessons: %w", err)
	}

	lessons := make([]Lesson, 0, len(rows))

	for _, r := range rows {
		lessons = append(lessons, lessonFromListRow(r))
	}

	return lessons, nil
}

// ---------------------------------------------------------------------------
// Lesson operations
// ---------------------------------------------------------------------------

// Lesson represents lesson.
type Lesson struct {
	ID              pgtype.UUID  `json:"id"`
	ModuleID        pgtype.UUID  `json:"module_id"`
	ModuleTitle     *string      `json:"module_title,omitempty"`
	Title           string       `json:"title"`
	Slug            string       `json:"slug"`
	LessonType      string       `json:"lesson_type"`
	Content         *string      `json:"content,omitempty"`
	MediaID         *pgtype.UUID `json:"media_id,omitempty"`
	DurationMinutes *int32       `json:"duration_minutes,omitempty"`
	SortOrder       int32        `json:"sort_order"`
	IsPublished     bool         `json:"is_published"`
	IsPreview       bool         `json:"is_preview"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
	// IsCompleted is populated for authenticated users by GetLessonDetail
	// (queries academy_lesson_progress). Always false for guests/unauthed requests.
	IsCompleted bool `json:"is_completed,omitempty"`
}

// CreateLessonInput represents create lesson input.
type CreateLessonInput struct {
	Title           string      `json:"title"`
	Slug            string      `json:"slug"`
	LessonType      string      `json:"lesson_type"`
	Content         *string     `json:"content,omitempty"`
	MediaID         pgtype.UUID `json:"media_id"`
	DurationMinutes *int32      `json:"duration_minutes,omitempty"`
	SortOrder       int32       `json:"sort_order"`
	IsPublished     bool        `json:"is_published"`
	IsPreview       bool        `json:"is_preview"`
}

// UpdateLessonInput represents update lesson input. Nil pointers leave the
// column untouched (partial-update safe).
type UpdateLessonInput struct {
	Title           *string     `json:"title,omitempty"`
	LessonType      *string     `json:"lesson_type,omitempty"`
	Content         *string     `json:"content,omitempty"`
	MediaID         pgtype.UUID `json:"media_id"`
	DurationMinutes *int32      `json:"duration_minutes,omitempty"`
	SortOrder       *int32      `json:"sort_order,omitempty"`
	IsPublished     *bool       `json:"is_published,omitempty"`
	IsPreview       *bool       `json:"is_preview,omitempty"`
}

// buildLesson assembles a Lesson from the individual academy_lessons column
// values. All sqlc lesson row types select the same columns (plus is_preview
// after migration 000267), so the per-query mappers delegate here to avoid
// drift. mediaID is the raw pgtype.UUID (nullable); a valid zero value is
// treated as no media.
func buildLesson(
	id, moduleID pgtype.UUID,
	title, slug, lessonType string,
	content *string,
	mediaID pgtype.UUID,
	durationMinutes *int32,
	sortOrder int32,
	isPublished, isPreview bool,
	createdAt, updatedAt pgtype.Timestamptz,
) Lesson {
	var mid *pgtype.UUID
	if mediaID.Valid {
		mid = &mediaID
	}

	return Lesson{
		ID: id,

		ModuleID:        moduleID,
		Title:           title,
		Slug:            slug,
		LessonType:      lessonType,
		Content:         content,
		MediaID:         mid,
		DurationMinutes: durationMinutes,
		SortOrder:       sortOrder,
		IsPublished:     isPublished,
		IsPreview:       isPreview,
		CreatedAt:       createdAt.Time,
		UpdatedAt:       updatedAt.Time,
	}
}

// lessonFromRow maps any of the sqlc lesson row types (which all select the
// same academy_lessons columns) into a Lesson. sqlc emits a distinct row
// type per query after migration 000267 added is_preview, so we accept any
// and type-switch. gen.AcademyLesson (the table model) is retained for
// callers that still hold the base model.
func lessonFromRow(r any) Lesson {
	switch r := r.(type) {
	case gen.AcademyLesson:
		return buildLesson(r.ID, r.ModuleID, r.Title, r.Slug, r.LessonType, r.Content, r.MediaID, r.DurationMinutes, r.SortOrder, r.IsPublished, r.IsPreview, r.CreatedAt, r.UpdatedAt)
	case gen.ListAcademyLessonsByModuleRow:
		return buildLesson(r.ID, r.ModuleID, r.Title, r.Slug, r.LessonType, r.Content, r.MediaID, r.DurationMinutes, r.SortOrder, r.IsPublished, r.IsPreview, r.CreatedAt, r.UpdatedAt)
	case gen.GetAcademyLessonBySlugRow:
		return buildLesson(r.ID, r.ModuleID, r.Title, r.Slug, r.LessonType, r.Content, r.MediaID, r.DurationMinutes, r.SortOrder, r.IsPublished, r.IsPreview, r.CreatedAt, r.UpdatedAt)
	case gen.GetAcademyLessonByIDRow:
		return buildLesson(r.ID, r.ModuleID, r.Title, r.Slug, r.LessonType, r.Content, r.MediaID, r.DurationMinutes, r.SortOrder, r.IsPublished, r.IsPreview, r.CreatedAt, r.UpdatedAt)
	case gen.CreateAcademyLessonRow:
		return buildLesson(r.ID, r.ModuleID, r.Title, r.Slug, r.LessonType, r.Content, r.MediaID, r.DurationMinutes, r.SortOrder, r.IsPublished, r.IsPreview, r.CreatedAt, r.UpdatedAt)
	case gen.UpdateAcademyLessonRow:
		return buildLesson(r.ID, r.ModuleID, r.Title, r.Slug, r.LessonType, r.Content, r.MediaID, r.DurationMinutes, r.SortOrder, r.IsPublished, r.IsPreview, r.CreatedAt, r.UpdatedAt)
	}

	return Lesson{}
}

// lessonFromListRow maps a ListAllAcademyLessonsRow (with JOIN) to a Lesson.
func lessonFromListRow(r gen.ListAllAcademyLessonsRow) Lesson {
	var mediaID *pgtype.UUID
	if r.MediaID.Valid {
		mediaID = &r.MediaID
	}

	return Lesson{
		ID: r.ID,

		ModuleID:        r.ModuleID,
		ModuleTitle:     r.ModuleTitle,
		Title:           r.Title,
		Slug:            r.Slug,
		LessonType:      r.LessonType,
		Content:         r.Content,
		MediaID:         mediaID,
		DurationMinutes: r.DurationMinutes,
		SortOrder:       r.SortOrder,
		IsPublished:     r.IsPublished,
		CreatedAt:       r.CreatedAt.Time,
		UpdatedAt:       r.UpdatedAt.Time,
	}
}

// ListLessonsByModule lists all lessons for a module.
func (s *Store) ListLessonsByModule(ctx context.Context, moduleID pgtype.UUID) ([]Lesson, error) {
	rows, err := s.read.ListAcademyLessonsByModule(ctx,

		moduleID,
	)
	if err != nil {
		return nil, fmt.Errorf("list lessons by module: %w", err)
	}

	lessons := make([]Lesson, 0, len(rows))

	for _, r := range rows {
		lessons = append(lessons, lessonFromRow(r))
	}

	return lessons, nil
}

// GetLessonBySlug retrieves a lesson by its slug.
func (s *Store) GetLessonBySlug(ctx context.Context, moduleID pgtype.UUID, slug string) (*Lesson, error) {
	r, err := s.read.GetAcademyLessonBySlug(ctx, gen.GetAcademyLessonBySlugParams{

		ModuleID: moduleID,
		Slug:     slug,
	})
	if err != nil {
		return nil, fmt.Errorf("get lesson by slug: %w", err)
	}

	l := lessonFromRow(r)

	return &l, nil
}

// LessonNavRef is a reference to an adjacent lesson for prev/next navigation
// in the lesson player (ADR-060).
type LessonNavRef struct {
	Slug       string `json:"slug"`
	Title      string `json:"title"`
	ModuleSlug string `json:"module_slug"`
}

// LessonDetail is the enriched lesson response for the lesson player
// (ADR-060). It extends Lesson with track/module context, prev/next
// navigation, and content_html (plain-text content rendered to basic HTML).
// The frontend Lesson type expects these fields (contract: §3.3).
type LessonDetail struct {
	Lesson
	ModuleTitle string      `json:"module_title"`
	ModuleSlug  string      `json:"module_slug"`
	TrackID     pgtype.UUID `json:"track_id"`
	TrackSlug   string      `json:"track_slug"`
	TrackTitle  string      `json:"track_title"`
	ContentHTML string      `json:"content_html,omitempty"`
	// VideoURL/VideoDurationSecs are populated for video lessons from the
	// joined media row (ll.media_id → media). VideoURL uses the same-origin
	// serve endpoint — imgproxy cannot transform video payloads.
	VideoURL          *string  `json:"video_url,omitempty"`
	VideoDurationSecs *float64 `json:"video_duration_secs,omitempty"`
	// MediaURL is the same-origin serve URL for the lesson's attached media
	// (document, screencast, audio). Populated whenever media_id resolves.
	MediaURL *string `json:"media_url,omitempty"`
	// ExternalURL surfaces the lesson's external resource URL for
	// link/embed types (youtube, screencast, video, document, link) — the
	// editor stores the URL in the content column.
	ExternalURL *string `json:"external_url,omitempty"`
	// Checklist is populated for lesson_type='checklist' — content stores a
	// JSON array of strings or {id,label} objects.
	Checklist []ChecklistItem `json:"checklist,omitempty"`
	// ProgressSecs is the saved playback position for the requesting user
	// (academy_lesson_progress.position_secs); the video player resumes from it.
	ProgressSecs *int32 `json:"progress_secs,omitempty"`
	// Quiz is populated by the handler for lesson_type='quiz' lessons —
	// the lesson page renders the quiz engine from lesson.quiz (§36).
	Quiz       *Quiz         `json:"quiz,omitempty"`
	PrevLesson *LessonNavRef `json:"prev_lesson,omitempty"`
	NextLesson *LessonNavRef `json:"next_lesson,omitempty"`
}

// GetLessonDetail retrieves a lesson by slug with track/module context and
// prev/next navigation, suitable for the lesson player (ADR-060). When userID
// is valid, it also queries academy_lesson_progress to populate IsCompleted so the
// lesson player can show the "Completed" state on SSR render.
func (s *Store) GetLessonDetail(ctx context.Context, moduleID pgtype.UUID, slug string, userID ...pgtype.UUID) (*LessonDetail, error) {
	r, err := s.read.GetAcademyLessonDetailBySlug(ctx, gen.GetAcademyLessonDetailBySlugParams{

		ModuleID: moduleID,
		Slug:     slug,
	})
	if err != nil {
		return nil, fmt.Errorf("get lesson detail by slug: %w", err)
	}

	var mediaID *pgtype.UUID
	if r.MediaID.Valid {
		mediaID = &r.MediaID
	}

	detail := &LessonDetail{
		Lesson: Lesson{
			ID: r.ID,

			ModuleID:        r.ModuleID,
			Title:           r.Title,
			Slug:            r.Slug,
			LessonType:      r.LessonType,
			Content:         r.Content,
			MediaID:         mediaID,
			DurationMinutes: r.DurationMinutes,
			SortOrder:       r.SortOrder,
			IsPublished:     r.IsPublished,
			IsPreview:       r.IsPreview,
			CreatedAt:       r.CreatedAt.Time,
			UpdatedAt:       r.UpdatedAt.Time,
		},
		ModuleTitle: r.ModuleTitle,
		ModuleSlug:  r.ModuleSlug,
		TrackID:     r.TrackID,
		TrackSlug:   r.TrackSlug,
		TrackTitle:  r.TrackTitle,
		ContentHTML: renderLessonContentHTML(r.Content),
		VideoURL:    media.ServeURLFromParts(r.VideoStorageBucket, r.VideoStoragePath),
	}

	// MediaURL aliases the media-row resolution for non-video resource types
	// (document, screencast, audio). ExternalURL surfaces a URL stored in the
	// content column for link/embed lesson types.
	detail.MediaURL = detail.VideoURL
	if r.Content != nil {
		if url := strings.TrimSpace(*r.Content); isExternalURL(url) && r.LessonType != "article" && r.LessonType != "checklist" {
			detail.ExternalURL = &url
		}
	}
	if r.LessonType == "checklist" {
		detail.Checklist = parseChecklistContent(r.Content)
	}

	if r.VideoDurationSecs.Valid {
		if f, ferr := r.VideoDurationSecs.Float64Value(); ferr == nil && f.Valid {
			detail.VideoDurationSecs = &f.Float64
		}
	}

	// Populate IsCompleted and the saved playback position for authenticated
	// users. pgx.ErrNoRows means no progress row exists — the lesson is not
	// completed and has no resume position.
	if len(userID) > 0 && userID[0].Valid {
		status, err := s.read.GetLessonCompletionStatus(ctx, gen.GetLessonCompletionStatusParams{
			UserID:   userID[0],
			LessonID: r.ID,
		})
		if err == nil {
			detail.IsCompleted = status.IsCompleted
			detail.ProgressSecs = status.PositionSecs
		}
	}

	// Compute prev/next lesson navigation across the whole track.
	navRows, err := s.read.ListTrackLessonsForNav(ctx,
		r.TrackID,
	)
	if err == nil {
		for i, row := range navRows {
			if row.ID == r.ID {
				if i > 0 {
					p := navRows[i-1]
					detail.PrevLesson = &LessonNavRef{
						Slug:       p.Slug,
						Title:      p.Title,
						ModuleSlug: p.ModuleSlug,
					}
				}

				if i < len(navRows)-1 {
					n := navRows[i+1]
					detail.NextLesson = &LessonNavRef{
						Slug:       n.Slug,
						Title:      n.Title,
						ModuleSlug: n.ModuleSlug,
					}
				}

				break
			}
		}
	}

	return detail, nil
}

// renderLessonContentHTML converts lesson content into display HTML. The
// academy_lessons.content column holds markdown/plain text (migration
// 000039) — content.RenderBodyHTML runs the shared goldmark pipeline, which
// escapes raw HTML in the source (§5.1 — no stored XSS) while still
// formatting headings, lists, bold, tables, and links. Empty/nil content
// renders to an empty string.
func renderLessonContentHTML(content *string) string {
	if content == nil || *content == "" {
		return ""
	}

	return contentpkg.RenderBodyHTML(*content)
}

// isExternalURL reports whether s is an absolute http(s) URL — used to
// surface external resource links stored in a lesson's content column.
func isExternalURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// ChecklistItem is one item in a checklist lesson (frontend Lesson.checklist
// contract — id + label).
type ChecklistItem struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// parseChecklistContent decodes a checklist lesson's content column: a JSON
// array of strings or {id,label} objects. One plain-text line per item is
// also accepted so simple editors work.
func parseChecklistContent(content *string) []ChecklistItem {
	if content == nil || strings.TrimSpace(*content) == "" {
		return nil
	}

	raw := strings.TrimSpace(*content)

	var objects []struct {
		ID    string `json:"id"`
		Label string `json:"label"`
		Text  string `json:"text"`
	}
	if err := json.Unmarshal([]byte(raw), &objects); err == nil {
		out := make([]ChecklistItem, 0, len(objects))
		for i, o := range objects {
			label := o.Label
			if label == "" {
				label = o.Text
			}
			if label == "" {
				continue
			}
			id := o.ID
			if id == "" {
				id = fmt.Sprintf("item-%d", i+1)
			}
			out = append(out, ChecklistItem{ID: id, Label: label})
		}
		return out
	}

	var labels []string
	if err := json.Unmarshal([]byte(raw), &labels); err == nil {
		out := make([]ChecklistItem, 0, len(labels))
		for i, label := range labels {
			if label == "" {
				continue
			}
			out = append(out, ChecklistItem{ID: fmt.Sprintf("item-%d", i+1), Label: label})
		}
		return out
	}

	// Fallback: newline-separated plain text.
	lines := strings.Split(raw, "\n")
	out := make([]ChecklistItem, 0, len(lines))
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, ChecklistItem{ID: fmt.Sprintf("item-%d", i+1), Label: line})
	}
	return out
}

// GetLessonByID retrieves a lesson by its ID.
func (s *Store) GetLessonByID(ctx context.Context, id pgtype.UUID) (*Lesson, error) {
	r, err := s.read.GetAcademyLessonByID(ctx,

		id,
	)
	if err != nil {
		return nil, fmt.Errorf("get lesson by id: %w", err)
	}

	l := lessonFromRow(r)

	return &l, nil
}

// CreateLesson creates a new lesson in a module.
func (s *Store) CreateLesson(ctx context.Context, moduleID pgtype.UUID, input CreateLessonInput) (*Lesson, error) {
	r, err := s.write.CreateAcademyLesson(ctx, gen.CreateAcademyLessonParams{

		ModuleID:        moduleID,
		Title:           input.Title,
		Slug:            input.Slug,
		LessonType:      input.LessonType,
		Content:         input.Content,
		MediaID:         input.MediaID,
		DurationMinutes: input.DurationMinutes,
		SortOrder:       input.SortOrder,
		IsPublished:     input.IsPublished,
		IsPreview:       input.IsPreview,
	})
	if err != nil {
		return nil, fmt.Errorf("create lesson: %w", err)
	}

	l := lessonFromRow(r)

	return &l, nil
}

// UpdateLesson updates an existing lesson.
func (s *Store) UpdateLesson(ctx context.Context, id pgtype.UUID, input UpdateLessonInput) (*Lesson, error) {
	r, err := s.write.UpdateAcademyLesson(ctx, gen.UpdateAcademyLessonParams{

		ID:              id,
		Title:           input.Title,
		LessonType:      input.LessonType,
		Content:         input.Content,
		MediaID:         input.MediaID,
		DurationMinutes: input.DurationMinutes,
		SortOrder:       input.SortOrder,
		IsPublished:     input.IsPublished,
		IsPreview:       input.IsPreview,
	})
	if err != nil {
		return nil, fmt.Errorf("update lesson: %w", err)
	}

	l := lessonFromRow(r)

	return &l, nil
}

// DeleteLesson soft-deletes a lesson.
func (s *Store) DeleteLesson(ctx context.Context, id pgtype.UUID) error {
	tag, err := s.db.Writer.Exec(ctx, `
		DELETE FROM academy_lessons WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("delete lesson: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// ---------------------------------------------------------------------------
// Progress operations
// ---------------------------------------------------------------------------

// Progress represents progress.
type Progress struct {
	ID       pgtype.UUID `json:"id"`
	UserID   pgtype.UUID `json:"user_id"`
	LessonID pgtype.UUID `json:"lesson_id"`
	Status   string      `json:"status"`
	// ProgressSecs is the last saved playback position (video lessons); the
	// frontend posts `progress_secs` and resumes from this value.
	ProgressSecs *int32     `json:"progress_secs,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// UpdateProgressInput represents update progress input.
type UpdateProgressInput struct {
	Status       string `json:"status"`
	ProgressSecs *int32 `json:"progress_secs"`
	// Complete is a shorthand the frontend uses to mark a lesson complete via
	// the progress endpoint (§36 body: { progress_secs, status }).
	Complete *bool `json:"complete"`
}

// GetProgress retrieves progress for a specific lesson.
func (s *Store) GetProgress(ctx context.Context, userID, lessonID pgtype.UUID) (*Progress, error) {
	r, err := s.read.GetLessonProgress(ctx, gen.GetLessonProgressParams{
		UserID:   userID,
		LessonID: lessonID,
	})
	if err != nil {
		return nil, fmt.Errorf("get progress: %w", err)
	}

	return &Progress{
		ID: r.ID,

		UserID:       r.UserID,
		LessonID:     r.LessonID,
		Status:       r.Status,
		ProgressSecs: r.PositionSecs,
		CompletedAt:  completedAtFromTimestamptz(r.CompletedAt),
		CreatedAt:    r.CreatedAt.Time,
	}, nil
}

// UpsertProgress creates or updates progress for a lesson.
func (s *Store) UpsertProgress(ctx context.Context, userID, lessonID pgtype.UUID, input UpdateProgressInput) (*Progress, error) {
	var completedAt *time.Time

	if input.Status == "completed" {
		now := time.Now()
		completedAt = &now
	}

	var completedAtTS pgtype.Timestamptz
	if completedAt != nil {
		completedAtTS = pgtype.Timestamptz{Time: *completedAt, Valid: true}
	}

	r, err := s.write.UpsertLessonProgress(ctx, gen.UpsertLessonProgressParams{

		UserID:       userID,
		LessonID:     lessonID,
		Status:       input.Status,
		PositionSecs: input.ProgressSecs,
		CompletedAt:  completedAtTS,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert progress: %w", err)
	}

	return &Progress{
		ID: r.ID,

		UserID:       r.UserID,
		LessonID:     r.LessonID,
		Status:       r.Status,
		ProgressSecs: r.PositionSecs,
		CompletedAt:  completedAtFromTimestamptz(r.CompletedAt),
		CreatedAt:    r.CreatedAt.Time,
	}, nil
}

// GetUserTrackProgress retrieves track progress for a user.
func (s *Store) GetUserTrackProgress(ctx context.Context, userID, trackID pgtype.UUID) (completed, total int, err error) {
	r, err := s.read.GetUserTrackProgress(ctx, gen.GetUserTrackProgressParams{
		UserID:  userID,
		TrackID: trackID,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("get user track progress: %w", err)
	}

	return int(r.Count), int(r.Count_2), nil
}

// ListCompletedLessonIDsByUserAndTrack returns the IDs of all lessons the user
// has completed in a track. Used to populate the curriculum rail's completion
// state on page refresh.
func (s *Store) ListCompletedLessonIDsByUserAndTrack(ctx context.Context, userID, trackID pgtype.UUID) ([]pgtype.UUID, error) {
	rows, err := s.read.ListCompletedLessonIDsByUserAndTrack(ctx, gen.ListCompletedLessonIDsByUserAndTrackParams{
		UserID:  userID,
		TrackID: trackID,
	})
	if err != nil {
		return nil, fmt.Errorf("list completed lesson ids: %w", err)
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// Certificate operations
// ---------------------------------------------------------------------------

// Certificate represents certificate.
type Certificate struct {
	ID                pgtype.UUID  `json:"id"`
	UserID            pgtype.UUID  `json:"user_id"`
	TrackID           pgtype.UUID  `json:"track_id"`
	CertificateNumber string       `json:"certificate_number"`
	MediaID           *pgtype.UUID `json:"media_id,omitempty"`
	IssuedAt          time.Time    `json:"issued_at"`
	Title             *string      `json:"title,omitempty"`
	TrackSlug         *string      `json:"track_slug,omitempty"`
	// RecipientName is the display name (falls back to username when no
	// display_name is set). Populated by the list/detail queries that JOIN
	// users + user_profiles; empty when issued via IssueCertificate.
	RecipientName     string `json:"recipient_name,omitempty"`
	RecipientUsername string `json:"recipient_username,omitempty"`
}

func certFromRow(r gen.AcademyCertificate) Certificate {
	var mediaID *pgtype.UUID
	if r.MediaID.Valid {
		mediaID = &r.MediaID
	}

	return Certificate{
		ID: r.ID,

		UserID:            r.UserID,
		TrackID:           r.TrackID,
		CertificateNumber: r.CertificateNumber,
		MediaID:           mediaID,
		IssuedAt:          r.IssuedAt.Time,
	}
}

// certFromListRow maps the joined certificate+track row (used by the public
// profile Certificates tab) to a Certificate, populating Title and TrackSlug
// from the joined academy_tracks columns.
func certFromListRow(r gen.ListAcademyCertificatesByUserRow) Certificate {
	var mediaID *pgtype.UUID
	if r.MediaID.Valid {
		mediaID = &r.MediaID
	}

	recipientUsername := ""
	if r.RecipientUsername != nil {
		recipientUsername = *r.RecipientUsername
	}

	return Certificate{
		ID: r.ID,

		UserID:            r.UserID,
		TrackID:           r.TrackID,
		CertificateNumber: r.CertificateNumber,
		MediaID:           mediaID,
		IssuedAt:          r.IssuedAt.Time,
		Title:             r.TrackTitle,
		TrackSlug:         r.TrackSlug,
		RecipientName:     r.RecipientName,
		RecipientUsername: recipientUsername,
	}
}

// GetCertificate retrieves a certificate by its ID.
func (s *Store) GetCertificate(ctx context.Context, userID, trackID pgtype.UUID) (*Certificate, error) {
	r, err := s.read.GetAcademyCertificate(ctx, gen.GetAcademyCertificateParams{
		UserID:  userID,
		TrackID: trackID,
	})
	if err != nil {
		return nil, fmt.Errorf("get certificate: %w", err)
	}

	c := certFromRow(r)

	return &c, nil
}

// IssueCertificate issues a certificate for a completed track.
func (s *Store) IssueCertificate(ctx context.Context, userID, trackID pgtype.UUID, certificateNumber string) (*Certificate, error) {
	r, err := s.write.IssueAcademyCertificate(ctx, gen.IssueAcademyCertificateParams{

		UserID:            userID,
		TrackID:           trackID,
		CertificateNumber: certificateNumber,
	})
	if err != nil {
		// ON CONFLICT DO NOTHING returns ErrNoRows when a certificate already
		// exists for this (user_id, track_id) pair — fetch and return it so
		// the caller sees a successful idempotent issuance (audit BUG-BE-7).
		if errors.Is(err, pgx.ErrNoRows) {
			existing, getErr := s.GetCertificate(ctx, userID, trackID)
			if getErr != nil {
				return nil, fmt.Errorf("issue certificate: conflict but existing lookup failed: %w", getErr)
			}

			return existing, nil
		}

		return nil, fmt.Errorf("issue certificate: %w", err)
	}

	c := certFromRow(r)

	return &c, nil
}

// ListCertificatesByUser lists a user's certificates, platform-scoped.
func (s *Store) ListCertificatesByUser(ctx context.Context, userID pgtype.UUID) ([]Certificate, error) {
	rows, err := s.read.ListAcademyCertificatesByUser(ctx,

		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list certificates by user: %w", err)
	}

	certs := make([]Certificate, 0, len(rows))

	for _, r := range rows {
		certs = append(certs, certFromListRow(r))
	}

	return certs, nil
}

// GetCertificateByID retrieves a certificate by its ID.
func (s *Store) GetCertificateByID(ctx context.Context, certID pgtype.UUID) (*Certificate, error) {
	r, err := s.read.GetAcademyCertificateByID(ctx, certID)
	if err != nil {
		return nil, fmt.Errorf("get certificate by id: %w", err)
	}

	var mediaID *pgtype.UUID
	if r.MediaID.Valid {
		mediaID = &r.MediaID
	}

	recipientUsername := ""
	if r.RecipientUsername != nil {
		recipientUsername = *r.RecipientUsername
	}

	c := Certificate{
		ID: r.ID,

		UserID:            r.UserID,
		TrackID:           r.TrackID,
		CertificateNumber: r.CertificateNumber,
		MediaID:           mediaID,
		IssuedAt:          r.IssuedAt.Time,
		Title:             r.TrackTitle,
		TrackSlug:         r.TrackSlug,
		RecipientName:     r.RecipientName,
		RecipientUsername: recipientUsername,
	}

	return &c, nil
}

// ---------------------------------------------------------------------------
// Enrollment operations
// ---------------------------------------------------------------------------

// Enrollment represents a user's enrollment in a learning track. The basic
// fields (ID, UserID, TrackID, AccessType, ProgressPct, EnrolledAt,
// CompletedAt) are always populated. The enriched fields (TrackSlug,
// TrackTitle, ThumbnailURL, Level, LessonsCompleted, LessonsTotal,
// LastActivityAt, NextLesson) are only populated when the enrollment is
// loaded via ListEnrollmentsByUser — the /me/enrollments endpoint that feeds
// the Continue Learning card and lesson player progress ring.
type Enrollment struct {
	ID          pgtype.UUID `json:"id"`
	UserID      pgtype.UUID `json:"user_id"`
	TrackID     pgtype.UUID `json:"track_id"`
	AccessType  string      `json:"access_type"`
	ProgressPct float64     `json:"progress_pct"`
	EnrolledAt  time.Time   `json:"enrolled_at"`
	CompletedAt *time.Time  `json:"completed_at,omitempty"`
	// Enriched fields — populated by ListEnrollmentsByUser.
	TrackSlug        string        `json:"track_slug,omitempty"`
	TrackTitle       string        `json:"track_title,omitempty"`
	ThumbnailURL     *string       `json:"thumbnail_url,omitempty"`
	Level            string        `json:"level,omitempty"`
	LessonsCompleted int32         `json:"lessons_completed,omitempty"`
	LessonsTotal     int32         `json:"lessons_total,omitempty"`
	LastActivityAt   *time.Time    `json:"last_activity_at,omitempty"`
	NextLesson       *LessonNavRef `json:"next_lesson,omitempty"`
}

// EnrollUser creates an enrollment for a user in a track. The is
// required by the academy_enrollments table FK constraint — omitting it leaves a
// zero-value UUID that violates enrollments_the id_fkey and surfaces as a
// 500 db_error.
func (s *Store) EnrollUser(ctx context.Context, userID, trackID pgtype.UUID, accessType string) (*Enrollment, error) {
	r, err := s.write.CreateEnrollment(ctx, gen.CreateEnrollmentParams{

		UserID:     userID,
		TrackID:    trackID,
		AccessType: accessType,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Rare race: a concurrent enroll committed between the
			// statement snapshot and ON CONFLICT resolution. Fetch the
			// now-visible row and treat the call as already-enrolled.
			return s.GetEnrollment(ctx, userID, trackID)
		}
		return nil, fmt.Errorf("enroll user: %w", err)
	}

	progressPct, _ := r.ProgressPct.Float64Value()

	return &Enrollment{
		ID:          r.ID,
		UserID:      r.UserID,
		TrackID:     r.TrackID,
		AccessType:  r.AccessType,
		ProgressPct: progressPct.Float64,
		EnrolledAt:  r.EnrolledAt.Time,
		CompletedAt: completedAtFromTimestamptz(r.CompletedAt),
	}, nil
}

// ListEnrollmentsByUser lists all academy_enrollments for a user with enriched track
// metadata (slug, title, thumbnail, level) and computed progress fields
// (lessons_completed, lessons_total, last_activity_at, next_lesson). This
// feeds GET /me/enrollments, which the Continue Learning card and lesson
// player progress ring consume (ADR-060 CSR boundary).
func (s *Store) ListEnrollmentsByUser(ctx context.Context, userID pgtype.UUID) ([]Enrollment, error) {
	rows, err := s.read.ListEnrollmentsByUserIDWithDetails(ctx,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list academy_enrollments by user: %w", err)
	}

	academy_enrollments := make([]Enrollment, 0, len(rows))

	for _, r := range rows {
		progressPct, _ := r.ProgressPct.Float64Value()

		var lastActivityAt *time.Time

		if r.LastActivityAt != nil {
			if ts, ok := r.LastActivityAt.(time.Time); ok {
				lastActivityAt = &ts
			}
		}

		e := Enrollment{
			ID:               r.ID,
			UserID:           r.UserID,
			TrackID:          r.TrackID,
			AccessType:       r.AccessType,
			ProgressPct:      progressPct.Float64,
			EnrolledAt:       r.EnrolledAt.Time,
			CompletedAt:      completedAtFromTimestamptz(r.CompletedAt),
			TrackSlug:        r.TrackSlug,
			TrackTitle:       r.TrackTitle,
			ThumbnailURL:     media.URLFromParts(r.ThumbnailStorageBucket, r.ThumbnailStoragePath, media.ImageProxyOpts{}),
			Level:            r.Level,
			LessonsCompleted: int32(r.LessonsCompleted), // #nosec G115 -- DB count, bounded by int32 column type
			LessonsTotal:     int32(r.LessonsTotal),     // #nosec G115 -- DB count, bounded by int32 column type
			LastActivityAt:   lastActivityAt,
		}

		// Next incomplete lesson comes from the LATERAL join — no N+1.
		// Empty slug means all lessons are completed → leave NextLesson nil.
		if r.NextLessonSlug != "" {
			e.NextLesson = &LessonNavRef{
				Slug:       r.NextLessonSlug,
				Title:      r.NextLessonTitle,
				ModuleSlug: r.NextLessonModuleSlug,
			}
		}

		academy_enrollments = append(academy_enrollments, e)
	}

	return academy_enrollments, nil
}

// GetEnrollment retrieves a specific enrollment.
func (s *Store) GetEnrollment(ctx context.Context, userID, trackID pgtype.UUID) (*Enrollment, error) {
	r, err := s.read.GetEnrollmentByUserAndTrack(ctx, gen.GetEnrollmentByUserAndTrackParams{
		UserID:  userID,
		TrackID: trackID,
	})
	if err != nil {
		return nil, fmt.Errorf("get enrollment: %w", err)
	}

	progressPct, _ := r.ProgressPct.Float64Value()

	return &Enrollment{
		ID:          r.ID,
		UserID:      r.UserID,
		TrackID:     r.TrackID,
		AccessType:  r.AccessType,
		ProgressPct: progressPct.Float64,
		EnrolledAt:  r.EnrolledAt.Time,
		CompletedAt: completedAtFromTimestamptz(r.CompletedAt),
	}, nil
}

// completedAtFromTimestamptz converts a nullable timestamptz to *time.Time.
func completedAtFromTimestamptz(ts pgtype.Timestamptz) *time.Time {
	if ts.Valid {
		t := ts.Time
		return &t
	}

	return nil
}

// UpdateEnrollmentProgress updates the progress percentage of an enrollment.
func (s *Store) UpdateEnrollmentProgress(ctx context.Context, userID, trackID pgtype.UUID, progressPct float64) error {
	var num pgtype.Numeric
	if err := num.Scan(fmt.Sprintf("%f", progressPct)); err != nil {
		return fmt.Errorf("scan progress pct: %w", err)
	}

	err := s.write.UpdateEnrollmentProgress(ctx, gen.UpdateEnrollmentProgressParams{
		UserID:      userID,
		TrackID:     trackID,
		ProgressPct: num,
	})
	if err != nil {
		return fmt.Errorf("update enrollment progress: %w", err)
	}

	return nil
}

// EnrollmentRow holds an enrollment with joined user info.
type EnrollmentRow struct {
	ID          pgtype.UUID        `json:"id"`
	UserID      pgtype.UUID        `json:"user_id"`
	TrackID     pgtype.UUID        `json:"track_id"`
	AccessType  string             `json:"access_type"`
	ProgressPct pgtype.Numeric     `json:"progress_pct"`
	EnrolledAt  pgtype.Timestamptz `json:"enrolled_at"`
	CompletedAt pgtype.Timestamptz `json:"completed_at"`
	Username    string             `json:"username"`
	DisplayName string             `json:"display_name"`
}

// ListEnrollmentsByTrack returns paginated academy_enrollments for a track with user info.
func (s *Store) ListEnrollmentsByTrack(ctx context.Context, trackID pgtype.UUID, limit, offset int32) ([]EnrollmentRow, error) {
	rows, err := s.read.ListEnrollmentsByTrackID(ctx, gen.ListEnrollmentsByTrackIDParams{

		TrackID:    trackID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing academy_enrollments by track: %w", err)
	}

	result := make([]EnrollmentRow, len(rows))
	for i, r := range rows {
		result[i] = EnrollmentRow{
			ID: r.ID,

			UserID:      r.UserID,
			TrackID:     r.TrackID,
			AccessType:  r.AccessType,
			ProgressPct: r.ProgressPct,
			EnrolledAt:  r.EnrolledAt,
			CompletedAt: r.CompletedAt,
			Username:    r.Username,
			DisplayName: r.DisplayName,
		}
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Quiz operations
// ---------------------------------------------------------------------------

// QuizQuestion represents a quiz question row (internal — grading path only).
type QuizQuestion struct {
	ID          pgtype.UUID `json:"id"`
	LessonID    pgtype.UUID `json:"lesson_id"`
	Question    string      `json:"question"`
	Options     string      `json:"options"` // JSON array
	Answer      string      `json:"answer"`
	Explanation *string     `json:"explanation,omitempty"`
	Position    int32       `json:"position"`
}

// quizPassScorePct is the pass threshold for lesson quizzes (§36 quiz engine).
const quizPassScorePct = 70

// QuizOption is one selectable answer in the frontend QuizOption contract.
// The option's ID is its text — academy_quizzes stores the correct answer as
// the option text (answer column), so the selected option's label is also
// its identity.
type QuizOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// QuizDTOQuestion is a quiz question in the frontend QuizQuestion contract
// (prompt/options/points). academy_quizzes has a single free-text answer per
// question, so every rendered question is type "single" worth 1 point.
type QuizDTOQuestion struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Prompt   string       `json:"prompt"`
	Options  []QuizOption `json:"options,omitempty"`
	Points   int          `json:"points"`
	Position int32        `json:"position"`
}

// Quiz is the quiz object embedded in the lesson detail response
// (lesson.quiz) and returned by GET /academy/quizzes/{lesson_id}. The ID is the
// lesson ID — a quiz belongs to exactly one lesson (lesson_id FK), and the
// route path parameter is the lesson ID (§36).
type Quiz struct {
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	PassScorePct int               `json:"pass_score_pct"`
	Questions    []QuizDTOQuestion `json:"questions"`
}

// parseQuizOptions decodes the academy_quizzes.options text column. Seed data
// and §36 write either a JSON array of strings (["A","B"]) or objects
// ([{text, is_correct}]) — accept both, emitting plain option labels. The
// is_correct flag is never surfaced to the client (answers are server-only).
func parseQuizOptions(raw string) []QuizOption {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	var objects []struct {
		Text  string `json:"text"`
		Label string `json:"label"`
	}
	if err := json.Unmarshal([]byte(trimmed), &objects); err == nil {
		out := make([]QuizOption, 0, len(objects))
		for _, o := range objects {
			label := o.Text
			if label == "" {
				label = o.Label
			}
			if label == "" {
				continue
			}
			out = append(out, QuizOption{ID: label, Label: label})
		}
		return out
	}

	var labels []string
	if err := json.Unmarshal([]byte(trimmed), &labels); err == nil {
		out := make([]QuizOption, 0, len(labels))
		for _, label := range labels {
			if label == "" {
				continue
			}
			out = append(out, QuizOption{ID: label, Label: label})
		}
		return out
	}

	return nil
}

// GetQuiz retrieves the quiz for a lesson WITHOUT answers or explanations.
// Used by the public GET /academy/quizzes/:id endpoint and the quiz-lesson
// embed so users cannot read correct answers before submitting (§36: "no
// answers revealed"). Returns nil (no error) when the lesson has no quiz.
func (s *Store) GetQuiz(ctx context.Context, lessonID pgtype.UUID) (*Quiz, error) {
	rows, err := s.read.GetQuizQuestionsByLesson(ctx,

		lessonID,
	)
	if err != nil {
		return nil, fmt.Errorf("get quiz questions: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	questions := make([]QuizDTOQuestion, 0, len(rows))

	for _, r := range rows {
		questions = append(questions, QuizDTOQuestion{
			ID:       r.ID.String(),
			Type:     "single",
			Prompt:   r.Question,
			Options:  parseQuizOptions(r.Options),
			Points:   1,
			Position: r.Position,
		})
	}

	return &Quiz{
		ID:           lessonID.String(),
		Title:        rows[0].LessonTitle,
		PassScorePct: quizPassScorePct,
		Questions:    questions,
	}, nil
}

// GetQuizQuestionsWithAnswers retrieves quiz questions with correct answers and
// explanations for grading (POST /academy/quizzes/:id/submit). This is the only
// path that includes the Answer field.
func (s *Store) GetQuizQuestionsWithAnswers(ctx context.Context, lessonID pgtype.UUID) ([]QuizQuestion, error) {
	rows, err := s.read.GetQuizQuestionsByLesson(ctx,

		lessonID,
	)
	if err != nil {
		return nil, fmt.Errorf("get quiz questions with answers: %w", err)
	}

	questions := make([]QuizQuestion, 0, len(rows))

	for _, r := range rows {
		questions = append(questions, QuizQuestion{
			ID:          r.ID,
			LessonID:    r.LessonID,
			Question:    r.Question,
			Options:     r.Options,
			Answer:      r.Answer,
			Explanation: r.Explanation,
			Position:    r.Position,
		})
	}

	return questions, nil
}

// ---------------------------------------------------------------------------
// Learning track reviews (AUDIT-BE-ROUTE-040)
// Reuse Section 29 reviews with entity_type=academy_track.
// ---------------------------------------------------------------------------

// TrackReview is the API-facing review record for a learning track. Field
// names mirror the frontend TrackReview type (ADR-060): track_id (the reviewed
// track), author_id/author_username/author_displayName/author_avatar_url (the
// reviewer, resolved via ListReviewsByEntityWithAuthor). The create path
// (CreateTrackReview) returns author_* empty since the just-inserted row has
// not been re-joined — callers that need author info should re-fetch the list.
type TrackReview struct {
	ID                string    `json:"id"`
	TrackID           string    `json:"track_id"`
	AuthorID          string    `json:"author_id"`
	AuthorUsername    string    `json:"author_username"`
	AuthorDisplayName *string   `json:"author_displayName,omitempty"`
	AuthorAvatarURL   *string   `json:"author_avatar_url,omitempty"`
	Rating            int16     `json:"rating"`
	Title             *string   `json:"title,omitempty"`
	Body              *string   `json:"body,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// TrackReviewAggregate holds the aggregate rating for a track. JSON keys
// `count`/`avg` match the frontend ReviewAggregate type (ADR-060) so the hero
// rating and the Reviews tab aggregate render without a field-name mismatch.
type TrackReviewAggregate struct {
	Count int32   `json:"count"`
	Avg   float64 `json:"avg"`
}

// CreateTrackReview inserts a review for a learning track and updates the
// review_aggregates rollup (Section 29). AUDIT-BE-ROUTE-040: entity_type=academy_track.
func (s *Store) CreateTrackReview(ctx context.Context, trackID, reviewerID pgtype.UUID, rating int16, title, body *string) (TrackReview, error) {
	row, err := s.write.CreateReview(ctx, gen.CreateReviewParams{

		EntityType:     "academy_track",
		EntityID:       trackID,
		ReviewerUserID: reviewerID,
		Rating:         rating,
		Title:          title,
		Body:           body,
	})
	if err != nil {
		return TrackReview{}, err
	}

	// Maintain the review_aggregates rollup so GetTrackReviewAggregate can
	// read the precomputed count/avg instead of scanning all review rows.
	if _, aggErr := s.write.UpsertReviewAggregate(ctx, gen.UpsertReviewAggregateParams{

		EntityType: "academy_track",
		EntityID:   trackID,
	}); aggErr != nil {
		// Non-fatal — the review was inserted; the aggregate will be
		// recomputed on the next review or by a maintenance task.
		slog.Warn("academy: upsert review aggregate for track", "track_id", trackID, "error", aggErr)
	}

	return trackReviewFromRow(row), nil
}

// ListTrackReviews lists approved reviews for a learning track, joined to
// users + user_profiles so author username/display name/avatar are populated
// for the Reviews tab (ADR-060). Ref: Section 29 polymorphic reviews.
func (s *Store) ListTrackReviews(ctx context.Context, trackID pgtype.UUID, limit, offset int32) ([]TrackReview, error) {
	rows, err := s.read.ListReviewsByEntityWithAuthor(ctx, gen.ListReviewsByEntityWithAuthorParams{

		EntityType: "academy_track",
		EntityID:   trackID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, err
	}

	reviews := make([]TrackReview, 0, len(rows))
	for _, r := range rows {
		reviews = append(reviews, trackReviewWithAuthorFromRow(r))
	}

	return reviews, nil
}

// GetTrackReviewAggregate returns the total review count and average rating
// for a learning track from the review_aggregates rollup table (Section 29).
// Falls back to zeros when no aggregate row exists yet (no reviews posted).
func (s *Store) GetTrackReviewAggregate(ctx context.Context, trackID pgtype.UUID) (TrackReviewAggregate, error) {
	agg, err := s.read.GetReviewAggregate(ctx, gen.GetReviewAggregateParams{

		EntityType: "academy_track",
		EntityID:   trackID,
	})
	if err != nil {
		// No aggregate row = no reviews yet. Return zeros.
		return TrackReviewAggregate{}, nil
	}

	avg, _ := agg.AverageRating.Float64Value()

	return TrackReviewAggregate{
		Count: agg.ReviewCount,
		Avg:   avg.Float64,
	}, nil
}

// trackReviewFromRow maps a base gen.Review (no author join) into the
// frontend-facing TrackReview. Author fields are left empty; the list path
// uses trackReviewWithAuthorFromRow instead.
func trackReviewFromRow(r gen.Review) TrackReview {
	return TrackReview{
		ID:             r.ID.String(),
		TrackID:        r.EntityID.String(),
		AuthorID:       r.ReviewerUserID.String(),
		AuthorUsername: "",
		Rating:         r.Rating,
		Title:          r.Title,
		Body:           r.Body,
		CreatedAt:      r.CreatedAt.Time,
		UpdatedAt:      r.UpdatedAt.Time,
	}
}

// trackReviewWithAuthorFromRow maps a ListReviewsByEntityWithAuthorRow (joined
// to users + user_profiles) into the frontend-facing TrackReview, populating
// author_username, author_displayName, and author_avatar_url.
func trackReviewWithAuthorFromRow(r gen.ListReviewsByEntityWithAuthorRow) TrackReview {
	return TrackReview{
		ID:                r.ID.String(),
		TrackID:           r.EntityID.String(),
		AuthorID:          r.ReviewerUserID.String(),
		AuthorUsername:    strOrEmpty(r.AuthorUsername),
		AuthorDisplayName: r.AuthorDisplayName,
		AuthorAvatarURL:   r.AuthorAvatarUrl,
		Rating:            r.Rating,
		Title:             r.Title,
		Body:              r.Body,
		CreatedAt:         r.CreatedAt.Time,
		UpdatedAt:         r.UpdatedAt.Time,
	}
}

// CheckSubscriptionAccess determines whether a user's active subscription
// plan grants access to a specific track. Returns (true, planSlug) when:
// - the user has an active subscription, AND
// - the corresponding subscription_plan either has no entries in
// academy_subscription_plan_tracks (meaning ALL tracks — backward compatible
// with the Pro = all model) OR explicitly includes the track.
// // Returns (false, "") when the user has no active subscription or the plan
// does not cover the track.
func (s *Store) CheckSubscriptionAccess(ctx context.Context, userID, trackID pgtype.UUID) (bool, string) {
	// 1. Get the user's active subscription plan text (e.g. "pro_monthly").
	var planText string

	err := s.db.Reader.QueryRow(ctx, `
		SELECT plan FROM subscriptions
		WHERE user_id = $1 AND status = 'active' AND current_period_end > now()
		ORDER BY created_at DESC LIMIT 1
	`, userID).Scan(&planText)
	if err != nil {
		return false, ""
	}

	// 2. Extract the plan slug (e.g. "pro" from "pro_monthly").
	slug := planText
	if idx := strings.Index(planText, "_"); idx > 0 {
		slug = planText[:idx]
	}

	// 3. Look up the subscription_plan by slug to get its ID.
	var planID pgtype.UUID

	err = s.db.Reader.QueryRow(ctx, `
		SELECT id FROM subscription_plans
		WHERE slug = $1
	`, slug).Scan(&planID)
	if err != nil {
		return false, ""
	}

	// 4. Count how many tracks are explicitly attached to this plan.
	var count int64

	err = s.db.Reader.QueryRow(ctx, `
		SELECT count(*) FROM academy_subscription_plan_tracks
		WHERE plan_id = $1
	`, planID).Scan(&count)
	if err != nil {
		return false, ""
	}

	// 5. If the plan has no entries, it includes ALL tracks (Pro = all).
	if count == 0 {
		return true, slug
	}

	// 6. Otherwise, check if the specific track is in the plan.
	var exists int32

	err = s.db.Reader.QueryRow(ctx, `
		SELECT 1 FROM academy_subscription_plan_tracks
		WHERE plan_id = $1 AND track_id = $2
		LIMIT 1
	`, planID, trackID).Scan(&exists)
	if err != nil {
		return false, slug
	}

	return true, slug
}
