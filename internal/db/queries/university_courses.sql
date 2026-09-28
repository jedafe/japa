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
 AND (@degree_type::text = '' OR c.degree_type = @degree_type)
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

-- name: CountCourseOfferingsByUniversity :one
SELECT count(*) AS total
FROM university_course_offerings o
JOIN university_course_catalog c ON c.id = o.course_id AND c.deleted_at IS NULL
JOIN spaces s ON s.id = o.university_id AND s.deleted_at IS NULL
WHERE o.deleted_at IS NULL AND o.is_published = true
 AND c.is_published = true
 AND (@university_id::uuid IS NULL OR o.university_id = @university_id)
 AND (@search::text = '' OR c.title ILIKE '%' || @search || '%')
 AND (@degree_type::text = '' OR c.degree_type = @degree_type);

-- name: GetPublishedCourseOfferingBySlugs :one
-- Public offering detail scoped to a university slug — the nested route is
-- canonical, so an offering requested under a different university 404s.
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
 AND o.is_published = true
 AND c.is_published = true
 AND o.slug = @offering_slug
 AND s.slug = @university_slug
LIMIT 1;

-- Admin: course catalog CRUD (universities.course_catalog.* RBAC nodes)

-- name: ListCourseCatalogAdmin :many
SELECT c.id, c.title, c.slug, c.description, c.degree_type, c.department,
 c.field_of_study, c.language, c.default_duration_years, c.is_published,
 c.created_at, c.updated_at, c.deleted_at,
 (SELECT count(*) FROM university_course_offerings o
  WHERE o.course_id = c.id AND o.deleted_at IS NULL) AS offering_count
FROM university_course_catalog c
WHERE (@search::text = '' OR c.title ILIKE '%' || @search || '%' OR c.slug ILIKE '%' || @search || '%')
 AND (@degree_type::text = '' OR c.degree_type = @degree_type)
 AND (sqlc.narg(is_published)::bool IS NULL OR c.is_published = sqlc.narg(is_published))
 -- ?deleted=only|all surfaces soft-deleted rows for admin force-delete.
 AND CASE @deleted::text
       WHEN 'only' THEN c.deleted_at IS NOT NULL
       WHEN 'all'  THEN true
       ELSE c.deleted_at IS NULL
     END
ORDER BY
 CASE WHEN @sort::text = 'title' THEN c.title END ASC NULLS LAST,
 CASE WHEN @sort::text = 'oldest' THEN c.created_at END ASC NULLS LAST,
 c.created_at DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: CountCourseCatalogAdmin :one
SELECT count(*) AS total
FROM university_course_catalog c
WHERE (@search::text = '' OR c.title ILIKE '%' || @search || '%' OR c.slug ILIKE '%' || @search || '%')
 AND (@degree_type::text = '' OR c.degree_type = @degree_type)
 AND (sqlc.narg(is_published)::bool IS NULL OR c.is_published = sqlc.narg(is_published))
 AND CASE @deleted::text
       WHEN 'only' THEN c.deleted_at IS NOT NULL
       WHEN 'all'  THEN true
       ELSE c.deleted_at IS NULL
     END;

-- name: GetCourseCatalogByID :one
SELECT c.id, c.title, c.slug, c.description, c.degree_type, c.department,
 c.field_of_study, c.language, c.default_duration_years, c.is_published,
 c.created_at, c.updated_at,
 (SELECT count(*) FROM university_course_offerings o
  WHERE o.course_id = c.id AND o.deleted_at IS NULL) AS offering_count
FROM university_course_catalog c
WHERE c.id = @id AND c.deleted_at IS NULL;

-- name: CreateCourseCatalog :one
INSERT INTO university_course_catalog (title, slug, description, degree_type,
 department, field_of_study, language, default_duration_years, is_published)
VALUES (@title, @slug, sqlc.narg(description), sqlc.narg(degree_type),
 sqlc.narg(department), sqlc.narg(field_of_study),
 COALESCE(sqlc.narg(language), 'English'), sqlc.narg(default_duration_years),
 COALESCE(sqlc.narg(is_published), true))
RETURNING id, title, slug, description, degree_type, department, field_of_study,
 language, default_duration_years, is_published, created_at, updated_at;

-- name: UpdateCourseCatalog :one
UPDATE university_course_catalog
SET title = @title, slug = @slug, description = sqlc.narg(description),
 degree_type = sqlc.narg(degree_type), department = sqlc.narg(department),
 field_of_study = sqlc.narg(field_of_study), language = @language,
 default_duration_years = sqlc.narg(default_duration_years),
 is_published = @is_published, updated_at = now()
WHERE id = @id AND deleted_at IS NULL
RETURNING id, title, slug, description, degree_type, department, field_of_study,
 language, default_duration_years, is_published, created_at, updated_at;

-- name: SoftDeleteCourseCatalog :exec
UPDATE university_course_catalog
SET deleted_at = now(), updated_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: HardDeleteCourseCatalog :execrows
-- Permanent delete — only for already-soft-deleted catalog rows (admin
-- ?force=true). Offerings FK-cascade, so the row must have no offerings at
-- all (live or deleted): force-delete the offerings first rather than
-- silently cascading live children.
DELETE FROM university_course_catalog c
WHERE c.id = @id AND c.deleted_at IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM university_course_offerings o WHERE o.course_id = c.id);

-- name: CourseCatalogExists :one
-- Existence probe for the hard-delete path: distinguishes 404 not_found
-- from 409 not_deleted/in_use when the guarded DELETE affects no row.
SELECT EXISTS(SELECT 1 FROM university_course_catalog WHERE id = @id) AS exists;

