-- name: ListPublishedAcademyTracks :many
SELECT id, title, slug, description, difficulty, cover_media_id,
 is_published, sort_order, created_at, updated_at,
 instructor_id, price_kobo, currency, is_published_to_academy,
 category, language, total_enrolled, subscription_eligible
FROM academy_tracks
WHERE is_published = true
ORDER BY sort_order ASC, created_at DESC;

-- name: GetAcademyTrackBySlug :one
SELECT id, title, slug, description, difficulty, cover_media_id,
 is_published, sort_order, created_at, updated_at,
 instructor_id, price_kobo, currency, is_published_to_academy,
 category, language, total_enrolled, subscription_eligible
FROM academy_tracks
WHERE slug = @slug;

-- name: GetAcademyTrackByID :one
SELECT id, title, slug, description, difficulty, cover_media_id,
 is_published, sort_order, created_at, updated_at,
 instructor_id, price_kobo, currency, is_published_to_academy,
 category, language, total_enrolled, subscription_eligible
FROM academy_tracks
WHERE id = @id;

-- name: GetAcademyTrackDetailBySlug :one
-- Track detail joined to media (thumbnail_url from cover_media_id) and
-- academy_instructor_profiles + users + user_profiles (instructor name/bio/username/
-- avatar) so GET /academy/tracks/:slug can populate the hero and instructor card
-- (ADR-060) without extra round-trips. Mirrors the university catalog join
-- pattern (university.sql: m.storage_path AS thumbnail_url). All joins are
-- LEFT so platform/staff-authored courses (no instructor) still resolve.
SELECT lt.id, lt.title, lt.slug, lt.description, lt.difficulty,
 lt.cover_media_id, lt.is_published, lt.sort_order, lt.created_at, lt.updated_at,
 lt.instructor_id, lt.price_kobo, lt.currency, lt.is_published_to_academy,
 lt.category, lt.language, lt.total_enrolled, lt.subscription_eligible,
 m.storage_bucket AS thumbnail_storage_bucket, m.storage_path AS thumbnail_storage_path,
 ip.id AS academy_instructor_profile_id, ip.display_name AS instructor_name,
 ip.bio AS instructor_bio,
 u.username AS instructor_username,
 up.avatar_url AS instructor_avatar_url
FROM academy_tracks lt
LEFT JOIN media m ON m.id = lt.cover_media_id
LEFT JOIN academy_instructor_profiles ip ON ip.id = lt.instructor_id
LEFT JOIN users u ON u.id = ip.user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE lt.slug = @slug;

-- name: CreateAcademyTrack :one
INSERT INTO academy_tracks (title, slug, description, difficulty, cover_media_id, is_published, sort_order, category, subscription_eligible) VALUES (@title, @slug, sqlc.narg(description), @difficulty, sqlc.narg(cover_media_id), @is_published, @sort_order, sqlc.narg(category), @subscription_eligible)

RETURNING id, title, slug, description, difficulty, cover_media_id, is_published, sort_order, created_at, updated_at,
 instructor_id, price_kobo, currency, is_published_to_academy, category, language, total_enrolled, subscription_eligible;

-- name: UpdateAcademyTrack :one
UPDATE academy_tracks
SET title = COALESCE(sqlc.narg(title), title),
 description = COALESCE(sqlc.narg(description), description),
 difficulty = COALESCE(sqlc.narg(difficulty), difficulty),
 cover_media_id = COALESCE(sqlc.narg(cover_media_id), cover_media_id),
 is_published = COALESCE(sqlc.narg(is_published), is_published),
 sort_order = COALESCE(sqlc.narg(sort_order), sort_order),
 category = COALESCE(sqlc.narg(category), category),
 subscription_eligible = COALESCE(sqlc.narg(subscription_eligible), subscription_eligible),
 updated_at = now()
WHERE id = @id
RETURNING id, title, slug, description, difficulty, cover_media_id, is_published, sort_order, created_at, updated_at,
 instructor_id, price_kobo, currency, is_published_to_academy, category, language, total_enrolled, subscription_eligible;

-- name: DeleteAcademyTrack :exec
DELETE FROM academy_tracks WHERE id = @id;

