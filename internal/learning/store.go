package learning

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

// Store provides direct pgx CRUD for learning tracks, modules, lessons,
// progress records, and certificates. All queries are tenant-scoped via
// the tenant_id column and use the shared read/write connection pools.
type Store struct {
	db *db.DB
}

// NewStore creates a learning store bound to the provided database pools.
func NewStore(database *db.DB) *Store {
	return &Store{db: database}
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
	Title        string       `json:"title"`
	Slug         string       `json:"slug"`
	Description  *string      `json:"description,omitempty"`
	Difficulty   string       `json:"difficulty"`
	CoverMediaID *pgtype.UUID `json:"cover_media_id,omitempty"`
	IsPublished  bool         `json:"is_published"`
	SortOrder    int32        `json:"sort_order"`
}

// UpdateTrackInput represents update track input.
type UpdateTrackInput struct {
	Title        *string      `json:"title,omitempty"`
	Description  *string      `json:"description,omitempty"`
	Difficulty   *string      `json:"difficulty,omitempty"`
	CoverMediaID *pgtype.UUID `json:"cover_media_id,omitempty"`
	IsPublished  *bool        `json:"is_published,omitempty"`
	SortOrder    *int32       `json:"sort_order,omitempty"`
}

// ListPublishedTracks lists all published tracks for a tenant.
func (s *Store) ListPublishedTracks(ctx context.Context, tenantID pgtype.UUID) ([]Track, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT id, tenant_id, title, slug, description, difficulty, cover_media_id,
		       is_published, sort_order, created_at, updated_at
		FROM learning_tracks
		WHERE tenant_id = $1 AND is_published = true
		ORDER BY sort_order ASC, created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list published tracks: %w", err)
	}
	defer rows.Close()

	var tracks []Track

	for rows.Next() {
		var t Track
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.Title, &t.Slug, &t.Description,
			&t.Difficulty, &t.CoverMediaID, &t.IsPublished, &t.SortOrder,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan track: %w", err)
		}

		tracks = append(tracks, t)
	}

	return tracks, rows.Err()
}

// GetTrackBySlug retrieves a track by its slug.
func (s *Store) GetTrackBySlug(ctx context.Context, tenantID pgtype.UUID, slug string) (*Track, error) {
	var t Track

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, title, slug, description, difficulty, cover_media_id,
		       is_published, sort_order, created_at, updated_at
		FROM learning_tracks
		WHERE tenant_id = $1 AND slug = $2
	`, tenantID, slug).Scan(
		&t.ID, &t.TenantID, &t.Title, &t.Slug, &t.Description,
		&t.Difficulty, &t.CoverMediaID, &t.IsPublished, &t.SortOrder,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get track by slug: %w", err)
	}

	return &t, nil
}

// GetTrackByID retrieves a track by its ID.
func (s *Store) GetTrackByID(ctx context.Context, tenantID, id pgtype.UUID) (*Track, error) {
	var t Track

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, title, slug, description, difficulty, cover_media_id,
		       is_published, sort_order, created_at, updated_at
		FROM learning_tracks
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id).Scan(
		&t.ID, &t.TenantID, &t.Title, &t.Slug, &t.Description,
		&t.Difficulty, &t.CoverMediaID, &t.IsPublished, &t.SortOrder,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get track by id: %w", err)
	}

	return &t, nil
}

// CreateTrack creates a new learning track.
func (s *Store) CreateTrack(ctx context.Context, tenantID pgtype.UUID, input CreateTrackInput) (*Track, error) {
	var t Track

	err := s.db.Writer.QueryRow(ctx, `
		INSERT INTO learning_tracks (tenant_id, title, slug, description, difficulty, cover_media_id, is_published, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, tenant_id, title, slug, description, difficulty, cover_media_id, is_published, sort_order, created_at, updated_at
	`, tenantID, input.Title, input.Slug, input.Description, input.Difficulty, input.CoverMediaID, input.IsPublished, input.SortOrder).Scan(
		&t.ID, &t.TenantID, &t.Title, &t.Slug, &t.Description,
		&t.Difficulty, &t.CoverMediaID, &t.IsPublished, &t.SortOrder,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create track: %w", err)
	}

	return &t, nil
}

// UpdateTrack updates an existing learning track.
func (s *Store) UpdateTrack(ctx context.Context, tenantID, id pgtype.UUID, input UpdateTrackInput) (*Track, error) {
	var t Track

	err := s.db.Writer.QueryRow(ctx, `
		UPDATE learning_tracks
		SET title = COALESCE($3, title),
		    description = COALESCE($4, description),
		    difficulty = COALESCE($5, difficulty),
		    cover_media_id = COALESCE($6, cover_media_id),
		    is_published = COALESCE($7, is_published),
		    sort_order = COALESCE($8, sort_order),
		    updated_at = now()
		WHERE tenant_id = $1 AND id = $2
		RETURNING id, tenant_id, title, slug, description, difficulty, cover_media_id, is_published, sort_order, created_at, updated_at
	`, tenantID, id, input.Title, input.Description, input.Difficulty, input.CoverMediaID, input.IsPublished, input.SortOrder).Scan(
		&t.ID, &t.TenantID, &t.Title, &t.Slug, &t.Description,
		&t.Difficulty, &t.CoverMediaID, &t.IsPublished, &t.SortOrder,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("update track: %w", err)
	}

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
	rows, err := s.db.Reader.Query(ctx, `
		SELECT id, tenant_id, title, slug, description, difficulty, cover_media_id,
		       is_published, sort_order, created_at, updated_at
		FROM learning_tracks
		WHERE tenant_id = $1
		ORDER BY sort_order ASC, created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list all tracks: %w", err)
	}
	defer rows.Close()

	var tracks []Track

	for rows.Next() {
		var t Track
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.Title, &t.Slug, &t.Description,
			&t.Difficulty, &t.CoverMediaID, &t.IsPublished, &t.SortOrder,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan track: %w", err)
		}

		tracks = append(tracks, t)
	}

	return tracks, rows.Err()
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
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	SortOrder   *int32  `json:"sort_order,omitempty"`
}