-- name: CourseCatalogHasOfferings :one
-- Dependency probe for the hard-delete path (offerings must go first).
SELECT EXISTS(SELECT 1 FROM university_course_offerings WHERE course_id = @id) AS exists;

-- Admin: course offerings CRUD (universities.course_offering.* RBAC nodes)

-- name: ListCourseOfferingsAdmin :many
-- ?deleted=only|all surfaces soft-deleted rows for admin force-delete.
-- The catalog/university JOINs drop their deleted_at filter so deleted
-- offerings still resolve their parent names in the admin list.
SELECT o.id, o.course_id, o.university_id, o.slug,
 o.tuition, o.currency, o.duration_years, o.study_mode, o.intake_months,
 o.entry_requirements, o.accreditation, o.is_published, o.created_at, o.updated_at,
 o.deleted_at,
 c.title AS course_title, c.slug AS course_slug, c.degree_type,
 s.name AS university_name, s.slug AS university_slug
FROM university_course_offerings o
JOIN university_course_catalog c ON c.id = o.course_id
JOIN spaces s ON s.id = o.university_id
WHERE (@search::text = '' OR c.title ILIKE '%' || @search || '%' OR o.slug ILIKE '%' || @search || '%' OR s.name ILIKE '%' || @search || '%')
 AND (sqlc.narg(university_id)::uuid IS NULL OR o.university_id = sqlc.narg(university_id))
 AND (sqlc.narg(course_id)::uuid IS NULL OR o.course_id = sqlc.narg(course_id))
 AND (sqlc.narg(is_published)::bool IS NULL OR o.is_published = sqlc.narg(is_published))
 AND CASE @deleted::text
       WHEN 'only' THEN o.deleted_at IS NOT NULL
       WHEN 'all'  THEN true
       ELSE o.deleted_at IS NULL
     END
ORDER BY
 CASE WHEN @sort::text = 'title' THEN c.title END ASC NULLS LAST,
 CASE WHEN @sort::text = 'university' THEN s.name END ASC NULLS LAST,
 CASE WHEN @sort::text = 'oldest' THEN o.created_at END ASC NULLS LAST,
 o.created_at DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: CountCourseOfferingsAdmin :one
SELECT count(*) AS total
FROM university_course_offerings o
JOIN university_course_catalog c ON c.id = o.course_id
JOIN spaces s ON s.id = o.university_id
WHERE (@search::text = '' OR c.title ILIKE '%' || @search || '%' OR o.slug ILIKE '%' || @search || '%' OR s.name ILIKE '%' || @search || '%')
 AND (sqlc.narg(university_id)::uuid IS NULL OR o.university_id = sqlc.narg(university_id))
 AND (sqlc.narg(course_id)::uuid IS NULL OR o.course_id = sqlc.narg(course_id))
 AND (sqlc.narg(is_published)::bool IS NULL OR o.is_published = sqlc.narg(is_published))
 AND CASE @deleted::text
       WHEN 'only' THEN o.deleted_at IS NOT NULL
       WHEN 'all'  THEN true
       ELSE o.deleted_at IS NULL
     END;

-- name: CreateCourseOffering :one
INSERT INTO university_course_offerings (course_id, university_id, slug,
 tuition, currency, duration_years, study_mode, intake_months,
 entry_requirements, accreditation, is_published)
VALUES (@course_id, @university_id, @slug, sqlc.narg(tuition),
 sqlc.narg(currency), sqlc.narg(duration_years), sqlc.narg(study_mode),
 sqlc.narg(intake_months), sqlc.narg(entry_requirements),
 sqlc.narg(accreditation), COALESCE(sqlc.narg(is_published), true))
RETURNING id, course_id, university_id, slug, tuition, currency, duration_years,
 study_mode, intake_months, entry_requirements, accreditation, is_published,
 created_at, updated_at;

-- name: UpdateCourseOffering :one
UPDATE university_course_offerings
SET course_id = @course_id, university_id = @university_id, slug = @slug,
 tuition = sqlc.narg(tuition), currency = sqlc.narg(currency),
 duration_years = sqlc.narg(duration_years), study_mode = sqlc.narg(study_mode),
 intake_months = sqlc.narg(intake_months),
 entry_requirements = sqlc.narg(entry_requirements),
 accreditation = sqlc.narg(accreditation), is_published = @is_published,
 updated_at = now()
WHERE id = @id AND deleted_at IS NULL
RETURNING id, course_id, university_id, slug, tuition, currency, duration_years,
 study_mode, intake_months, entry_requirements, accreditation, is_published,
 created_at, updated_at;

-- name: SoftDeleteCourseOffering :exec
UPDATE university_course_offerings
SET deleted_at = now(), updated_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: HardDeleteCourseOffering :execrows
-- Permanent delete — only for already-soft-deleted offerings (admin
-- ?force=true). Nothing references offerings as a parent FK.
DELETE FROM university_course_offerings WHERE id = @id AND deleted_at IS NOT NULL;

-- name: CourseOfferingExists :one
-- Existence probe for the hard-delete path: distinguishes 404 not_found
-- from 409 not_deleted when the DELETE affects no row.
SELECT EXISTS(SELECT 1 FROM university_course_offerings WHERE id = @id) AS exists;

-- name: BulkSetCourseOfferingsPublished :exec
UPDATE university_course_offerings
SET is_published = @is_published, updated_at = now()
WHERE id = ANY(@ids::uuid[]) AND deleted_at IS NULL;

-- name: BulkSoftDeleteCourseOfferings :exec
UPDATE university_course_offerings
SET deleted_at = now(), updated_at = now()
WHERE id = ANY(@ids::uuid[]) AND deleted_at IS NULL;
