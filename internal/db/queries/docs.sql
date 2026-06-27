-- name: GetDocPageBySlug :one
SELECT id, tenant_id, title, slug, audience, content, ai_generated, review_status,
       last_reviewed_by, last_reviewed_at, parent_id, position, created_at, updated_at
FROM doc_pages
WHERE tenant_id = @tenant_id AND audience = @audience AND slug = @slug AND review_status = 'published';

-- name: ListDocPagesByAudience :many
SELECT id, title, slug, audience, review_status, parent_id, position, created_at
FROM doc_pages
WHERE tenant_id = @tenant_id AND audience = @audience AND review_status = 'published'
ORDER BY position ASC, title ASC;

-- name: ListDocPagesTree :many
SELECT id, title, slug, audience, parent_id, position, review_status
FROM doc_pages
WHERE tenant_id = @tenant_id AND audience = @audience AND review_status = 'published'
ORDER BY position ASC;

-- name: ListAllDocPagesByAudience :many
SELECT id, title, slug, audience, ai_generated, review_status, last_reviewed_by,
       last_reviewed_at, parent_id, position, created_at, updated_at
FROM doc_pages
WHERE tenant_id = @tenant_id AND audience = @audience
ORDER BY position ASC, title ASC;

-- name: GetDocPageByID :one
SELECT id, tenant_id, title, slug, audience, content, ai_generated, review_status,
       last_reviewed_by, last_reviewed_at, parent_id, position, created_at, updated_at
FROM doc_pages
WHERE id = @id AND tenant_id = @tenant_id;

-- name: CreateDocPage :exec
INSERT INTO doc_pages (tenant_id, title, slug, audience, content, ai_generated, review_status, parent_id, position)
VALUES (@tenant_id, @title, @slug, @audience, @content, @ai_generated, @review_status, @parent_id, @position);

-- name: UpdateDocPage :exec
UPDATE doc_pages
SET title = @title, content = @content, review_status = @review_status,
    last_reviewed_by = @last_reviewed_by, last_reviewed_at = now(), updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id;

-- name: DeleteDocPage :exec
DELETE FROM doc_pages WHERE id = @id AND tenant_id = @tenant_id;
