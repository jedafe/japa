-- name: GetDocPageBySlug :one
SELECT p.id, p.title, p.slug, p.audience, p.content, p.ai_generated, p.review_status,
 p.last_reviewed_by, p.last_reviewed_at, p.parent_id, p.position, p.category, p.subcategory,
 p.helpful_yes, p.helpful_no, p.feature_image_id,
 m.storage_bucket AS feature_image_bucket, m.storage_path AS feature_image_path,
 p.created_at, p.updated_at
FROM doc_pages p
LEFT JOIN media m ON m.id = p.feature_image_id
WHERE p.audience = @audience AND p.slug = @slug AND p.review_status = 'published';

-- name: GetDocPageBySlugAnyStatus :one
-- Editor draft preview (§44 amendment 2): like GetDocPageBySlug but status-
-- agnostic. Callers MUST gate on docs.manage — never expose to public readers.
SELECT p.id, p.title, p.slug, p.audience, p.content, p.ai_generated, p.review_status,
 p.last_reviewed_by, p.last_reviewed_at, p.parent_id, p.position, p.category, p.subcategory,
 p.helpful_yes, p.helpful_no, p.feature_image_id,
 m.storage_bucket AS feature_image_bucket, m.storage_path AS feature_image_path,
 p.created_at, p.updated_at
FROM doc_pages p
LEFT JOIN media m ON m.id = p.feature_image_id
WHERE p.audience = @audience AND p.slug = @slug;

-- name: ListDocPagesByAudience :many
-- content is selected so the service can derive a plain-text excerpt for
-- summary consumers (changelog, portal listings) — it is never returned raw.
SELECT p.id, p.title, p.slug, p.audience, p.review_status, p.parent_id, p.position, p.category, p.subcategory,
 p.feature_image_id,
 m.storage_bucket AS feature_image_bucket, m.storage_path AS feature_image_path,
 p.created_at, p.content
FROM doc_pages p
LEFT JOIN media m ON m.id = p.feature_image_id
WHERE p.audience = @audience AND p.review_status = 'published'
ORDER BY p.position ASC, p.title ASC;

-- name: ListDocPagesTree :many
SELECT id, title, slug, audience, parent_id, position, review_status, category, subcategory
FROM doc_pages
WHERE audience = @audience AND review_status = 'published'
ORDER BY position ASC;

-- name: ListAllDocPagesByAudience :many
SELECT p.id, p.title, p.slug, p.audience, p.ai_generated, p.review_status, p.last_reviewed_by,
 p.last_reviewed_at, p.parent_id, p.position, p.category, p.subcategory,
 p.feature_image_id,
 m.storage_bucket AS feature_image_bucket, m.storage_path AS feature_image_path,
 p.created_at, p.updated_at
FROM doc_pages p
LEFT JOIN media m ON m.id = p.feature_image_id
WHERE p.audience = @audience
ORDER BY p.position ASC, p.title ASC
LIMIT NULLIF(@result_limit::int, 0) OFFSET COALESCE(@result_offset::int, 0);

-- name: ListAllDocPages :many
SELECT p.id, p.title, p.slug, p.audience, p.ai_generated, p.review_status, p.last_reviewed_by,
 p.last_reviewed_at, p.parent_id, p.position, p.category, p.subcategory,
 p.feature_image_id,
 m.storage_bucket AS feature_image_bucket, m.storage_path AS feature_image_path,
 p.created_at, p.updated_at
FROM doc_pages p
LEFT JOIN media m ON m.id = p.feature_image_id
ORDER BY p.audience ASC, p.position ASC, p.title ASC
LIMIT NULLIF(@result_limit::int, 0) OFFSET COALESCE(@result_offset::int, 0);

-- name: GetDocPageByID :one
SELECT p.id, p.title, p.slug, p.audience, p.content, p.ai_generated, p.review_status,
 p.last_reviewed_by, p.last_reviewed_at, p.parent_id, p.position, p.category, p.subcategory,
 p.helpful_yes, p.helpful_no, p.feature_image_id,
 m.storage_bucket AS feature_image_bucket, m.storage_path AS feature_image_path,
 p.created_at, p.updated_at
FROM doc_pages p
LEFT JOIN media m ON m.id = p.feature_image_id
WHERE p.id = @id;

-- name: CreateDocPage :one
INSERT INTO doc_pages (title, slug, audience, content, ai_generated, review_status, parent_id, position, category, subcategory, created_by, feature_image_id) VALUES (@title, @slug, @audience, @content, @ai_generated, @review_status, @parent_id, @position, @category, @subcategory, sqlc.narg(created_by)::uuid, sqlc.narg(feature_image_id)::uuid)

RETURNING id;