-- name: ListAllAcademyTracks :many
SELECT id, title, slug, description, difficulty, cover_media_id,
 is_published, sort_order, created_at, updated_at,
 instructor_id, price_kobo, currency, is_published_to_academy,
 category, language, total_enrolled, subscription_eligible
FROM academy_tracks

ORDER BY sort_order ASC, created_at DESC;

-- =====================
-- Module Operations
-- =====================

-- name: ListAcademyModulesByTrack :many
SELECT id, track_id, title, slug, description, sort_order, created_at, updated_at
FROM academy_modules
WHERE track_id = @track_id
ORDER BY sort_order ASC, created_at DESC;

-- name: GetAcademyModuleBySlug :one
SELECT id, track_id, title, slug, description, sort_order, created_at, updated_at
FROM academy_modules
WHERE track_id = @track_id AND slug = @slug;

-- name: GetAcademyModuleByID :one
SELECT id, track_id, title, slug, description, sort_order, created_at, updated_at
FROM academy_modules
WHERE id = @id;

-- name: CreateAcademyModule :one
INSERT INTO academy_modules (track_id, title, slug, description, sort_order) VALUES (@track_id, @title, @slug, sqlc.narg(description), @sort_order)

RETURNING id, track_id, title, slug, description, sort_order, created_at, updated_at;

-- name: UpdateAcademyModule :one
UPDATE academy_modules
SET title = COALESCE(sqlc.narg(title), title),
 description = COALESCE(sqlc.narg(description), description),
 sort_order = COALESCE(sqlc.narg(sort_order), sort_order),
 updated_at = now()
WHERE id = @id
RETURNING id, track_id, title, slug, description, sort_order, created_at, updated_at;

-- name: DeleteAcademyModule :exec
DELETE FROM academy_modules WHERE id = @id;

-- =====================
-- Lesson Operations
-- =====================

-- name: ListAcademyLessonsByModule :many
SELECT id, module_id, title, slug, lesson_type, content, media_id,
 duration_minutes, sort_order, is_published, is_preview, created_at, updated_at
FROM academy_lessons
WHERE module_id = @module_id
ORDER BY sort_order ASC, created_at DESC;

-- name: GetAcademyLessonBySlug :one
SELECT id, module_id, title, slug, lesson_type, content, media_id,
 duration_minutes, sort_order, is_published, is_preview, created_at, updated_at
FROM academy_lessons
WHERE module_id = @module_id AND slug = @slug;

-- name: GetAcademyLessonByID :one
SELECT id, module_id, title, slug, lesson_type, content, media_id,
 duration_minutes, sort_order, is_published, is_preview, created_at, updated_at
FROM academy_lessons
WHERE id = @id;

-- name: GetAcademyLessonDetailBySlug :one
-- Fetches a lesson joined to its module and track for the lesson player
-- (ADR-060). Returns module_title, track_id, track_slug, track_title so the
-- frontend can build breadcrumbs, the curriculum rail, and prev/next nav.
SELECT ll.id, ll.module_id, ll.title, ll.slug, ll.lesson_type,
 ll.content, ll.media_id, ll.duration_minutes, ll.sort_order,
 ll.is_published, ll.is_preview, ll.created_at, ll.updated_at,
 lm.title AS module_title, lm.slug AS module_slug,
 lt.id AS track_id, lt.slug AS track_slug, lt.title AS track_title,
 vm.storage_bucket AS video_storage_bucket, vm.storage_path AS video_storage_path,
 vm.duration_secs AS video_duration_secs
FROM academy_lessons ll
JOIN academy_modules lm ON lm.id = ll.module_id
JOIN academy_tracks lt ON lt.id = lm.track_id
LEFT JOIN media vm ON vm.id = ll.media_id
WHERE ll.module_id = @module_id AND ll.slug = @slug;

-- name: GetLessonCompletionStatus :one
-- Checks whether a user has completed a specific lesson. Used by GetLessonDetail
-- to populate is_completed for authenticated users so the lesson player can
-- show the "Completed" state on SSR render without waiting for CSR.
SELECT (lp.status = 'completed') AS is_completed, lp.position_secs
FROM academy_lesson_progress lp
WHERE lp.user_id = @user_id AND lp.lesson_id = @lesson_id;

