-- Learning platform queries: tracks, modules, lessons, progress, certificates, enrollments, quizzes.

-- =====================
-- Track Operations
-- =====================

-- name: ListPublishedLearningTracks :many
SELECT id, tenant_id, title, slug, description, difficulty, cover_media_id,
       is_published, sort_order, created_at, updated_at
FROM learning_tracks
WHERE tenant_id = @tenant_id AND is_published = true
ORDER BY sort_order ASC, created_at DESC;

-- name: GetLearningTrackBySlug :one
SELECT id, tenant_id, title, slug, description, difficulty, cover_media_id,
       is_published, sort_order, created_at, updated_at
FROM learning_tracks
WHERE tenant_id = @tenant_id AND slug = @slug;

-- name: GetLearningTrackByID :one
SELECT id, tenant_id, title, slug, description, difficulty, cover_media_id,
       is_published, sort_order, created_at, updated_at
FROM learning_tracks
WHERE tenant_id = @tenant_id AND id = @id;

-- name: CreateLearningTrack :one
INSERT INTO learning_tracks (tenant_id, title, slug, description, difficulty, cover_media_id, is_published, sort_order)
VALUES (@tenant_id, @title, @slug, sqlc.narg(description), @difficulty, sqlc.narg(cover_media_id), @is_published, @sort_order)
RETURNING id, tenant_id, title, slug, description, difficulty, cover_media_id, is_published, sort_order, created_at, updated_at;

-- name: UpdateLearningTrack :one
UPDATE learning_tracks
SET title = COALESCE(@title, title),
    description = COALESCE(sqlc.narg(description), description),
    difficulty = COALESCE(@difficulty, difficulty),
    cover_media_id = COALESCE(sqlc.narg(cover_media_id), cover_media_id),
    is_published = COALESCE(@is_published, is_published),
    sort_order = COALESCE(@sort_order, sort_order),
    updated_at = now()
WHERE tenant_id = @tenant_id AND id = @id
RETURNING id, tenant_id, title, slug, description, difficulty, cover_media_id, is_published, sort_order, created_at, updated_at;

-- name: DeleteLearningTrack :exec
DELETE FROM learning_tracks WHERE tenant_id = @tenant_id AND id = @id;

-- name: ListAllLearningTracks :many
SELECT id, tenant_id, title, slug, description, difficulty, cover_media_id,
       is_published, sort_order, created_at, updated_at
FROM learning_tracks
WHERE tenant_id = @tenant_id
ORDER BY sort_order ASC, created_at DESC;

-- =====================
-- Module Operations
-- =====================

-- name: ListLearningModulesByTrack :many
SELECT id, tenant_id, track_id, title, slug, description, sort_order, created_at, updated_at
FROM learning_modules
WHERE tenant_id = @tenant_id AND track_id = @track_id
ORDER BY sort_order ASC, created_at DESC;

-- name: GetLearningModuleBySlug :one
SELECT id, tenant_id, track_id, title, slug, description, sort_order, created_at, updated_at
FROM learning_modules
WHERE tenant_id = @tenant_id AND track_id = @track_id AND slug = @slug;

-- name: GetLearningModuleByID :one
SELECT id, tenant_id, track_id, title, slug, description, sort_order, created_at, updated_at
FROM learning_modules
WHERE tenant_id = @tenant_id AND id = @id;

-- name: CreateLearningModule :one
INSERT INTO learning_modules (tenant_id, track_id, title, slug, description, sort_order)
VALUES (@tenant_id, @track_id, @title, @slug, sqlc.narg(description), @sort_order)
RETURNING id, tenant_id, track_id, title, slug, description, sort_order, created_at, updated_at;

-- name: UpdateLearningModule :one
UPDATE learning_modules
SET title = COALESCE(@title, title),
    description = COALESCE(sqlc.narg(description), description),
    sort_order = COALESCE(@sort_order, sort_order),
    updated_at = now()