// ListModulesByTrack lists all modules for a track.
func (s *Store) ListModulesByTrack(ctx context.Context, tenantID, trackID pgtype.UUID) ([]Module, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT id, tenant_id, track_id, title, slug, description, sort_order, created_at, updated_at
		FROM learning_modules
		WHERE tenant_id = $1 AND track_id = $2
		ORDER BY sort_order ASC, created_at DESC
	`, tenantID, trackID)
	if err != nil {
		return nil, fmt.Errorf("list modules by track: %w", err)
	}
	defer rows.Close()

	var modules []Module

	for rows.Next() {
		var m Module
		if err := rows.Scan(
			&m.ID, &m.TenantID, &m.TrackID, &m.Title, &m.Slug,
			&m.Description, &m.SortOrder, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan module: %w", err)
		}

		modules = append(modules, m)
	}

	return modules, rows.Err()
}

// GetModuleBySlug retrieves a module by its slug.
func (s *Store) GetModuleBySlug(ctx context.Context, tenantID, trackID pgtype.UUID, slug string) (*Module, error) {
	var m Module

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, track_id, title, slug, description, sort_order, created_at, updated_at
		FROM learning_modules
		WHERE tenant_id = $1 AND track_id = $2 AND slug = $3
	`, tenantID, trackID, slug).Scan(
		&m.ID, &m.TenantID, &m.TrackID, &m.Title, &m.Slug,
		&m.Description, &m.SortOrder, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get module by slug: %w", err)
	}

	return &m, nil
}

// GetModuleByID retrieves a module by its ID.
func (s *Store) GetModuleByID(ctx context.Context, tenantID, id pgtype.UUID) (*Module, error) {
	var m Module

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, track_id, title, slug, description, sort_order, created_at, updated_at
		FROM learning_modules
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id).Scan(
		&m.ID, &m.TenantID, &m.TrackID, &m.Title, &m.Slug,
		&m.Description, &m.SortOrder, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get module by id: %w", err)
	}

	return &m, nil
}