-- name: ListTrackLessonsForNav :many
-- Lists all published lessons in a track (across modules) ordered by module
-- sort_order then lesson sort_order, so the handler can compute prev/next
-- lesson navigation for the lesson player (ADR-060).
SELECT ll.id, ll.slug, ll.title, ll.lesson_type, ll.sort_order,
 lm.slug AS module_slug, lm.sort_order AS module_sort_order
FROM academy_lessons ll
JOIN academy_modules lm ON lm.id = ll.module_id
WHERE lm.track_id = @track_id AND ll.is_published = true
ORDER BY lm.sort_order ASC, ll.sort_order ASC, ll.created_at DESC;

-- name: CreateAcademyLesson :one
INSERT INTO academy_lessons (module_id, title, slug, lesson_type, content, media_id, duration_minutes, sort_order, is_published, is_preview) VALUES (@module_id, @title, @slug, @lesson_type, sqlc.narg(content), sqlc.narg(media_id), sqlc.narg(duration_minutes), @sort_order, @is_published, @is_preview)

RETURNING id, module_id, title, slug, lesson_type, content, media_id, duration_minutes, sort_order, is_published, is_preview, created_at, updated_at;

-- name: UpdateAcademyLesson :one
UPDATE academy_lessons
SET title = COALESCE(sqlc.narg(title), title),
 lesson_type = COALESCE(sqlc.narg(lesson_type), lesson_type),
 content = COALESCE(sqlc.narg(content), content),
 media_id = COALESCE(sqlc.narg(media_id), media_id),
 duration_minutes = COALESCE(sqlc.narg(duration_minutes), duration_minutes),
 sort_order = COALESCE(sqlc.narg(sort_order), sort_order),
 is_published = COALESCE(sqlc.narg(is_published), is_published),
 is_preview = COALESCE(sqlc.narg(is_preview), is_preview),
 updated_at = now()
WHERE id = @id
RETURNING id, module_id, title, slug, lesson_type, content, media_id, duration_minutes, sort_order, is_published, is_preview, created_at, updated_at;

-- name: DeleteAcademyLesson :exec
DELETE FROM academy_lessons WHERE id = @id;

-- =====================
-- Progress Operations
-- =====================

-- name: GetLessonProgress :one
SELECT id, user_id, lesson_id, status, position_secs, completed_at, created_at
FROM academy_lesson_progress
WHERE user_id = @user_id AND lesson_id = @lesson_id;

-- name: UpsertLessonProgress :one
INSERT INTO academy_lesson_progress (user_id, lesson_id, status, position_secs, completed_at) VALUES (@user_id, @lesson_id, @status, sqlc.narg(position_secs), sqlc.narg(completed_at))

ON CONFLICT (user_id, lesson_id) DO UPDATE
SET status = EXCLUDED.status,
 position_secs = COALESCE(EXCLUDED.position_secs, academy_lesson_progress.position_secs),
 completed_at = COALESCE(EXCLUDED.completed_at, academy_lesson_progress.completed_at)
RETURNING id, user_id, lesson_id, status, position_secs, completed_at, created_at;

-- name: GetUserTrackProgress :one
SELECT
 COUNT(*) FILTER (WHERE lp.status = 'completed'),
 COUNT(*)
FROM academy_lessons ll
JOIN academy_modules lm ON lm.id = ll.module_id
LEFT JOIN academy_lesson_progress lp ON lp.lesson_id = ll.id AND lp.user_id = @user_id
WHERE lm.track_id = @track_id AND ll.is_published = true;

-- name: ListCompletedLessonIDsByUserAndTrack :many
-- Returns the IDs of all lessons the user has completed (status = 'completed')
-- in a track. Used by GET /academy/progress/{trackId} to populate the
-- completed_lesson_ids array so the curriculum rail can restore completion
-- state on page refresh without checking each lesson individually.
SELECT ll.id
FROM academy_lessons ll
JOIN academy_modules lm ON lm.id = ll.module_id
JOIN academy_lesson_progress lp ON lp.lesson_id = ll.id AND lp.user_id = @user_id
WHERE lm.track_id = @track_id AND ll.is_published = true AND lp.status = 'completed';

