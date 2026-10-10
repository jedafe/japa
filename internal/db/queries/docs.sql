-- name: GetDocPageBySlug :one
SELECT id, title, slug, audience, content, ai_generated, review_status,
 last_reviewed_by, last_reviewed_at, parent_id, position, category, subcategory,
 created_at, updated_at
FROM doc_pages
WHERE audience = @audience AND slug = @slug AND review_status = 'published';

-- name: ListDocPagesByAudience :many
SELECT id, title, slug, audience, review_status, parent_id, position, category, subcategory, created_at
FROM doc_pages
WHERE audience = @audience AND review_status = 'published'
ORDER BY position ASC, title ASC;

-- name: ListDocPagesTree :many
SELECT id, title, slug, audience, parent_id, position, review_status, category, subcategory
FROM doc_pages
WHERE audience = @audience AND review_status = 'published'
ORDER BY position ASC;

-- name: ListAllDocPagesByAudience :many
SELECT id, title, slug, audience, ai_generated, review_status, last_reviewed_by,
 last_reviewed_at, parent_id, position, category, subcategory, created_at, updated_at
FROM doc_pages
WHERE audience = @audience
ORDER BY position ASC, title ASC;

-- name: ListAllDocPages :many
SELECT id, title, slug, audience, ai_generated, review_status, last_reviewed_by,
 last_reviewed_at, parent_id, position, category, subcategory, created_at, updated_at
FROM doc_pages

ORDER BY audience ASC, position ASC, title ASC;

-- name: GetDocPageByID :one
SELECT id, title, slug, audience, content, ai_generated, review_status,
 last_reviewed_by, last_reviewed_at, parent_id, position, category, subcategory,
 created_at, updated_at
FROM doc_pages
WHERE id = @id;

-- name: CreateDocPage :one
INSERT INTO doc_pages (title, slug, audience, content, ai_generated, review_status, parent_id, position, category, subcategory) VALUES (@title, @slug, @audience, @content, @ai_generated, @review_status, @parent_id, @position, @category, @subcategory)

RETURNING id;

-- name: UpdateDocPage :exec
UPDATE doc_pages
SET title = @title, content = @content, review_status = @review_status,
 category = @category, subcategory = @subcategory,
 last_reviewed_by = @last_reviewed_by, last_reviewed_at = now(), updated_at = now()
WHERE id = @id;

-- name: UpdateDocPageStatus :exec
UPDATE doc_pages
SET review_status = @review_status, last_reviewed_by = @last_reviewed_by,
 last_reviewed_at = now(), updated_at = now()
WHERE id = @id;

-- name: GetDocPageStatus :one
SELECT review_status FROM doc_pages WHERE id = @id;

-- name: DeleteDocPage :exec
DELETE FROM doc_pages WHERE id = @id;

-- name: IncrementDocPageHelpful :exec
UPDATE doc_pages
SET helpful_yes = helpful_yes + CASE WHEN @vote = 'up' THEN 1 ELSE 0 END,
 helpful_no = helpful_no + CASE WHEN @vote = 'down' THEN 1 ELSE 0 END
WHERE audience = @audience AND slug = @slug;

-- name: ListPublishedDocPages :many
SELECT id, title, slug, audience, parent_id, position, category, subcategory, created_at
FROM doc_pages
WHERE review_status = 'published'
ORDER BY audience ASC, position ASC, title ASC;

-- name: SearchDocPages :many
SELECT id, title, slug, audience, content, parent_id, position, category, subcategory, created_at
FROM doc_pages
WHERE review_status = 'published'
 AND (sqlc.narg(audience)::text IS NULL OR audience = sqlc.narg(audience))
 AND (title ILIKE '%' || @query::text || '%' OR content::text ILIKE '%' || @query::text || '%')
ORDER BY position ASC, title ASC
LIMIT @result_limit;