// CreateModule creates a new module in a track.
func (s *Store) CreateModule(ctx context.Context, tenantID, trackID pgtype.UUID, input CreateModuleInput) (*Module, error) {
	var m Module

	err := s.db.Writer.QueryRow(ctx, `
		INSERT INTO learning_modules (tenant_id, track_id, title, slug, description, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, track_id, title, slug, description, sort_order, created_at, updated_at
	`, tenantID, trackID, input.Title, input.Slug, input.Description, input.SortOrder).Scan(
		&m.ID, &m.TenantID, &m.TrackID, &m.Title, &m.Slug,
		&m.Description, &m.SortOrder, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create module: %w", err)
	}

	return &m, nil
}

// UpdateModule updates an existing module.
func (s *Store) UpdateModule(ctx context.Context, tenantID, id pgtype.UUID, input UpdateModuleInput) (*Module, error) {
	var m Module

	err := s.db.Writer.QueryRow(ctx, `
		UPDATE learning_modules
		SET title = COALESCE($3, title),
		    description = COALESCE($4, description),
		    sort_order = COALESCE($5, sort_order),
		    updated_at = now()
		WHERE tenant_id = $1 AND id = $2
		RETURNING id, tenant_id, track_id, title, slug, description, sort_order, created_at, updated_at
	`, tenantID, id, input.Title, input.Description, input.SortOrder).Scan(
		&m.ID, &m.TenantID, &m.TrackID, &m.Title, &m.Slug,
		&m.Description, &m.SortOrder, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("update module: %w", err)
	}

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
	MediaID         *pgtype.UUID `json:"media_id,omitempty"`
	DurationMinutes *int32       `json:"duration_minutes,omitempty"`
	SortOrder       int32        `json:"sort_order"`
	IsPublished     bool         `json:"is_published"`
}

// UpdateLessonInput represents update lesson input.
type UpdateLessonInput struct {
	Title           *string      `json:"title,omitempty"`
	LessonType      *string      `json:"lesson_type,omitempty"`
	Content         *string      `json:"content,omitempty"`
	MediaID         *pgtype.UUID `json:"media_id,omitempty"`
	DurationMinutes *int32       `json:"duration_minutes,omitempty"`
	SortOrder       *int32       `json:"sort_order,omitempty"`
	IsPublished     *bool        `json:"is_published,omitempty"`
}

// ListLessonsByModule lists all lessons for a module.
func (s *Store) ListLessonsByModule(ctx context.Context, tenantID, moduleID pgtype.UUID) ([]Lesson, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT id, tenant_id, module_id, title, slug, lesson_type, content, media_id,
		       duration_minutes, sort_order, is_published, created_at, updated_at
		FROM learning_lessons
		WHERE tenant_id = $1 AND module_id = $2
		ORDER BY sort_order ASC, created_at DESC
	`, tenantID, moduleID)
	if err != nil {
		return nil, fmt.Errorf("list lessons by module: %w", err)
	}
	defer rows.Close()

	var lessons []Lesson

	for rows.Next() {
		var l Lesson
		if err := rows.Scan(
			&l.ID, &l.TenantID, &l.ModuleID, &l.Title, &l.Slug,
			&l.LessonType, &l.Content, &l.MediaID, &l.DurationMinutes,
			&l.SortOrder, &l.IsPublished, &l.CreatedAt, &l.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan lesson: %w", err)
		}

		lessons = append(lessons, l)
	}

	return lessons, rows.Err()
}

// GetLessonBySlug retrieves a lesson by its slug.
func (s *Store) GetLessonBySlug(ctx context.Context, tenantID, moduleID pgtype.UUID, slug string) (*Lesson, error) {
	var l Lesson

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, module_id, title, slug, lesson_type, content, media_id,
		       duration_minutes, sort_order, is_published, created_at, updated_at
		FROM learning_lessons
		WHERE tenant_id = $1 AND module_id = $2 AND slug = $3
	`, tenantID, moduleID, slug).Scan(
		&l.ID, &l.TenantID, &l.ModuleID, &l.Title, &l.Slug,
		&l.LessonType, &l.Content, &l.MediaID, &l.DurationMinutes,
		&l.SortOrder, &l.IsPublished, &l.CreatedAt, &l.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get lesson by slug: %w", err)
	}

	return &l, nil
}