-- name: GetNextIncompleteLessonByTrack :one
-- Finds the first published lesson in a track that the user has NOT completed
-- (status != 'completed' or no progress row). Used to populate the
-- next_lesson field on enriched academy_enrollments for the Continue Learning card.
SELECT ll.id, ll.slug, ll.title, lm.slug AS module_slug
FROM academy_lessons ll
JOIN academy_modules lm ON lm.id = ll.module_id
LEFT JOIN academy_lesson_progress lp ON lp.lesson_id = ll.id AND lp.user_id = @user_id
WHERE lm.track_id = @track_id AND ll.is_published = true
 AND (lp.status IS NULL OR lp.status != 'completed')
ORDER BY lm.sort_order ASC, ll.sort_order ASC
LIMIT 1;

-- =====================
-- Certificate Operations
-- =====================

-- name: GetAcademyCertificate :one
SELECT id, user_id, track_id, certificate_number, media_id, issued_at
FROM academy_certificates
WHERE user_id = @user_id AND track_id = @track_id;

-- name: IssueAcademyCertificate :one
INSERT INTO academy_certificates (user_id, track_id, certificate_number) VALUES (@user_id, @track_id, @certificate_number)

ON CONFLICT DO NOTHING
RETURNING id, user_id, track_id, certificate_number, media_id, issued_at;

-- name: ListAcademyCertificatesByUser :many
SELECT lc.id, lc.user_id, lc.track_id, lc.certificate_number, lc.media_id, lc.issued_at,
 lt.title AS track_title, lt.slug AS track_slug,
 u.username AS recipient_username,
 COALESCE(up.display_name, u.username) AS recipient_name
FROM academy_certificates lc
LEFT JOIN academy_tracks lt ON lt.id = lc.track_id
LEFT JOIN users u ON u.id = lc.user_id
LEFT JOIN user_profiles up ON up.user_id = lc.user_id
WHERE lc.user_id = @user_id
ORDER BY lc.issued_at DESC;

-- name: GetAcademyCertificateByID :one
SELECT lc.id, lc.user_id, lc.track_id, lc.certificate_number, lc.media_id, lc.issued_at,
 lt.title AS track_title, lt.slug AS track_slug,
 u.username AS recipient_username,
 COALESCE(up.display_name, u.username) AS recipient_name
FROM academy_certificates lc
LEFT JOIN academy_tracks lt ON lt.id = lc.track_id
LEFT JOIN users u ON u.id = lc.user_id
LEFT JOIN user_profiles up ON up.user_id = lc.user_id
WHERE lc.id = @id;

-- =====================
-- Enrollment Operations
-- =====================

-- name: CreateEnrollment :one
INSERT INTO academy_enrollments (user_id, track_id, access_type) VALUES (@user_id, @track_id, @access_type)

ON CONFLICT (user_id, track_id) DO NOTHING
RETURNING id, user_id, track_id, access_type, progress_pct, enrolled_at, completed_at;

-- name: ListEnrollmentsByUserID :many
SELECT id, user_id, track_id, access_type, progress_pct, enrolled_at, completed_at
FROM academy_enrollments
WHERE user_id = @user_id
ORDER BY enrolled_at DESC;

-- name: ListEnrollmentsByUserIDWithDetails :many
-- Enriched enrollment list for GET /me/enrollments — joins academy_tracks
-- for slug/title/thumbnail/level and computes lessons_completed/lessons_total
-- + last_activity_at via a LATERAL subquery on academy_lesson_progress. Also fetches
-- the next incomplete lesson via a second LATERAL join so the Continue
-- Learning card can render without an N+1 per enrollment. The frontend
-- Enrollment type (apps/academy/src/lib/types) expects all these fields.
SELECT e.id, e.user_id, e.track_id, e.access_type,
 CASE
 WHEN COALESCE(lp_stats.lessons_total, 0) > 0 THEN
 ROUND(lp_stats.lessons_completed * 100.0 / lp_stats.lessons_total)
 ELSE 0
 END::numeric AS progress_pct,
 e.enrolled_at,
 CASE
 WHEN COALESCE(lp_stats.lessons_total, 0) > 0 AND lp_stats.lessons_completed >= lp_stats.lessons_total
 THEN COALESCE(e.completed_at, lp_stats.last_activity_at)
 END::timestamptz AS completed_at,
 lt.slug AS track_slug, lt.title AS track_title, lt.difficulty AS level,
 m.storage_bucket AS thumbnail_storage_bucket, m.storage_path AS thumbnail_storage_path,
 COALESCE(lp_stats.lessons_completed, 0) AS lessons_completed,
 COALESCE(lp_stats.lessons_total, 0) AS lessons_total,
 lp_stats.last_activity_at,
 COALESCE(nl.next_lesson_slug, '') AS next_lesson_slug,
 COALESCE(nl.next_lesson_title, '') AS next_lesson_title,
 COALESCE(nl.next_lesson_module_slug, '') AS next_lesson_module_slug