-- name: UpdateDocPage :execrows
-- §44 amendment 2: full update contract — slug, parent_id, position, audience
-- are settable; last_reviewed_* are NOT touched (approve/publish endpoints own
-- them). expected_updated_at is an optimistic-lock precondition: a stale
-- caller writes 0 rows → mapped to 409 stale_write upstream.
UPDATE doc_pages
SET title = @title, content = @content, review_status = @review_status,
 slug = @slug, parent_id = @parent_id, position = @position, audience = @audience,
 category = @category, subcategory = @subcategory,
 feature_image_id = @feature_image_id,
 -- clear_review: content edits to an approved page demote it back to
 -- in_review and void the prior review stamp (§44 amendment 3).
 last_reviewed_by = CASE WHEN sqlc.narg(clear_review)::boolean THEN NULL ELSE last_reviewed_by END,
 last_reviewed_at = CASE WHEN sqlc.narg(clear_review)::boolean THEN NULL ELSE last_reviewed_at END,
 updated_at = now()
WHERE id = @id
  AND (sqlc.narg(expected_updated_at)::timestamptz IS NULL
       OR updated_at = sqlc.narg(expected_updated_at)::timestamptz);

-- name: UpdateDocPageStatus :exec
-- Approve/publish transitions only — records the reviewer (§44).
UPDATE doc_pages
SET review_status = @review_status, last_reviewed_by = @last_reviewed_by,
 last_reviewed_at = now(), updated_at = now()
WHERE id = @id;

-- name: GetDocPageStatus :one
SELECT review_status, created_by, ai_generated FROM doc_pages WHERE id = @id;

-- name: DeleteDocPage :exec
DELETE FROM doc_pages WHERE id = @id;

-- name: ClearDocPageParent :exec
-- Orphan children before their parent is deleted — parent_id is not an FK.
UPDATE doc_pages SET parent_id = NULL, updated_at = now() WHERE parent_id = @id;

-- name: IncrementDocPageHelpful :exec
UPDATE doc_pages
SET helpful_yes = helpful_yes + CASE WHEN @vote = 'up' THEN 1 ELSE 0 END,
 helpful_no = helpful_no + CASE WHEN @vote = 'down' THEN 1 ELSE 0 END
WHERE audience = @audience AND slug = @slug;

-- name: GetDocPageFeedback :one
-- Aggregate counts + the requesting user's recorded vote (§44 amendment 2).
SELECT p.id AS page_id, p.helpful_yes, p.helpful_no, v.vote AS my_vote
FROM doc_pages p
LEFT JOIN doc_page_votes v ON v.page_id = p.id AND v.user_id = @user_id::uuid
WHERE p.audience = @audience AND p.slug = @slug AND p.review_status = 'published';

-- name: SetDocPageVote :execrows
-- Atomic vote upsert + counter adjustment (§44 amendment 2). The CTEs see the
-- pre-statement snapshot: prev captures any existing vote, ups applies the
-- change only when the vote is new or different, and the UPDATE moves the
-- aggregate counters to match. Repeat votes and missing pages are no-ops.
WITH prev AS (
    SELECT vote FROM doc_page_votes
    WHERE doc_page_votes.page_id = @page_id AND doc_page_votes.user_id = @user_id
), ups AS (
    INSERT INTO doc_page_votes (page_id, user_id, vote)
    VALUES (@page_id, @user_id, @vote)
    ON CONFLICT (page_id, user_id) DO UPDATE
        SET vote = EXCLUDED.vote, updated_at = now()
        WHERE doc_page_votes.vote <> @vote
)
UPDATE doc_pages SET
    helpful_yes = helpful_yes + CASE
        WHEN @vote = 'up' AND NOT EXISTS (SELECT 1 FROM prev WHERE prev.vote = 'up') THEN 1
        WHEN @vote = 'down' AND EXISTS (SELECT 1 FROM prev WHERE prev.vote = 'up') THEN -1
        ELSE 0 END,
    helpful_no = helpful_no + CASE
        WHEN @vote = 'down' AND NOT EXISTS (SELECT 1 FROM prev WHERE prev.vote = 'down') THEN 1
        WHEN @vote = 'up' AND EXISTS (SELECT 1 FROM prev WHERE prev.vote = 'down') THEN -1
        ELSE 0 END
WHERE doc_pages.id = @page_id;

-- name: DeleteDocPageVote :exec
-- Atomic vote retraction + counter decrement — deleting the vote row and
-- moving the aggregate counters in one statement (§44 amendment 2: the
-- widget retracts on re-click, matching wiki page feedback).
WITH del AS (
    DELETE FROM doc_page_votes
    WHERE doc_page_votes.page_id = @page_id AND doc_page_votes.user_id = @user_id
    RETURNING vote
)
UPDATE doc_pages SET
    helpful_yes = helpful_yes - (SELECT COUNT(*)::int FROM del WHERE del.vote = 'up'),
    helpful_no  = helpful_no  - (SELECT COUNT(*)::int FROM del WHERE del.vote = 'down')