// GetLessonByID retrieves a lesson by its ID.
func (s *Store) GetLessonByID(ctx context.Context, tenantID, id pgtype.UUID) (*Lesson, error) {
	var l Lesson

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, module_id, title, slug, lesson_type, content, media_id,
		       duration_minutes, sort_order, is_published, created_at, updated_at
		FROM learning_lessons
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id).Scan(
		&l.ID, &l.TenantID, &l.ModuleID, &l.Title, &l.Slug,
		&l.LessonType, &l.Content, &l.MediaID, &l.DurationMinutes,
		&l.SortOrder, &l.IsPublished, &l.CreatedAt, &l.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get lesson by id: %w", err)
	}

	return &l, nil
}

// CreateLesson creates a new lesson in a module.
func (s *Store) CreateLesson(ctx context.Context, tenantID, moduleID pgtype.UUID, input CreateLessonInput) (*Lesson, error) {
	var l Lesson

	err := s.db.Writer.QueryRow(ctx, `
		INSERT INTO learning_lessons (tenant_id, module_id, title, slug, lesson_type, content, media_id, duration_minutes, sort_order, is_published)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, tenant_id, module_id, title, slug, lesson_type, content, media_id, duration_minutes, sort_order, is_published, created_at, updated_at
	`, tenantID, moduleID, input.Title, input.Slug, input.LessonType, input.Content, input.MediaID, input.DurationMinutes, input.SortOrder, input.IsPublished).Scan(
		&l.ID, &l.TenantID, &l.ModuleID, &l.Title, &l.Slug,
		&l.LessonType, &l.Content, &l.MediaID, &l.DurationMinutes,
		&l.SortOrder, &l.IsPublished, &l.CreatedAt, &l.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create lesson: %w", err)
	}

	return &l, nil
}

// UpdateLesson updates an existing lesson.
func (s *Store) UpdateLesson(ctx context.Context, tenantID, id pgtype.UUID, input UpdateLessonInput) (*Lesson, error) {
	var l Lesson

	err := s.db.Writer.QueryRow(ctx, `
		UPDATE learning_lessons
		SET title = COALESCE($3, title),
		    lesson_type = COALESCE($4, lesson_type),
		    content = COALESCE($5, content),
		    media_id = COALESCE($6, media_id),
		    duration_minutes = COALESCE($7, duration_minutes),
		    sort_order = COALESCE($8, sort_order),
		    is_published = COALESCE($9, is_published),
		    updated_at = now()
		WHERE tenant_id = $1 AND id = $2
		RETURNING id, tenant_id, module_id, title, slug, lesson_type, content, media_id, duration_minutes, sort_order, is_published, created_at, updated_at
	`, tenantID, id, input.Title, input.LessonType, input.Content, input.MediaID, input.DurationMinutes, input.SortOrder, input.IsPublished).Scan(
		&l.ID, &l.TenantID, &l.ModuleID, &l.Title, &l.Slug,
		&l.LessonType, &l.Content, &l.MediaID, &l.DurationMinutes,
		&l.SortOrder, &l.IsPublished, &l.CreatedAt, &l.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("update lesson: %w", err)
	}

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
	var p Progress

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, user_id, lesson_id, status, completed_at, created_at
		FROM lesson_progress
		WHERE user_id = $1 AND lesson_id = $2
	`, userID, lessonID).Scan(
		&p.ID, &p.TenantID, &p.UserID, &p.LessonID, &p.Status,
		&p.CompletedAt, &p.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get progress: %w", err)
	}

	return &p, nil
}

// UpsertProgress creates or updates progress for a lesson.
func (s *Store) UpsertProgress(ctx context.Context, tenantID, userID, lessonID pgtype.UUID, input UpdateProgressInput) (*Progress, error) {
	var completedAt *time.Time

	if input.Status == "completed" {
		now := time.Now()
		completedAt = &now
	}

	var p Progress

	err := s.db.Writer.QueryRow(ctx, `
		INSERT INTO lesson_progress (tenant_id, user_id, lesson_id, status, completed_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, lesson_id) DO UPDATE
		SET status = EXCLUDED.status,
		    completed_at = COALESCE(EXCLUDED.completed_at, lesson_progress.completed_at)
		RETURNING id, tenant_id, user_id, lesson_id, status, completed_at, created_at
	`, tenantID, userID, lessonID, input.Status, completedAt).Scan(
		&p.ID, &p.TenantID, &p.UserID, &p.LessonID, &p.Status,
		&p.CompletedAt, &p.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert progress: %w", err)
	}

	return &p, nil
}

// GetUserTrackProgress retrieves track progress for a user.
func (s *Store) GetUserTrackProgress(ctx context.Context, userID, trackID pgtype.UUID) (completed, total int, err error) {
	err = s.db.Reader.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE lp.status = 'completed'),
			COUNT(*)
		FROM learning_lessons ll
		JOIN learning_modules lm ON lm.id = ll.module_id
		LEFT JOIN lesson_progress lp ON lp.lesson_id = ll.id AND lp.user_id = $1
		WHERE lm.track_id = $2 AND ll.is_published = true
	`, userID, trackID).Scan(&completed, &total)
	if err != nil {
		return 0, 0, fmt.Errorf("get user track progress: %w", err)
	}

	return completed, total, nil
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