WHERE tenant_id = @tenant_id AND id = @id
RETURNING id, tenant_id, track_id, title, slug, description, sort_order, created_at, updated_at;

-- name: DeleteLearningModule :exec
DELETE FROM learning_modules WHERE tenant_id = @tenant_id AND id = @id;

-- =====================
-- Lesson Operations
-- =====================

-- name: ListLearningLessonsByModule :many
SELECT id, tenant_id, module_id, title, slug, lesson_type, content, media_id,
       duration_minutes, sort_order, is_published, created_at, updated_at
FROM learning_lessons
WHERE tenant_id = @tenant_id AND module_id = @module_id
ORDER BY sort_order ASC, created_at DESC;

-- name: GetLearningLessonBySlug :one
SELECT id, tenant_id, module_id, title, slug, lesson_type, content, media_id,
       duration_minutes, sort_order, is_published, created_at, updated_at
FROM learning_lessons
WHERE tenant_id = @tenant_id AND module_id = @module_id AND slug = @slug;

-- name: GetLearningLessonByID :one
SELECT id, tenant_id, module_id, title, slug, lesson_type, content, media_id,
       duration_minutes, sort_order, is_published, created_at, updated_at
FROM learning_lessons
WHERE tenant_id = @tenant_id AND id = @id;

-- name: CreateLearningLesson :one
INSERT INTO learning_lessons (tenant_id, module_id, title, slug, lesson_type, content, media_id, duration_minutes, sort_order, is_published)
VALUES (@tenant_id, @module_id, @title, @slug, @lesson_type, sqlc.narg(content), sqlc.narg(media_id), sqlc.narg(duration_minutes), @sort_order, @is_published)
RETURNING id, tenant_id, module_id, title, slug, lesson_type, content, media_id, duration_minutes, sort_order, is_published, created_at, updated_at;

-- name: UpdateLearningLesson :one
UPDATE learning_lessons
SET title = COALESCE(@title, title),
    lesson_type = COALESCE(@lesson_type, lesson_type),
    content = COALESCE(sqlc.narg(content), content),
    media_id = COALESCE(sqlc.narg(media_id), media_id),
    duration_minutes = COALESCE(sqlc.narg(duration_minutes), duration_minutes),
    sort_order = COALESCE(@sort_order, sort_order),
    is_published = COALESCE(@is_published, is_published),
    updated_at = now()
WHERE tenant_id = @tenant_id AND id = @id
RETURNING id, tenant_id, module_id, title, slug, lesson_type, content, media_id, duration_minutes, sort_order, is_published, created_at, updated_at;

-- name: DeleteLearningLesson :exec
DELETE FROM learning_lessons WHERE tenant_id = @tenant_id AND id = @id;

-- =====================
-- Progress Operations
-- =====================

-- name: GetLessonProgress :one
SELECT id, tenant_id, user_id, lesson_id, status, completed_at, created_at
FROM lesson_progress
WHERE user_id = @user_id AND lesson_id = @lesson_id;

-- name: UpsertLessonProgress :one
INSERT INTO lesson_progress (tenant_id, user_id, lesson_id, status, completed_at)
VALUES (@tenant_id, @user_id, @lesson_id, @status, sqlc.narg(completed_at))
ON CONFLICT (user_id, lesson_id) DO UPDATE
SET status = EXCLUDED.status,
    completed_at = COALESCE(EXCLUDED.completed_at, lesson_progress.completed_at)
RETURNING id, tenant_id, user_id, lesson_id, status, completed_at, created_at;

-- name: GetUserTrackProgress :one
SELECT
    COUNT(*) FILTER (WHERE lp.status = 'completed'),
    COUNT(*)
FROM learning_lessons ll
JOIN learning_modules lm ON lm.id = ll.module_id
LEFT JOIN lesson_progress lp ON lp.lesson_id = ll.id AND lp.user_id = @user_id
WHERE lm.track_id = @track_id AND ll.is_published = true;

-- =====================
-- Certificate Operations
-- =====================

