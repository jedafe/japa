package learning

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides direct pgx CRUD for learning tracks, modules, lessons,
// progress records, and certificates. All queries are tenant-scoped via
// the tenant_id column and use the shared read/write connection pools.
type Store struct {
	read  *gen.Queries
	write *gen.Queries
	db    *db.DB
}

// NewStore creates a learning store bound to the provided database pools.
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
	ID           pgtype.UUID  `json:"id"`
	TenantID     pgtype.UUID  `json:"tenant_id"`
	Title        string       `json:"title"`
	Slug         string       `json:"slug"`
	Description  *string      `json:"description,omitempty"`
	Difficulty   string       `json:"difficulty"`
	/** Level is an alias for Difficulty, matching the frontend TrackSummary.level
	 *  contract (ADR-060). Always equal to Difficulty. */
	Level        string       `json:"level"`
	CoverMediaID *pgtype.UUID `json:"cover_media_id,omitempty"`
	IsPublished  bool         `json:"is_published"`
	SortOrder    int32        `json:"sort_order"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	// Marketplace columns (migration 000166, ADR-063).
	InstructorID            *pgtype.UUID `json:"instructor_id,omitempty"`
	PriceKobo               *int64       `json:"price_kobo,omitempty"`
	Currency                string       `json:"currency,omitempty"`
	IsPublishedToUniversity bool         `json:"is_published_to_university,omitempty"`
	Category                *string      `json:"category,omitempty"`
	Language                string       `json:"language,omitempty"`
	TotalEnrolled           int32        `json:"total_enrolled,omitempty"`
}

// CreateTrackInput represents create track input.
type CreateTrackInput struct {
	Title        string      `json:"title"`
	Slug         string      `json:"slug"`
	Description  *string     `json:"description,omitempty"`
	Difficulty   string      `json:"difficulty"`
	CoverMediaID pgtype.UUID `json:"cover_media_id"`
	IsPublished  bool        `json:"is_published"`
	SortOrder    int32       `json:"sort_order"`
}

// UpdateTrackInput represents update track input.
type UpdateTrackInput struct {
	Title        string      `json:"title"`
	Description  *string     `json:"description,omitempty"`
	Difficulty   string      `json:"difficulty"`
	CoverMediaID pgtype.UUID `json:"cover_media_id"`
	IsPublished  bool        `json:"is_published"`
	SortOrder    int32       `json:"sort_order"`
}

// trackFromFields builds a Track DTO from the base learning_tracks columns
// plus the marketplace columns added by migration 000166 (ADR-063). Migration
// 000166 caused sqlc to emit per-query row types instead of reusing the
// gen.LearningTrack model; all these row types share the same columns, so
// callers extract the fields and pass them here.
func trackFromFields(
	id, tenantID pgtype.UUID,
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
	isPublishedToUniversity bool,
	category *string,
	language string,
	totalEnrolled int32,
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
		ID:                      id,
		TenantID:                tenantID,
		Title:                   title,
		Slug:                    slug,
		Description:             description,
		Difficulty:              difficulty,
		Level:                   difficulty,
		CoverMediaID:            cmID,
		IsPublished:             isPublished,
		SortOrder:               sortOrder,
		CreatedAt:               createdAt.Time,
		UpdatedAt:               updatedAt.Time,
		InstructorID:            instrID,
		PriceKobo:               priceKobo,
		Currency:                currency,
		IsPublishedToUniversity: isPublishedToUniversity,
		Category:                category,
		Language:                language,
		TotalEnrolled:           totalEnrolled,
	}
}

// ListPublishedTracks lists all published tracks for a tenant.
func (s *Store) ListPublishedTracks(ctx context.Context, tenantID pgtype.UUID) ([]Track, error) {
	rows, err := s.read.ListPublishedLearningTracks(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list published tracks: %w", err)
	}

	tracks := make([]Track, 0, len(rows))

	for _, r := range rows {
		tracks = append(tracks, trackFromFields(r.ID, r.TenantID, r.Title, r.Slug, r.Description, r.Difficulty, r.CoverMediaID, r.IsPublished, r.SortOrder, r.CreatedAt, r.UpdatedAt, r.InstructorID, r.PriceKobo, r.Currency, r.IsPublishedToUniversity, r.Category, r.Language, r.TotalEnrolled))
	}

	return tracks, nil
}

// GetTrackBySlug retrieves a track by its slug.
func (s *Store) GetTrackBySlug(ctx context.Context, tenantID pgtype.UUID, slug string) (*Track, error) {
	r, err := s.read.GetLearningTrackBySlug(ctx, gen.GetLearningTrackBySlugParams{
		TenantID: tenantID,
		Slug:     slug,
	})
	if err != nil {
		return nil, fmt.Errorf("get track by slug: %w", err)
	}

	t := trackFromFields(r.ID, r.TenantID, r.Title, r.Slug, r.Description, r.Difficulty, r.CoverMediaID, r.IsPublished, r.SortOrder, r.CreatedAt, r.UpdatedAt, r.InstructorID, r.PriceKobo, r.Currency, r.IsPublishedToUniversity, r.Category, r.Language, r.TotalEnrolled)
	return &t, nil
}

// GetTrackByID retrieves a track by its ID.
func (s *Store) GetTrackByID(ctx context.Context, tenantID, id pgtype.UUID) (*Track, error) {
	r, err := s.read.GetLearningTrackByID(ctx, gen.GetLearningTrackByIDParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return nil, fmt.Errorf("get track by id: %w", err)
	}

	t := trackFromFields(r.ID, r.TenantID, r.Title, r.Slug, r.Description, r.Difficulty, r.CoverMediaID, r.IsPublished, r.SortOrder, r.CreatedAt, r.UpdatedAt, r.InstructorID, r.PriceKobo, r.Currency, r.IsPublishedToUniversity, r.Category, r.Language, r.TotalEnrolled)
	return &t, nil
}

// CreateTrack creates a new learning track.
func (s *Store) CreateTrack(ctx context.Context, tenantID pgtype.UUID, input CreateTrackInput) (*Track, error) {
	r, err := s.write.CreateLearningTrack(ctx, gen.CreateLearningTrackParams{
		TenantID:     tenantID,
		Title:        input.Title,
		Slug:         input.Slug,
		Description:  input.Description,
		Difficulty:   input.Difficulty,
		CoverMediaID: input.CoverMediaID,
		IsPublished:  input.IsPublished,
		SortOrder:    input.SortOrder,
	})
	if err != nil {
		return nil, fmt.Errorf("create track: %w", err)
	}

	t := trackFromFields(r.ID, r.TenantID, r.Title, r.Slug, r.Description, r.Difficulty, r.CoverMediaID, r.IsPublished, r.SortOrder, r.CreatedAt, r.UpdatedAt, r.InstructorID, r.PriceKobo, r.Currency, r.IsPublishedToUniversity, r.Category, r.Language, r.TotalEnrolled)
	return &t, nil
}

// UpdateTrack updates an existing learning track.
func (s *Store) UpdateTrack(ctx context.Context, tenantID, id pgtype.UUID, input UpdateTrackInput) (*Track, error) {
	r, err := s.write.UpdateLearningTrack(ctx, gen.UpdateLearningTrackParams{
		TenantID:     tenantID,
		ID:           id,
		Title:        input.Title,
		Description:  input.Description,
		Difficulty:   input.Difficulty,
		CoverMediaID: input.CoverMediaID,
		IsPublished:  input.IsPublished,
		SortOrder:    input.SortOrder,
	})
	if err != nil {
		return nil, fmt.Errorf("update track: %w", err)
	}

	t := trackFromFields(r.ID, r.TenantID, r.Title, r.Slug, r.Description, r.Difficulty, r.CoverMediaID, r.IsPublished, r.SortOrder, r.CreatedAt, r.UpdatedAt, r.InstructorID, r.PriceKobo, r.Currency, r.IsPublishedToUniversity, r.Category, r.Language, r.TotalEnrolled)
	return &t, nil
}

// DeleteTrack soft-deletes a learning track.
func (s *Store) DeleteTrack(ctx context.Context, tenantID, id pgtype.UUID) error {
	tag, err := s.db.Writer.Exec(ctx, `
		DELETE FROM learning_tracks WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	if err != nil {
		return fmt.Errorf("delete track: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// PublishTrack sets the published status of a learning track.
func (s *Store) PublishTrack(ctx context.Context, tenantID, id pgtype.UUID, isPublished bool) error {
	tag, err := s.db.Writer.Exec(ctx, `
		UPDATE learning_tracks
		SET is_published = $1, updated_at = now()
		WHERE tenant_id = $2 AND id = $3
	`, isPublished, tenantID, id)
	if err != nil {
		return fmt.Errorf("publish track: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ListAllTracks lists all tracks including drafts.
func (s *Store) ListAllTracks(ctx context.Context, tenantID pgtype.UUID) ([]Track, error) {
	rows, err := s.read.ListAllLearningTracks(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list all tracks: %w", err)
	}

	tracks := make([]Track, 0, len(rows))

	for _, r := range rows {
		tracks = append(tracks, trackFromFields(r.ID, r.TenantID, r.Title, r.Slug, r.Description, r.Difficulty, r.CoverMediaID, r.IsPublished, r.SortOrder, r.CreatedAt, r.UpdatedAt, r.InstructorID, r.PriceKobo, r.Currency, r.IsPublishedToUniversity, r.Category, r.Language, r.TotalEnrolled))
	}

	return tracks, nil
}

// ---------------------------------------------------------------------------
// Module operations
// ---------------------------------------------------------------------------

// Module represents module.
type Module struct {
	ID          pgtype.UUID `json:"id"`
	TenantID    pgtype.UUID `json:"tenant_id"`
	TrackID     pgtype.UUID `json:"track_id"`
	Title       string      `json:"title"`
	Slug        string      `json:"slug"`
	Description *string     `json:"description,omitempty"`
	SortOrder   int32       `json:"sort_order"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// LessonSummary is the lightweight lesson view nested inside ModuleWithLessons
// for the track-detail response (GET /learn/tracks/:slug). Mirrors the frontend
// LessonSummary type (ADR-060). Content is omitted — only metadata.
type LessonSummary struct {
	ID              pgtype.UUID `json:"id"`
	Slug            string      `json:"slug"`
	Title           string      `json:"title"`
	LessonType      string      `json:"lesson_type"`
	DurationMinutes *int32      `json:"duration_minutes,omitempty"`
	SortOrder       int32       `json:"sort_order"`
	IsPreview       bool        `json:"is_preview"`
}

// ModuleWithLessons is a module with its nested lessons, used in the track-detail
// response (GET /learn/tracks/:slug). Matches the frontend ModuleSummary contract
// (ADR-060): modules carry their lessons so the curriculum accordion renders
// from a single fetch.
type ModuleWithLessons struct {
	ID          pgtype.UUID    `json:"id"`
	Slug        string         `json:"slug"`
	Title       string         `json:"title"`
	Description *string        `json:"description,omitempty"`
	SortOrder   int32          `json:"sort_order"`
	Lessons     []LessonSummary `json:"lessons"`
}

// TrackDetail is the flat DTO returned by GET /learn/tracks/:slug. Per Section 36
// the route returns "track detail: modules, lessons, instructor" in one response.
// The frontend TrackDetail type (ADR-060) expects a flat object with a `modules`
// array where each module carries its `lessons` — NOT the legacy `{track, modules}`
// envelope. `Level` aliases `Difficulty` for the frontend contract.
// Fields not yet backed by DB columns (thumbnail_url, subtitle, overview_html,
// objectives, prerequisites, duration_iso) are included as optional JSON keys
// so the frontend contract is stable; they serialize as null/omitted until the
// corresponding migrations land.
type TrackDetail struct {
	ID           pgtype.UUID        `json:"id"`
	TenantID     pgtype.UUID        `json:"tenant_id"`
	Title        string             `json:"title"`
	Slug         string             `json:"slug"`
	Description  *string            `json:"description,omitempty"`
	Difficulty   string             `json:"difficulty"`
	Level        string             `json:"level"`
	CoverMediaID *pgtype.UUID       `json:"cover_media_id,omitempty"`
	IsPublished  bool               `json:"is_published"`
	SortOrder    int32              `json:"sort_order"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
	Modules      []ModuleWithLessons `json:"modules"`
	// Marketplace / enrollment fields (migration 000166).
	PriceKobo               *int64  `json:"price_kobo,omitempty"`
	Currency                string  `json:"currency,omitempty"`
	EnrolledCount           int32   `json:"enrolled_count,omitempty"`
	IsPublishedToUniversity bool    `json:"is_published_to_university,omitempty"`
	Category                *string `json:"category,omitempty"`
	Language                string  `json:"language,omitempty"`
	// Duration computed from the sum of lesson duration_minutes (ADR-060).
	DurationLabel string `json:"duration_label,omitempty"`
	DurationISO   string `json:"duration_iso,omitempty"`
	// Optional fields the frontend reads with {#if} guards — omitted from JSON
	// when absent so they're undefined on the client (falsy).
	ThumbnailURL   *string `json:"thumbnail_url,omitempty"`
	Subtitle       *string `json:"subtitle,omitempty"`
	InstructorName *string `json:"instructor_name,omitempty"`
}

// CreateModuleInput represents create module input.
type CreateModuleInput struct {
	Title       string  `json:"title"`
	Slug        string  `json:"slug"`
	Description *string `json:"description,omitempty"`
	SortOrder   int32   `json:"sort_order"`
}

// UpdateModuleInput represents update module input.
type UpdateModuleInput struct {
	Title       string  `json:"title"`
	Description *string `json:"description,omitempty"`
	SortOrder   int32   `json:"sort_order"`
}

func moduleFromRow(r gen.LearningModule) Module {
	return Module{
		ID:          r.ID,
		TenantID:    r.TenantID,
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
func (s *Store) ListModulesByTrack(ctx context.Context, tenantID, trackID pgtype.UUID) ([]Module, error) {
	rows, err := s.read.ListLearningModulesByTrack(ctx, gen.ListLearningModulesByTrackParams{
		TenantID: tenantID,
		TrackID:  trackID,
	})
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
// modules, lessons, instructor") for GET /learn/tracks/:slug. It fetches the
// track by slug, its modules, and each module's lessons, then assembles them
// into a single TrackDetail response with nested ModuleWithLessons. This
// matches the frontend TrackDetail contract (ADR-060) — NOT the legacy
// `{track, modules}` envelope.
func (s *Store) GetTrackDetail(ctx context.Context, tenantID pgtype.UUID, slug string) (*TrackDetail, error) {
	track, err := s.GetTrackBySlug(ctx, tenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("get track by slug: %w", err)
	}

	modRows, err := s.read.ListLearningModulesByTrack(ctx, gen.ListLearningModulesByTrackParams{
		TenantID: tenantID,
		TrackID:  track.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("list modules by track: %w", err)
	}

	modules := make([]ModuleWithLessons, 0, len(modRows))
	for _, m := range modRows {
		lessonRows, err := s.read.ListLearningLessonsByModule(ctx, gen.ListLearningLessonsByModuleParams{
			TenantID: tenantID,
			ModuleID: m.ID,
		})
		if err != nil {
			return nil, fmt.Errorf("list lessons by module: %w", err)
		}

		lessons := make([]LessonSummary, 0, len(lessonRows))
		for _, l := range lessonRows {
			// Only include published lessons in the public track-detail view.
			if !l.IsPublished {
				continue
			}
			lessons = append(lessons, LessonSummary{
				ID:              l.ID,
				Slug:            l.Slug,
				Title:           l.Title,
				LessonType:      l.LessonType,
				DurationMinutes: l.DurationMinutes,
				SortOrder:       l.SortOrder,
				// is_preview column not yet migrated (Section 36 spec calls for
				// it); default to false until migration 000NNN adds it.
				IsPreview: false,
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

	return &TrackDetail{
		ID:                      track.ID,
		TenantID:                track.TenantID,
		Title:                   track.Title,
		Slug:                    track.Slug,
		Description:             track.Description,
		Difficulty:              track.Difficulty,
		Level:                   track.Difficulty,
		CoverMediaID:            track.CoverMediaID,
		IsPublished:             track.IsPublished,
		SortOrder:               track.SortOrder,
		CreatedAt:               track.CreatedAt,
		UpdatedAt:               track.UpdatedAt,
		Modules:                 modules,
		PriceKobo:               track.PriceKobo,
		Currency:                track.Currency,
		EnrolledCount:           track.TotalEnrolled,
		IsPublishedToUniversity: track.IsPublishedToUniversity,
		Category:                track.Category,
		Language:                track.Language,
		DurationLabel:           durationLabel,
		DurationISO:             durationISO,
	}, nil
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
func (s *Store) GetModuleBySlug(ctx context.Context, tenantID, trackID pgtype.UUID, slug string) (*Module, error) {
	r, err := s.read.GetLearningModuleBySlug(ctx, gen.GetLearningModuleBySlugParams{
		TenantID: tenantID,
		TrackID:  trackID,
		Slug:     slug,
	})
	if err != nil {
		return nil, fmt.Errorf("get module by slug: %w", err)
	}

	m := moduleFromRow(r)
	return &m, nil
}

// GetModuleByID retrieves a module by its ID.
func (s *Store) GetModuleByID(ctx context.Context, tenantID, id pgtype.UUID) (*Module, error) {
	r, err := s.read.GetLearningModuleByID(ctx, gen.GetLearningModuleByIDParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return nil, fmt.Errorf("get module by id: %w", err)
	}

	m := moduleFromRow(r)
	return &m, nil
}

// CreateModule creates a new module in a track.
func (s *Store) CreateModule(ctx context.Context, tenantID, trackID pgtype.UUID, input CreateModuleInput) (*Module, error) {
	r, err := s.write.CreateLearningModule(ctx, gen.CreateLearningModuleParams{
		TenantID:    tenantID,
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
func (s *Store) UpdateModule(ctx context.Context, tenantID, id pgtype.UUID, input UpdateModuleInput) (*Module, error) {
	r, err := s.write.UpdateLearningModule(ctx, gen.UpdateLearningModuleParams{
		TenantID:    tenantID,
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
func (s *Store) DeleteModule(ctx context.Context, tenantID, id pgtype.UUID) error {
	tag, err := s.db.Writer.Exec(ctx, `
		DELETE FROM learning_modules WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	if err != nil {
		return fmt.Errorf("delete module: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// ListAllModules returns all modules for a tenant (admin view).
func (s *Store) ListAllModules(ctx context.Context, tenantID pgtype.UUID) ([]Module, error) {
	rows, err := s.read.ListAllLearningModules(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list all modules: %w", err)
	}

	modules := make([]Module, 0, len(rows))

	for _, r := range rows {
		modules = append(modules, moduleFromRow(r))
	}

	return modules, nil
}

// ListAllLessons returns all lessons for a tenant (admin view).
func (s *Store) ListAllLessons(ctx context.Context, tenantID pgtype.UUID) ([]Lesson, error) {
	rows, err := s.read.ListAllLearningLessons(ctx, tenantID)
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
	TenantID        pgtype.UUID  `json:"tenant_id"`
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
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

// CreateLessonInput represents create lesson input.
type CreateLessonInput struct {
	Title           string       `json:"title"`
	Slug            string       `json:"slug"`
	LessonType      string       `json:"lesson_type"`
	Content         *string      `json:"content,omitempty"`
	MediaID         pgtype.UUID  `json:"media_id"`
	DurationMinutes *int32       `json:"duration_minutes,omitempty"`
	SortOrder       int32        `json:"sort_order"`
	IsPublished     bool         `json:"is_published"`
}

// UpdateLessonInput represents update lesson input.
type UpdateLessonInput struct {
	Title           string       `json:"title"`
	LessonType      string       `json:"lesson_type"`
	Content         *string      `json:"content,omitempty"`
	MediaID         pgtype.UUID  `json:"media_id"`
	DurationMinutes *int32       `json:"duration_minutes,omitempty"`
	SortOrder       int32        `json:"sort_order"`
	IsPublished     bool         `json:"is_published"`
}

func lessonFromRow(r gen.LearningLesson) Lesson {
	var mediaID *pgtype.UUID
	if r.MediaID.Valid {
		mediaID = &r.MediaID
	}

	return Lesson{
		ID:              r.ID,
		TenantID:        r.TenantID,
		ModuleID:        r.ModuleID,
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

// lessonFromListRow maps a ListAllLearningLessonsRow (with JOIN) to a Lesson.
func lessonFromListRow(r gen.ListAllLearningLessonsRow) Lesson {
	var mediaID *pgtype.UUID
	if r.MediaID.Valid {
		mediaID = &r.MediaID
	}

	return Lesson{
		ID:              r.ID,
		TenantID:        r.TenantID,
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
func (s *Store) ListLessonsByModule(ctx context.Context, tenantID, moduleID pgtype.UUID) ([]Lesson, error) {
	rows, err := s.read.ListLearningLessonsByModule(ctx, gen.ListLearningLessonsByModuleParams{
		TenantID: tenantID,
		ModuleID: moduleID,
	})
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
func (s *Store) GetLessonBySlug(ctx context.Context, tenantID, moduleID pgtype.UUID, slug string) (*Lesson, error) {
	r, err := s.read.GetLearningLessonBySlug(ctx, gen.GetLearningLessonBySlugParams{
		TenantID: tenantID,
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
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	ModuleSlug  string `json:"module_slug"`
}

// LessonDetail is the enriched lesson response for the lesson player
// (ADR-060). It extends Lesson with track/module context, prev/next
// navigation, and content_html (plain-text content rendered to basic HTML).
// The frontend Lesson type expects these fields (contract: §3.3).
type LessonDetail struct {
	Lesson
	ModuleTitle string        `json:"module_title"`
	ModuleSlug  string        `json:"module_slug"`
	TrackID     pgtype.UUID   `json:"track_id"`
	TrackSlug   string        `json:"track_slug"`
	TrackTitle  string        `json:"track_title"`
	ContentHTML string        `json:"content_html,omitempty"`
	IsPreview   bool          `json:"is_preview"`
	PrevLesson  *LessonNavRef `json:"prev_lesson,omitempty"`
	NextLesson  *LessonNavRef `json:"next_lesson,omitempty"`
}

// GetLessonDetail retrieves a lesson by slug with track/module context and
// prev/next navigation, suitable for the lesson player (ADR-060).
func (s *Store) GetLessonDetail(ctx context.Context, tenantID, moduleID pgtype.UUID, slug string) (*LessonDetail, error) {
	r, err := s.read.GetLearningLessonDetailBySlug(ctx, gen.GetLearningLessonDetailBySlugParams{
		TenantID: tenantID,
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
			ID:              r.ID,
			TenantID:        r.TenantID,
			ModuleID:        r.ModuleID,
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
		},
		ModuleTitle: r.ModuleTitle,
		ModuleSlug:  r.ModuleSlug,
		TrackID:     r.TrackID,
		TrackSlug:   r.TrackSlug,
		TrackTitle:  r.TrackTitle,
		IsPreview:   false, // no is_preview column in V1 (migration 000039)
		ContentHTML: renderLessonContentHTML(r.Content),
	}

	// Compute prev/next lesson navigation across the whole track.
	navRows, err := s.read.ListTrackLessonsForNav(ctx, gen.ListTrackLessonsForNavParams{
		TrackID:  r.TrackID,
		TenantID: tenantID,
	})
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

// renderLessonContentHTML converts plain-text lesson content into basic,
// escaped HTML paragraphs. The learning_lessons.content column is plain text
// (migration 000039), not ProseMirror JSON, so the TipTap renderer does not
// apply. Empty/nil content renders to an empty string.
func renderLessonContentHTML(content *string) string {
	if content == nil || *content == "" {
		return ""
	}
	// Escape HTML special characters to prevent stored XSS (§5.1).
	text := *content
	var b strings.Builder
	b.Grow(len(text) + 64)
	for _, r := range text {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&#39;")
		default:
			b.WriteRune(r)
		}
	}
	// Split on blank lines into paragraphs; single-line content gets one <p>.
	escaped := b.String()
	paragraphs := strings.Split(escaped, "\n\n")
	var out strings.Builder
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Preserve single newlines within a paragraph as <br>.
		p = strings.ReplaceAll(p, "\n", "<br>")
		out.WriteString("<p>")
		out.WriteString(p)
		out.WriteString("</p>")
	}
	return out.String()
}

// GetLessonByID retrieves a lesson by its ID.
func (s *Store) GetLessonByID(ctx context.Context, tenantID, id pgtype.UUID) (*Lesson, error) {
	r, err := s.read.GetLearningLessonByID(ctx, gen.GetLearningLessonByIDParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return nil, fmt.Errorf("get lesson by id: %w", err)
	}

	l := lessonFromRow(r)
	return &l, nil
}

// CreateLesson creates a new lesson in a module.
func (s *Store) CreateLesson(ctx context.Context, tenantID, moduleID pgtype.UUID, input CreateLessonInput) (*Lesson, error) {
	r, err := s.write.CreateLearningLesson(ctx, gen.CreateLearningLessonParams{
		TenantID:        tenantID,
		ModuleID:        moduleID,
		Title:           input.Title,
		Slug:            input.Slug,
		LessonType:      input.LessonType,
		Content:         input.Content,
		MediaID:         input.MediaID,
		DurationMinutes: input.DurationMinutes,
		SortOrder:       input.SortOrder,
		IsPublished:     input.IsPublished,
	})
	if err != nil {
		return nil, fmt.Errorf("create lesson: %w", err)
	}

	l := lessonFromRow(r)
	return &l, nil
}

// UpdateLesson updates an existing lesson.
func (s *Store) UpdateLesson(ctx context.Context, tenantID, id pgtype.UUID, input UpdateLessonInput) (*Lesson, error) {
	r, err := s.write.UpdateLearningLesson(ctx, gen.UpdateLearningLessonParams{
		TenantID:        tenantID,
		ID:              id,
		Title:           input.Title,
		LessonType:      input.LessonType,
		Content:         input.Content,
		MediaID:         input.MediaID,
		DurationMinutes: input.DurationMinutes,
		SortOrder:       input.SortOrder,
		IsPublished:     input.IsPublished,
	})
	if err != nil {
		return nil, fmt.Errorf("update lesson: %w", err)
	}

	l := lessonFromRow(r)
	return &l, nil
}

// DeleteLesson soft-deletes a lesson.
func (s *Store) DeleteLesson(ctx context.Context, tenantID, id pgtype.UUID) error {
	tag, err := s.db.Writer.Exec(ctx, `
		DELETE FROM learning_lessons WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
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
	ID          pgtype.UUID `json:"id"`
	TenantID    pgtype.UUID `json:"tenant_id"`
	UserID      pgtype.UUID `json:"user_id"`
	LessonID    pgtype.UUID `json:"lesson_id"`
	Status      string      `json:"status"`
	CompletedAt *time.Time  `json:"completed_at,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
}

// UpdateProgressInput represents update progress input.
type UpdateProgressInput struct {
	Status string `json:"status"`
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
		ID:          r.ID,
		TenantID:    r.TenantID,
		UserID:      r.UserID,
		LessonID:    r.LessonID,
		Status:      r.Status,
		CompletedAt: completedAtFromTimestamptz(r.CompletedAt),
		CreatedAt:   r.CreatedAt.Time,
	}, nil
}

// UpsertProgress creates or updates progress for a lesson.
func (s *Store) UpsertProgress(ctx context.Context, tenantID, userID, lessonID pgtype.UUID, input UpdateProgressInput) (*Progress, error) {
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
		TenantID:    tenantID,
		UserID:      userID,
		LessonID:    lessonID,
		Status:      input.Status,
		CompletedAt: completedAtTS,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert progress: %w", err)
	}

	return &Progress{
		ID:          r.ID,
		TenantID:    r.TenantID,
		UserID:      r.UserID,
		LessonID:    r.LessonID,
		Status:      r.Status,
		CompletedAt: completedAtFromTimestamptz(r.CompletedAt),
		CreatedAt:   r.CreatedAt.Time,
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

// ---------------------------------------------------------------------------
// Certificate operations
// ---------------------------------------------------------------------------

// Certificate represents certificate.
type Certificate struct {
	ID                pgtype.UUID  `json:"id"`
	TenantID          pgtype.UUID  `json:"-"` // internal multi-tenancy key; never serialized (public + /me responses)
	UserID            pgtype.UUID  `json:"user_id"`
	TrackID           pgtype.UUID  `json:"track_id"`
	CertificateNumber string       `json:"certificate_number"`
	MediaID           *pgtype.UUID `json:"media_id,omitempty"`
	IssuedAt          time.Time    `json:"issued_at"`
	Title             *string      `json:"title,omitempty"`
	TrackSlug         *string      `json:"track_slug,omitempty"`
}

func certFromRow(r gen.LearningCertificate) Certificate {
	var mediaID *pgtype.UUID
	if r.MediaID.Valid {
		mediaID = &r.MediaID
	}

	return Certificate{
		ID:                r.ID,
		TenantID:          r.TenantID,
		UserID:            r.UserID,
		TrackID:           r.TrackID,
		CertificateNumber: r.CertificateNumber,
		MediaID:           mediaID,
		IssuedAt:          r.IssuedAt.Time,
	}
}

// certFromListRow maps the joined certificate+track row (used by the public
// profile Certificates tab) to a Certificate, populating Title and TrackSlug
// from the joined learning_tracks columns.
func certFromListRow(r gen.ListLearningCertificatesByUserRow) Certificate {
	var mediaID *pgtype.UUID
	if r.MediaID.Valid {
		mediaID = &r.MediaID
	}

	return Certificate{
		ID:                r.ID,
		TenantID:          r.TenantID,
		UserID:            r.UserID,
		TrackID:           r.TrackID,
		CertificateNumber: r.CertificateNumber,
		MediaID:           mediaID,
		IssuedAt:          r.IssuedAt.Time,
		Title:             r.TrackTitle,
		TrackSlug:         r.TrackSlug,
	}
}

// GetCertificate retrieves a certificate by its ID.
func (s *Store) GetCertificate(ctx context.Context, userID, trackID pgtype.UUID) (*Certificate, error) {
	r, err := s.read.GetLearningCertificate(ctx, gen.GetLearningCertificateParams{
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
func (s *Store) IssueCertificate(ctx context.Context, tenantID, userID, trackID pgtype.UUID, certificateNumber string) (*Certificate, error) {
	r, err := s.write.IssueLearningCertificate(ctx, gen.IssueLearningCertificateParams{
		TenantID:          tenantID,
		UserID:            userID,
		TrackID:           trackID,
		CertificateNumber: certificateNumber,
	})
	if err != nil {
		return nil, fmt.Errorf("issue certificate: %w", err)
	}

	c := certFromRow(r)
	return &c, nil
}

// ListCertificatesByUser lists a user's certificates, tenant-scoped.
func (s *Store) ListCertificatesByUser(ctx context.Context, tenantID, userID pgtype.UUID) ([]Certificate, error) {
	rows, err := s.read.ListLearningCertificatesByUser(ctx, gen.ListLearningCertificatesByUserParams{
		TenantID: tenantID,
		UserID:   userID,
	})
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
	r, err := s.read.GetLearningCertificateByID(ctx, certID)
	if err != nil {
		return nil, fmt.Errorf("get certificate by id: %w", err)
	}

	c := certFromRow(r)
	return &c, nil
}

// ---------------------------------------------------------------------------
// Enrollment operations
// ---------------------------------------------------------------------------

// Enrollment represents a user's enrollment in a learning track.
type Enrollment struct {
	ID          pgtype.UUID `json:"id"`
	UserID      pgtype.UUID `json:"user_id"`
	TrackID     pgtype.UUID `json:"track_id"`
	AccessType string      `json:"access_type"`
	ProgressPct float64     `json:"progress_pct"`
	EnrolledAt  time.Time   `json:"enrolled_at"`
	CompletedAt *time.Time  `json:"completed_at,omitempty"`
}

// EnrollUser creates an enrollment for a user in a track.
func (s *Store) EnrollUser(ctx context.Context, userID, trackID pgtype.UUID, accessType string) (*Enrollment, error) {
	r, err := s.write.CreateEnrollment(ctx, gen.CreateEnrollmentParams{
		UserID:     userID,
		TrackID:    trackID,
		AccessType: accessType,
	})
	if err != nil {
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

// ListEnrollmentsByUser lists all enrollments for a user.
func (s *Store) ListEnrollmentsByUser(ctx context.Context, tenantID, userID pgtype.UUID) ([]Enrollment, error) {
	rows, err := s.read.ListEnrollmentsByUserID(ctx, gen.ListEnrollmentsByUserIDParams{
		UserID:   userID,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("list enrollments by user: %w", err)
	}

	enrollments := make([]Enrollment, 0, len(rows))

	for _, r := range rows {
		progressPct, _ := r.ProgressPct.Float64Value()
		enrollments = append(enrollments, Enrollment{
			ID:          r.ID,
			UserID:      r.UserID,
			TrackID:     r.TrackID,
			AccessType:  r.AccessType,
			ProgressPct: progressPct.Float64,
			EnrolledAt:  r.EnrolledAt.Time,
			CompletedAt: completedAtFromTimestamptz(r.CompletedAt),
		})
	}

	return enrollments, nil
}

// GetEnrollment retrieves a specific enrollment.
func (s *Store) GetEnrollment(ctx context.Context, tenantID, userID, trackID pgtype.UUID) (*Enrollment, error) {
	r, err := s.read.GetEnrollmentByUserAndTrack(ctx, gen.GetEnrollmentByUserAndTrackParams{
		UserID:   userID,
		TrackID:  trackID,
		TenantID: tenantID,
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
	ID           pgtype.UUID        `json:"id"`
	TenantID     pgtype.UUID        `json:"tenant_id"`
	UserID       pgtype.UUID        `json:"user_id"`
	TrackID      pgtype.UUID        `json:"track_id"`
	AccessType   string             `json:"access_type"`
	ProgressPct  pgtype.Numeric     `json:"progress_pct"`
	EnrolledAt   pgtype.Timestamptz `json:"enrolled_at"`
	CompletedAt  pgtype.Timestamptz `json:"completed_at"`
	Username     string             `json:"username"`
	DisplayName  string             `json:"display_name"`
}

// ListEnrollmentsByTrack returns paginated enrollments for a track with user info.
func (s *Store) ListEnrollmentsByTrack(ctx context.Context, tenantID, trackID pgtype.UUID, limit, offset int32) ([]EnrollmentRow, error) {
	rows, err := s.read.ListEnrollmentsByTrackID(ctx, gen.ListEnrollmentsByTrackIDParams{
		TenantID:  tenantID,
		TrackID:   trackID,
		PageLimit: limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing enrollments by track: %w", err)
	}

	result := make([]EnrollmentRow, len(rows))
	for i, r := range rows {
		result[i] = EnrollmentRow{
			ID:          r.ID,
			TenantID:    r.TenantID,
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

// QuizQuestion represents a quiz question.
type QuizQuestion struct {
	ID          pgtype.UUID `json:"id"`
	LessonID    pgtype.UUID `json:"lesson_id"`
	Question    string      `json:"question"`
	Options     string      `json:"options"` // JSON array
	Answer      string      `json:"answer"`
	Explanation *string     `json:"explanation,omitempty"`
	Position    int32       `json:"position"`
}

// GetQuizQuestions retrieves quiz questions for a lesson (without answers).
func (s *Store) GetQuizQuestions(ctx context.Context, tenantID, lessonID pgtype.UUID) ([]QuizQuestion, error) {
	rows, err := s.read.GetQuizQuestionsByLesson(ctx, gen.GetQuizQuestionsByLessonParams{
		TenantID: tenantID,
		LessonID: lessonID,
	})
	if err != nil {
		return nil, fmt.Errorf("get quiz questions: %w", err)
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

// GetQuizQuestionsWithAnswers retrieves quiz questions with correct answers (for grading).
func (s *Store) GetQuizQuestionsWithAnswers(ctx context.Context, tenantID, lessonID pgtype.UUID) ([]QuizQuestion, error) {
	return s.GetQuizQuestions(ctx, tenantID, lessonID)
}

// ---------------------------------------------------------------------------
// Learning track reviews (AUDIT-BE-ROUTE-040)
// Reuse Section 29 reviews with entity_type=learning_track.
// ---------------------------------------------------------------------------

// TrackReview is the API-facing review record for a learning track.
type TrackReview struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	ReviewerID string    `json:"reviewer_user_id"`
	Rating     int16     `json:"rating"`
	Title      *string   `json:"title,omitempty"`
	Body       *string   `json:"body,omitempty"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TrackReviewAggregate holds the aggregate rating for a track.
type TrackReviewAggregate struct {
	TotalReviews int32   `json:"total_reviews"`
	AverageRating float64 `json:"average_rating"`
}

// CreateTrackReview inserts a review for a learning track.
// AUDIT-BE-ROUTE-040: entity_type=learning_track.
func (s *Store) CreateTrackReview(ctx context.Context, tenantID, trackID, reviewerID pgtype.UUID, rating int16, title, body *string) (TrackReview, error) {
	row, err := s.write.CreateReview(ctx, gen.CreateReviewParams{
		TenantID:       tenantID,
		EntityType:     "learning_track",
		EntityID:       trackID,
		ReviewerUserID: reviewerID,
		Rating:         rating,
		Title:          title,
		Body:           body,
	})
	if err != nil {
		return TrackReview{}, err
	}
	return trackReviewFromRow(row), nil
}

// ListTrackReviews lists approved reviews for a learning track.
func (s *Store) ListTrackReviews(ctx context.Context, tenantID, trackID pgtype.UUID, limit, offset int32) ([]TrackReview, error) {
	rows, err := s.read.ListReviewsByEntity(ctx, gen.ListReviewsByEntityParams{
		TenantID:   tenantID,
		EntityType: "learning_track",
		EntityID:   trackID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, err
	}
	reviews := make([]TrackReview, 0, len(rows))
	for _, r := range rows {
		reviews = append(reviews, trackReviewFromRow(r))
	}
	return reviews, nil
}

// GetTrackReviewAggregate returns the total review count and average rating
// for a learning track.
func (s *Store) GetTrackReviewAggregate(ctx context.Context, tenantID, trackID pgtype.UUID) (TrackReviewAggregate, error) {
	rows, err := s.read.ListReviewsByEntity(ctx, gen.ListReviewsByEntityParams{
		TenantID:   tenantID,
		EntityType: "learning_track",
		EntityID:   trackID,
		PageLimit:  10000,
		PageOffset: 0,
	})
	if err != nil {
		return TrackReviewAggregate{}, err
	}
	if len(rows) == 0 {
		return TrackReviewAggregate{}, nil
	}
	var sum float64
	for _, r := range rows {
		sum += float64(r.Rating)
	}
	return TrackReviewAggregate{
		TotalReviews:  int32(len(rows)),
		AverageRating: sum / float64(len(rows)),
	}, nil
}

func trackReviewFromRow(r gen.Review) TrackReview {
	return TrackReview{
		ID:         r.ID.String(),
		TenantID:   r.TenantID.String(),
		EntityType: r.EntityType,
		EntityID:   r.EntityID.String(),
		ReviewerID: r.ReviewerUserID.String(),
		Rating:     r.Rating,
		Title:      r.Title,
		Body:       r.Body,
		Status:     r.Status,
		CreatedAt:  r.CreatedAt.Time,
		UpdatedAt:  r.UpdatedAt.Time,
	}
}