WHERE doc_pages.id = @page_id;

-- name: ListPublishedDocPages :many
SELECT p.id, p.title, p.slug, p.audience, p.parent_id, p.position, p.category, p.subcategory,
 p.feature_image_id,
 m.storage_bucket AS feature_image_bucket, m.storage_path AS feature_image_path,
 p.created_at
FROM doc_pages p
LEFT JOIN media m ON m.id = p.feature_image_id
WHERE p.review_status = 'published'
ORDER BY p.audience ASC, p.position ASC, p.title ASC
LIMIT NULLIF(@result_limit::int, 0) OFFSET COALESCE(@result_offset::int, 0);

-- name: SearchDocPages :many
SELECT id, title, slug, audience, content, parent_id, position, category, subcategory, created_at
FROM doc_pages
WHERE review_status = 'published'
 AND (sqlc.narg(audience)::text IS NULL OR audience = sqlc.narg(audience))
 AND (title ILIKE '%' || @query::text || '%'
   OR regexp_replace(content::text, '"[^"]*"[[:space:]]*:', ' ', 'g') ILIKE '%' || @query::text || '%')
ORDER BY position ASC, title ASC
LIMIT @result_limit;

-- name: SearchDocPagesInAudiences :many
-- §44 amendment 2: single query over the caller's accessible audiences.
-- The content match strips JSON keys first ("key": → space) so structural
-- keys like "doc"/"paragraph"/"text" cannot match every page.
SELECT id, title, slug, audience, content, parent_id, position, category, subcategory, created_at
FROM doc_pages
WHERE review_status = 'published'
 AND audience = ANY(@audiences::text[])
 AND (title ILIKE '%' || @query::text || '%'
   OR regexp_replace(content::text, '"[^"]*"[[:space:]]*:', ' ', 'g') ILIKE '%' || @query::text || '%')
ORDER BY array_position(@audiences::text[], audience), position ASC, title ASC
LIMIT @result_limit;

-- name: InsertDocPageRevision :one
-- §44 amendment 5: snapshot of title/content/feature_image after a
-- content-changing write; version is the per-page sequence (MAX+1) computed
-- in the same statement so concurrent writers can't pick the same number.
INSERT INTO doc_page_revisions (doc_page_id, version, title, content, feature_image_id, summary, created_by)
SELECT @doc_page_id,
       COALESCE(MAX(version), 0) + 1,
       @title, @content, sqlc.narg(feature_image_id)::uuid, sqlc.narg(summary)::text, sqlc.narg(created_by)::uuid
FROM doc_page_revisions
WHERE doc_page_id = @doc_page_id
RETURNING id, version;

-- name: ListDocPageRevisions :many
-- content is selected so the service derives the plain-text excerpt — same
-- convention as ListDocPagesByAudience.
SELECT id, doc_page_id, version, title, content, feature_image_id, summary, created_by, created_at
FROM doc_page_revisions
WHERE doc_page_id = @doc_page_id
ORDER BY version DESC
LIMIT NULLIF(@result_limit::int, 0) OFFSET COALESCE(@result_offset::int, 0);

-- name: CountDocPageRevisions :one
SELECT COUNT(*) FROM doc_page_revisions WHERE doc_page_id = @doc_page_id;

-- name: GetDocPageRevision :one
SELECT id, doc_page_id, version, title, content, feature_image_id, summary, created_by, created_at
FROM doc_page_revisions
WHERE id = @id;

-- name: GetDocPageMaxRevisionVersion :one
SELECT COALESCE(MAX(version), 0)::int FROM doc_page_revisions WHERE doc_page_id = @doc_page_id;

-- name: DeleteDocPageRevision :execrows
DELETE FROM doc_page_revisions WHERE id = @id;

-- name: DeleteDocPageRevisions :exec
DELETE FROM doc_page_revisions WHERE doc_page_id = @doc_page_id;

-- name: PruneDocPageRevisions :exec
-- §44 amendment 5 retention: keep the 5 newest snapshots per page — the
-- current edition plus the last 4 in history. Runs in the same tx as each
-- revision insert so the window never grows.
DELETE FROM doc_page_revisions dpr
WHERE dpr.doc_page_id = @doc_page_id
  AND dpr.version <= (
    SELECT COALESCE(MAX(r.version), 0) - 5
    FROM doc_page_revisions r
    WHERE r.doc_page_id = @doc_page_id
  );
