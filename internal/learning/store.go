package learning

import (
	"context"
	"fmt"
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
	CoverMediaID *pgtype.UUID `json:"cover_media_id,omitempty"`
	IsPublished  bool         `json:"is_published"`
	SortOrder    int32        `json:"sort_order"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
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

func trackFromRow(r gen.LearningTrack) Track {
	var coverMediaID *pgtype.UUID
	if r.CoverMediaID.Valid {
		coverMediaID = &r.CoverMediaID
	}

	return Track{
		ID:           r.ID,
		TenantID:     r.TenantID,
		Title:        r.Title,
		Slug:         r.Slug,
		Description:  r.Description,
		Difficulty:   r.Difficulty,
		CoverMediaID: coverMediaID,
		IsPublished:  r.IsPublished,
		SortOrder:    r.SortOrder,
		CreatedAt:    r.CreatedAt.Time,
		UpdatedAt:    r.UpdatedAt.Time,
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
		tracks = append(tracks, trackFromRow(r))
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

	t := trackFromRow(r)
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

	t := trackFromRow(r)
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

	t := trackFromRow(r)
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

	t := trackFromRow(r)
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

// ListAllTracks lists all tracks including drafts.
func (s *Store) ListAllTracks(ctx context.Context, tenantID pgtype.UUID) ([]Track, error) {
	rows, err := s.read.ListAllLearningTracks(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list all tracks: %w", err)
	}

	tracks := make([]Track, 0, len(rows))

	for _, r := range rows {
		tracks = append(tracks, trackFromRow(r))
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

// ---------------------------------------------------------------------------
// Lesson operations
// ---------------------------------------------------------------------------

// Lesson represents lesson.
type Lesson struct {
	ID              pgtype.UUID  `json:"id"`
	TenantID        pgtype.UUID  `json:"tenant_id"`
	ModuleID        pgtype.UUID  `json:"module_id"`
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
	TenantID          pgtype.UUID  `json:"tenant_id"`
	UserID            pgtype.UUID  `json:"user_id"`
	TrackID           pgtype.UUID  `json:"track_id"`
	CertificateNumber string       `json:"certificate_number"`
	MediaID           *pgtype.UUID `json:"media_id,omitempty"`
	IssuedAt          time.Time    `json:"issued_at"`
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

// ListCertificatesByUser lists certificates for a user.
func (s *Store) ListCertificatesByUser(ctx context.Context, userID pgtype.UUID) ([]Certificate, error) {
	rows, err := s.read.ListLearningCertificatesByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list certificates by user: %w", err)
	}

	certs := make([]Certificate, 0, len(rows))

	for _, r := range rows {
		certs = append(certs, certFromRow(r))
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
func (s *Store) ListEnrollmentsByUser(ctx context.Context, userID pgtype.UUID) ([]Enrollment, error) {
	rows, err := s.read.ListEnrollmentsByUserID(ctx, gen.ListEnrollmentsByUserIDParams{
		UserID: userID,
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