FROM academy_enrollments e
JOIN academy_tracks lt ON lt.id = e.track_id
LEFT JOIN media m ON m.id = lt.cover_media_id
LEFT JOIN LATERAL (
 SELECT
 COUNT(*) FILTER (WHERE lp.status = 'completed') AS lessons_completed,
 COUNT(*) AS lessons_total,
 MAX(lp.completed_at) AS last_activity_at
 FROM academy_lessons ll
 JOIN academy_modules lm ON lm.id = ll.module_id
 LEFT JOIN academy_lesson_progress lp ON lp.lesson_id = ll.id AND lp.user_id = e.user_id
 WHERE lm.track_id = e.track_id AND ll.is_published = true
) lp_stats ON true
LEFT JOIN LATERAL (
 SELECT ll.slug AS next_lesson_slug, ll.title AS next_lesson_title, lm.slug AS next_lesson_module_slug
 FROM academy_lessons ll
 JOIN academy_modules lm ON lm.id = ll.module_id
 LEFT JOIN academy_lesson_progress lp ON lp.lesson_id = ll.id AND lp.user_id = e.user_id
 WHERE lm.track_id = e.track_id AND ll.is_published = true
 AND (lp.status IS NULL OR lp.status != 'completed')
 ORDER BY lm.sort_order ASC, ll.sort_order ASC
 LIMIT 1
) nl ON true
WHERE e.user_id = @user_id
ORDER BY e.enrolled_at DESC;

-- name: GetEnrollmentByUserAndTrack :one
SELECT id, user_id, track_id, access_type, progress_pct, enrolled_at, completed_at
FROM academy_enrollments
WHERE user_id = @user_id AND track_id = @track_id;

-- name: UpdateEnrollmentProgress :exec
UPDATE academy_enrollments
SET progress_pct = @progress_pct,
 completed_at = CASE WHEN @progress_pct::numeric >= 100 THEN now() ELSE completed_at END
WHERE user_id = @user_id AND track_id = @track_id;

-- name: ListEnrollmentsByTrackID :many
SELECT e.id, e.user_id, e.track_id, e.access_type, e.progress_pct, e.enrolled_at, e.completed_at,
 u.username,
 COALESCE(up.display_name, '') AS display_name
FROM academy_enrollments e
JOIN users u ON u.id = e.user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE e.track_id = @track_id
ORDER BY e.enrolled_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================
-- Quiz Operations
-- =====================

-- name: GetQuizQuestionsByLesson :many
SELECT lq.id, lq.lesson_id, lq.question, lq.options, lq.answer, lq.explanation,
 lq.position, lq.created_at, ll.title AS lesson_title
FROM academy_quizzes lq
JOIN academy_lessons ll ON ll.id = lq.lesson_id
WHERE lq.lesson_id = @lesson_id
ORDER BY lq.position ASC;

-- =====================
-- Admin List Operations
-- =====================

-- name: ListAllAcademyModules :many
SELECT id, track_id, title, slug, description, sort_order, created_at, updated_at
FROM academy_modules

ORDER BY sort_order ASC, created_at DESC;

-- name: ListAllAcademyLessons :many
SELECT ll.id, ll.module_id, lm.title AS module_title, ll.title, ll.slug,
 ll.lesson_type, ll.content, ll.media_id, ll.duration_minutes, ll.sort_order,
 ll.is_published, ll.created_at, ll.updated_at
FROM academy_lessons ll
LEFT JOIN academy_modules lm ON lm.id = ll.module_id

ORDER BY ll.sort_order ASC, ll.created_at DESC;