// GetCertificate retrieves a certificate by its ID.
func (s *Store) GetCertificate(ctx context.Context, userID, trackID pgtype.UUID) (*Certificate, error) {
	var c Certificate

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, user_id, track_id, certificate_number, media_id, issued_at
		FROM learning_certificates
		WHERE user_id = $1 AND track_id = $2
	`, userID, trackID).Scan(
		&c.ID, &c.TenantID, &c.UserID, &c.TrackID,
		&c.CertificateNumber, &c.MediaID, &c.IssuedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get certificate: %w", err)
	}

	return &c, nil
}

// IssueCertificate issues a certificate for a completed track.
func (s *Store) IssueCertificate(ctx context.Context, tenantID, userID, trackID pgtype.UUID, certificateNumber string) (*Certificate, error) {
	var c Certificate

	err := s.db.Writer.QueryRow(ctx, `
		INSERT INTO learning_certificates (tenant_id, user_id, track_id, certificate_number)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING
		RETURNING id, tenant_id, user_id, track_id, certificate_number, media_id, issued_at
	`, tenantID, userID, trackID, certificateNumber).Scan(
		&c.ID, &c.TenantID, &c.UserID, &c.TrackID,
		&c.CertificateNumber, &c.MediaID, &c.IssuedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("issue certificate: %w", err)
	}

	return &c, nil
}

// ListCertificatesByUser lists certificates for a user.
func (s *Store) ListCertificatesByUser(ctx context.Context, userID pgtype.UUID) ([]Certificate, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT id, tenant_id, user_id, track_id, certificate_number, media_id, issued_at
		FROM learning_certificates
		WHERE user_id = $1
		ORDER BY issued_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list certificates by user: %w", err)
	}
	defer rows.Close()

	var certs []Certificate

	for rows.Next() {
		var c Certificate
		if err := rows.Scan(
			&c.ID, &c.TenantID, &c.UserID, &c.TrackID,
			&c.CertificateNumber, &c.MediaID, &c.IssuedAt,
		); err != nil {
			return nil, fmt.Errorf("scan certificate: %w", err)
		}

		certs = append(certs, c)
	}

	return certs, rows.Err()
}

// GetCertificateByID retrieves a certificate by its ID.
func (s *Store) GetCertificateByID(ctx context.Context, certID pgtype.UUID) (*Certificate, error) {
	var c Certificate

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, user_id, track_id, certificate_number, media_id, issued_at
		FROM learning_certificates
		WHERE id = $1
	`, certID).Scan(
		&c.ID, &c.TenantID, &c.UserID, &c.TrackID,
		&c.CertificateNumber, &c.MediaID, &c.IssuedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get certificate by id: %w", err)
	}

	return &c, nil
}

// ---------------------------------------------------------------------------
// Enrollment operations
// ---------------------------------------------------------------------------

// Enrollment represents a user's enrollment in a learning track.
type Enrollment struct {
	ID           pgtype.UUID `json:"id"`
	UserID       pgtype.UUID `json:"user_id"`
	TrackID      pgtype.UUID `json:"track_id"`
	AccessType   string      `json:"access_type"`
	ProgressPct  float64     `json:"progress_pct"`
	EnrolledAt   time.Time   `json:"enrolled_at"`
	CompletedAt  *time.Time  `json:"completed_at,omitempty"`
}

