-- University course catalog and offerings queries.
-- Ref: japaconnect-engineering/sections/39-space-types-university.md

-- name: ListCourseOfferingsByUniversity :many
SELECT o.id, o.course_id, o.university_id, o.slug,
 o.tuition, o.currency, o.duration_years, o.study_mode, o.intake_months,
 o.entry_requirements, o.accreditation, o.is_published, o.created_at, o.updated_at,
 c.title AS course_title, c.slug AS course_slug, c.description AS course_description,
 c.degree_type, c.department, c.field_of_study, c.language, c.default_duration_years,
 s.name AS university_name, s.slug AS university_slug
FROM university_course_offerings o
JOIN university_course_catalog c ON c.id = o.course_id AND c.deleted_at IS NULL
JOIN spaces s ON s.id = o.university_id AND s.deleted_at IS NULL
WHERE o.deleted_at IS NULL AND o.is_published = true
 AND c.is_published = true
 AND (@university_id::uuid IS NULL OR o.university_id = @university_id)
 AND (@search::text = '' OR c.title ILIKE '%' || @search || '%')
ORDER BY c.title ASC
LIMIT @limit_val OFFSET @offset_val;

-- name: GetCourseOfferingByID :one
SELECT o.id, o.course_id, o.university_id, o.slug,
 o.tuition, o.currency, o.duration_years, o.study_mode, o.intake_months,
 o.entry_requirements, o.accreditation, o.is_published, o.created_at, o.updated_at,
 c.title AS course_title, c.slug AS course_slug, c.description AS course_description,
 c.degree_type, c.department, c.field_of_study, c.language, c.default_duration_years,
 s.name AS university_name, s.slug AS university_slug
FROM university_course_offerings o
JOIN university_course_catalog c ON c.id = o.course_id AND c.deleted_at IS NULL
JOIN spaces s ON s.id = o.university_id AND s.deleted_at IS NULL
WHERE o.id = @id AND o.deleted_at IS NULL;

-- name: GetCourseOfferingBySlug :one
SELECT o.id, o.course_id, o.university_id, o.slug,
 o.tuition, o.currency, o.duration_years, o.study_mode, o.intake_months,
 o.entry_requirements, o.accreditation, o.is_published, o.created_at, o.updated_at,
 c.title AS course_title, c.slug AS course_slug, c.description AS course_description,
 c.degree_type, c.department, c.field_of_study, c.language, c.default_duration_years,
 s.name AS university_name, s.slug AS university_slug
FROM university_course_offerings o
JOIN university_course_catalog c ON c.id = o.course_id AND c.deleted_at IS NULL
JOIN spaces s ON s.id = o.university_id AND s.deleted_at IS NULL
WHERE o.deleted_at IS NULL
 AND o.slug = @slug
LIMIT 1;