-- name: GetLearningCertificate :one
SELECT id, tenant_id, user_id, track_id, certificate_number, media_id, issued_at
FROM learning_certificates
WHERE user_id = @user_id AND track_id = @track_id;

-- name: IssueLearningCertificate :one
INSERT INTO learning_certificates (tenant_id, user_id, track_id, certificate_number)
VALUES (@tenant_id, @user_id, @track_id, @certificate_number)
ON CONFLICT DO NOTHING
RETURNING id, tenant_id, user_id, track_id, certificate_number, media_id, issued_at;

-- name: ListLearningCertificatesByUser :many
SELECT id, tenant_id, user_id, track_id, certificate_number, media_id, issued_at
FROM learning_certificates
WHERE tenant_id = @tenant_id AND user_id = @user_id
ORDER BY issued_at DESC;

-- name: GetLearningCertificateByID :one
SELECT id, tenant_id, user_id, track_id, certificate_number, media_id, issued_at
FROM learning_certificates
WHERE id = @id;

-- =====================
-- Enrollment Operations
-- =====================

-- name: CreateEnrollment :one
INSERT INTO enrollments (tenant_id, user_id, track_id, access_type)
VALUES (@tenant_id, @user_id, @track_id, @access_type)
ON CONFLICT (user_id, track_id) DO NOTHING
RETURNING id, tenant_id, user_id, track_id, access_type, progress_pct, enrolled_at, completed_at;

-- name: ListEnrollmentsByUserID :many
SELECT id, tenant_id, user_id, track_id, access_type, progress_pct, enrolled_at, completed_at
FROM enrollments
WHERE user_id = @user_id AND tenant_id = @tenant_id
ORDER BY enrolled_at DESC;

-- name: GetEnrollmentByUserAndTrack :one
SELECT id, tenant_id, user_id, track_id, access_type, progress_pct, enrolled_at, completed_at
FROM enrollments
WHERE user_id = @user_id AND track_id = @track_id AND tenant_id = @tenant_id;

-- name: UpdateEnrollmentProgress :exec
UPDATE enrollments
SET progress_pct = @progress_pct,
    completed_at = CASE WHEN @progress_pct >= 100 THEN now() ELSE completed_at END
WHERE user_id = @user_id AND track_id = @track_id;

-- name: ListEnrollmentsByTrackID :many
SELECT e.id, e.tenant_id, e.user_id, e.track_id, e.access_type, e.progress_pct, e.enrolled_at, e.completed_at,
       u.username,
       COALESCE(up.display_name, '') AS display_name
FROM enrollments e
JOIN users u ON u.id = e.user_id
LEFT JOIN user_profiles up ON up.user_id = u.id AND up.tenant_id = e.tenant_id
WHERE e.track_id = @track_id AND e.tenant_id = @tenant_id
ORDER BY e.enrolled_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================
-- Quiz Operations
-- =====================

-- name: GetQuizQuestionsByLesson :many
SELECT id, tenant_id, lesson_id, question, options, answer, explanation, position, created_at
FROM lesson_quizzes
WHERE lesson_id = @lesson_id AND tenant_id = @tenant_id
ORDER BY position ASC;

-- =====================
-- Admin List Operations
-- =====================

-- name: ListAllLearningModules :many
SELECT id, tenant_id, track_id, title, slug, description, sort_order, created_at, updated_at
FROM learning_modules
WHERE tenant_id = $1
ORDER BY sort_order ASC, created_at DESC;

-- name: ListAllLearningLessons :many
SELECT ll.id, ll.tenant_id, ll.module_id, lm.title AS module_title, ll.title, ll.slug,
       ll.lesson_type, ll.content, ll.media_id, ll.duration_minutes, ll.sort_order,
       ll.is_published, ll.created_at, ll.updated_at
FROM learning_lessons ll
LEFT JOIN learning_modules lm ON lm.id = ll.module_id AND lm.tenant_id = ll.tenant_id
WHERE ll.tenant_id = $1
ORDER BY ll.sort_order ASC, ll.created_at DESC;