// EnrollUser creates an enrollment for a user in a track.
func (s *Store) EnrollUser(ctx context.Context, userID, trackID pgtype.UUID, accessType string) (*Enrollment, error) {
	var e Enrollment

	err := s.db.Writer.QueryRow(ctx, `
		INSERT INTO enrollments (user_id, track_id, access_type)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, track_id) DO NOTHING
		RETURNING id, user_id, track_id, access_type, progress_pct, enrolled_at, completed_at
	`, userID, trackID, accessType).Scan(
		&e.ID, &e.UserID, &e.TrackID, &e.AccessType,
		&e.ProgressPct, &e.EnrolledAt, &e.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("enroll user: %w", err)
	}

	return &e, nil
}

// ListEnrollmentsByUser lists all enrollments for a user.
func (s *Store) ListEnrollmentsByUser(ctx context.Context, userID pgtype.UUID) ([]Enrollment, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT id, user_id, track_id, access_type, progress_pct, enrolled_at, completed_at
		FROM enrollments
		WHERE user_id = $1
		ORDER BY enrolled_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list enrollments by user: %w", err)
	}
	defer rows.Close()

	var enrollments []Enrollment

	for rows.Next() {
		var e Enrollment
		if err := rows.Scan(
			&e.ID, &e.UserID, &e.TrackID, &e.AccessType,
			&e.ProgressPct, &e.EnrolledAt, &e.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan enrollment: %w", err)
		}

		enrollments = append(enrollments, e)
	}

	return enrollments, rows.Err()
}

// GetEnrollment retrieves a specific enrollment.
func (s *Store) GetEnrollment(ctx context.Context, userID, trackID pgtype.UUID) (*Enrollment, error) {
	var e Enrollment

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, user_id, track_id, access_type, progress_pct, enrolled_at, completed_at
		FROM enrollments
		WHERE user_id = $1 AND track_id = $2
	`, userID, trackID).Scan(
		&e.ID, &e.UserID, &e.TrackID, &e.AccessType,
		&e.ProgressPct, &e.EnrolledAt, &e.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get enrollment: %w", err)
	}

	return &e, nil
}

// UpdateEnrollmentProgress updates the progress percentage of an enrollment.
func (s *Store) UpdateEnrollmentProgress(ctx context.Context, enrollmentID pgtype.UUID, progressPct float64) error {
	_, err := s.db.Writer.Exec(ctx, `
		UPDATE enrollments
		SET progress_pct = $2,
		    completed_at = CASE WHEN $2 >= 100 THEN now() ELSE completed_at END
		WHERE id = $1
	`, enrollmentID, progressPct)
	if err != nil {
		return fmt.Errorf("update enrollment progress: %w", err)
	}

	return nil
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
	Explanation *string     `json:"explanation,omitempty"`
	Position    int32       `json:"position"`
}

// GetQuizQuestions retrieves quiz questions for a lesson (without answers).
func (s *Store) GetQuizQuestions(ctx context.Context, lessonID pgtype.UUID) ([]QuizQuestion, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT id, lesson_id, question, options, explanation, position
		FROM lesson_quizzes
		WHERE lesson_id = $1
		ORDER BY position ASC
	`, lessonID)
	if err != nil {
		return nil, fmt.Errorf("get quiz questions: %w", err)
	}
	defer rows.Close()

	var questions []QuizQuestion

	for rows.Next() {
		var q QuizQuestion
		if err := rows.Scan(
			&q.ID, &q.LessonID, &q.Question, &q.Options,
			&q.Explanation, &q.Position,
		); err != nil {
			return nil, fmt.Errorf("scan quiz question: %w", err)
		}

		questions = append(questions, q)
	}

	return questions, rows.Err()
}

// GetQuizQuestionsWithAnswers retrieves quiz questions with correct answers (for grading).
func (s *Store) GetQuizQuestionsWithAnswers(ctx context.Context, lessonID pgtype.UUID) ([]QuizQuestion, error) {
	return s.GetQuizQuestions(ctx, lessonID)
}
