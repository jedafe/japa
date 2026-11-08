-- Wiki (§80) queries — consolidated from internal/wiki/store.go.
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md

-- =====================
-- Pages
-- =====================

-- name: ListWikiPageSiblings :many
-- Ordered sibling pages within a space+category — the Prev/Next set (§80
-- Hierarchical UX amendment). position ASC, then created_at for stability.
-- Soft-deleted pages are excluded like every other wiki_pages read.
SELECT wp.id, wp.slug, wp.title, wp.position, wp.created_at
FROM wiki_pages wp
WHERE wp.space_id = @space_id AND wp.category = @category
  AND wp.is_active = true AND wp.deleted_at IS NULL
ORDER BY wp.position ASC, wp.created_at ASC, wp.slug ASC;


-- name: GetWikiPageBySlug :one
SELECT wp.id, wp.space_id, wp.slug, wp.title, wp.category,
       wp.is_active, wp.is_featured, wp.is_locked, wp.created_by,
       wp.created_at, wp.updated_at, wp.is_japabot_indexed,
       wp.data_entity_type, wp.page_template_id, wp.field_group_id, wp.position,
       sp.slug AS space_slug, sp.name AS space_name
FROM wiki_pages wp
JOIN spaces sp ON sp.id = wp.space_id
WHERE sp.slug = @space_slug AND wp.slug = @page_slug
  AND wp.is_active = true AND wp.deleted_at IS NULL
LIMIT 1;

-- name: WikiPageRowExistsBySlug :one
-- Filter-free existence check (§80 lazy pages): distinguishes "no row —
-- eligible for lazy template render" from "row exists but is inactive or
-- soft-deleted — real 404, the lazy path must not resurrect it".
SELECT EXISTS (
    SELECT 1 FROM wiki_pages wp
    JOIN spaces sp ON sp.id = wp.space_id
    WHERE sp.slug = @space_slug AND wp.slug = @page_slug
) AS exists;

-- name: ListWikiPagesBySpaceSlug :many
SELECT wp.id, wp.space_id, wp.slug, wp.title, wp.category,
       wp.is_active, wp.is_featured, wp.is_locked, wp.created_by,
       wp.created_at, wp.updated_at, wp.is_japabot_indexed,
       wp.data_entity_type, wp.page_template_id, wp.field_group_id, wp.position,
       sp.slug AS space_slug, sp.name AS space_name
FROM wiki_pages wp
JOIN spaces sp ON sp.id = wp.space_id
WHERE sp.slug = @space_slug AND wp.is_active = true
ORDER BY wp.category, wp.title;

-- name: ListWikiPagesBySpaceID :many
SELECT wp.id, wp.space_id, wp.slug, wp.title, wp.category,
       wp.is_active, wp.is_featured, wp.is_locked, wp.created_by,
       wp.created_at, wp.updated_at, wp.is_japabot_indexed,
       wp.data_entity_type, wp.page_template_id, wp.field_group_id, wp.position,
       sp.slug AS space_slug, sp.name AS space_name
FROM wiki_pages wp
JOIN spaces sp ON sp.id = wp.space_id
WHERE wp.space_id = @space_id AND wp.is_active = true
ORDER BY wp.category, wp.title;

-- name: ListWikiDestinations :many
-- Wiki-type-driven destination list (§80 Wiki Types amendment): the base set
-- is spaces whose space_type has wiki_type_configs.enabled — the former
-- hardcoded (level IN geo+university) predicate is gone so new wiki types
-- appear automatically. ?level matches sp.level OR st.slug so legacy
-- level='university' spaces and future type slugs both resolve.
-- ?within= descendant set: walk DOWN from the target (≤3 hops over
-- space_parent_spaces ∪ parent_id) once — the old form ran a 3-level
-- correlated ancestor EXISTS per candidate row (~2s on a full scan).
WITH RECURSIVE subtree AS (
    -- Non-recursive: direct children of the target over both edge kinds.
    -- One unified edge list keeps a single self-reference term — Postgres
    -- rejects mixed multi-branch shapes (SQLSTATE 42P19).
    SELECT e.cid AS id, 1 AS depth
    FROM (
        SELECT sps.parent_space_id AS pid, sps.space_id AS cid FROM space_parent_spaces sps
        UNION ALL
        SELECT c.parent_id, c.id FROM spaces c WHERE c.parent_id IS NOT NULL
    ) e
    JOIN spaces t ON t.id = e.pid AND t.slug = @within AND t.deleted_at IS NULL
    JOIN spaces dc ON dc.id = e.cid AND dc.deleted_at IS NULL
    UNION ALL
    -- Recursive: children of any node already in the subtree.
    SELECT e.cid, st.depth + 1
    FROM subtree st
    JOIN (
        SELECT sps.parent_space_id AS pid, sps.space_id AS cid FROM space_parent_spaces sps
        UNION ALL
        SELECT c.parent_id, c.id FROM spaces c WHERE c.parent_id IS NOT NULL
    ) e ON e.pid = st.id
    JOIN spaces dc ON dc.id = e.cid AND dc.deleted_at IS NULL
    WHERE st.depth < 3
)
SELECT sp.id, sp.slug, sp.name, sp.geo_code, sp.region, sp.level,
       sp.logo_media_id, sp.description,
       -- "Guides" = pages with real prose — a seeded stub has zero
       -- revisions, so requiring one approved non-fields revision keeps
       -- the badge from reporting the uniform stub count as guides.
       COUNT(wp.id) FILTER (
           WHERE wp.is_active = true AND EXISTS (
               SELECT 1 FROM wiki_revisions wr
               WHERE wr.wiki_page_id = wp.id
                 AND wr.status = 'approved'
                 AND wr.kind <> 'fields'
                 AND wr.deleted_at IS NULL
           )
       ) AS page_count
FROM spaces sp
JOIN wiki_type_configs wtc ON wtc.space_type_id = sp.space_type_id AND wtc.enabled = true
LEFT JOIN space_types st ON st.id = sp.space_type_id
LEFT JOIN wiki_pages wp ON wp.space_id = sp.id
WHERE sp.status = 'active'
  -- Soft-deleted spaces stay active-status in some rows — exclude them
  -- (ancestor hops were already deleted-checked; the candidate itself
  -- was not, leaking e.g. deleted cities into Popular widgets).
  AND sp.deleted_at IS NULL
  -- §05 amendment — education type carries universities AND member-created
  -- education spaces; wiki is restricted to platform universities only.
  AND (st.slug <> 'education' OR sp.level = 'university')
  AND (@level::text = '' OR sp.level = @level OR st.slug = @level)
  AND (@region::text = '' OR sp.region = @region)
  AND (@search::text = '' OR sp.name ILIKE '%' || @search || '%')
  AND (@parent::text = '' OR sp.parent_id = (SELECT p.id FROM spaces p WHERE p.slug = @parent))
  -- A space is never "within" itself — also guards a cyclic link that
  -- would otherwise let the target match its own ancestor walk.
  AND (@within::text = '' OR sp.slug <> @within)
  -- §80 directories: ?within= membership is the subtree CTE above — the
  -- same ≤3-hop edges over space_parent_spaces ∪ parent_id as
  -- ListWikiDirectoryChildren descends, resolved once for the whole scan.
  AND (@within::text = '' OR sp.id IN (SELECT DISTINCT id FROM subtree))
GROUP BY sp.id, sp.slug, sp.name, sp.geo_code, sp.region, sp.level,
         sp.logo_media_id, sp.description
HAVING (NOT @has_wiki::bool OR COUNT(wp.id) FILTER (WHERE wp.is_active = true) > 0)
-- §wiki-seo guest surface — ?has_content=true hides spaces whose pages are
-- all seeded stubs (zero approved prose revisions); guests see only
-- destinations that actually have written guides.
AND (NOT @has_content::bool OR COUNT(wp.id) FILTER (
    WHERE wp.is_active = true AND EXISTS (
        SELECT 1 FROM wiki_revisions wr
        WHERE wr.wiki_page_id = wp.id
          AND wr.status = 'approved'
          AND wr.kind <> 'fields'
          AND wr.deleted_at IS NULL
    )
) > 0)
-- §80 Reader UX: ?sort=name (default, A–Z) or page_count (most guides
-- first). Each non-selected CASE collapses to all-NULL, so only the
-- picked key orders; sp.name is the stable tiebreak.
ORDER BY
    CASE WHEN @sort::text = 'name' THEN sp.name END ASC,
    CASE WHEN @sort::text = 'page_count' THEN COUNT(wp.id) FILTER (
        WHERE wp.is_active = true AND EXISTS (
            SELECT 1 FROM wiki_revisions wr
            WHERE wr.wiki_page_id = wp.id
              AND wr.status = 'approved'
              AND wr.kind <> 'fields'
              AND wr.deleted_at IS NULL
        )
    ) END DESC,
    sp.name ASC
LIMIT NULLIF(@page_limit::int, 0) OFFSET @page_offset;

-- name: CountWikiDestinations :one
-- §80 Reader UX: total matching the ListWikiDestinations filters (pre-
-- pagination) so the response can carry `total` for "Showing N" counts
-- and numbered pagination. Same predicates, same within-CTE.
WITH RECURSIVE subtree AS (
    SELECT e.cid AS id, 1 AS depth
    FROM (
        SELECT sps.parent_space_id AS pid, sps.space_id AS cid FROM space_parent_spaces sps
        UNION ALL
        SELECT c.parent_id, c.id FROM spaces c WHERE c.parent_id IS NOT NULL
    ) e
    JOIN spaces t ON t.id = e.pid AND t.slug = @within AND t.deleted_at IS NULL
    JOIN spaces dc ON dc.id = e.cid AND dc.deleted_at IS NULL
    UNION ALL
    SELECT e.cid, st.depth + 1
    FROM subtree st
    JOIN (
        SELECT sps.parent_space_id AS pid, sps.space_id AS cid FROM space_parent_spaces sps
        UNION ALL
        SELECT c.parent_id, c.id FROM spaces c WHERE c.parent_id IS NOT NULL
    ) e ON e.pid = st.id
    JOIN spaces dc ON dc.id = e.cid AND dc.deleted_at IS NULL
    WHERE st.depth < 3
)
SELECT COUNT(*) FROM (
    SELECT sp.id
    FROM spaces sp
    JOIN wiki_type_configs wtc ON wtc.space_type_id = sp.space_type_id AND wtc.enabled = true
    LEFT JOIN space_types st ON st.id = sp.space_type_id
    LEFT JOIN wiki_pages wp ON wp.space_id = sp.id
    WHERE sp.status = 'active'
      AND sp.deleted_at IS NULL
      AND (st.slug <> 'education' OR sp.level = 'university')
      AND (@level::text = '' OR sp.level = @level OR st.slug = @level)
      AND (@region::text = '' OR sp.region = @region)
      AND (@search::text = '' OR sp.name ILIKE '%' || @search || '%')
      AND (@parent::text = '' OR sp.parent_id = (SELECT p.id FROM spaces p WHERE p.slug = @parent))
      AND (@within::text = '' OR sp.slug <> @within)
      AND (@within::text = '' OR sp.id IN (SELECT DISTINCT id FROM subtree))
    GROUP BY sp.id
    HAVING (NOT @has_wiki::bool OR COUNT(wp.id) FILTER (WHERE wp.is_active = true) > 0)
-- §wiki-seo guest surface — ?has_content=true hides spaces whose pages are
-- all seeded stubs (zero approved prose revisions); guests see only
-- destinations that actually have written guides.
AND (NOT @has_content::bool OR COUNT(wp.id) FILTER (
    WHERE wp.is_active = true AND EXISTS (
        SELECT 1 FROM wiki_revisions wr
        WHERE wr.wiki_page_id = wp.id
          AND wr.status = 'approved'
          AND wr.kind <> 'fields'
          AND wr.deleted_at IS NULL
    )
) > 0)
) counted;

-- =====================
-- Revisions
-- =====================

-- name: GetApprovedWikiRevision :one
-- kind='fields' revisions are excluded: they carry no prose and approving
-- one must never replace the page body (§80 community field ops).
SELECT wr.id, wr.wiki_page_id, wr.content, wr.summary,
       wr.kind, wr.ops, wr.target_entity_type, wr.target_entity_id,
       wr.status, wr.submitted_by, wr.reviewed_by, wr.reviewed_at,
       wr.apply_error,
       wr.ai_generated, wr.ai_model, wr.ai_prompt_hash,
       wr.hitl_reviewed_by, wr.hitl_reviewed_at, wr.hitl_review_notes,
       wr.created_at,
       COALESCE(u.public_username, u.username, '') AS submitted_by_name,
       up.avatar_url AS submitted_by_avatar_url
FROM wiki_revisions wr
LEFT JOIN users u ON u.id = wr.submitted_by
LEFT JOIN user_profiles up ON up.user_id = wr.submitted_by
WHERE wr.wiki_page_id = @page_id AND wr.status = 'approved'
  AND wr.kind IN ('content', 'mixed') AND wr.deleted_at IS NULL
ORDER BY wr.created_at DESC
LIMIT 1;

-- name: ListWikiRevisions :many
SELECT wr.id, wr.wiki_page_id, wr.content, wr.summary,
       wr.kind, wr.ops, wr.target_entity_type, wr.target_entity_id,
       wr.status, wr.submitted_by, wr.reviewed_by, wr.reviewed_at,
       wr.apply_error,
       wr.ai_generated, wr.ai_model, wr.ai_prompt_hash,
       wr.hitl_reviewed_by, wr.hitl_reviewed_at, wr.hitl_review_notes,
       wr.created_at,
       COALESCE(u.public_username, u.username, '') AS submitted_by_name,
       COALESCE(ru.public_username, ru.username, '') AS reviewed_by_name
FROM wiki_revisions wr
LEFT JOIN users u ON u.id = wr.submitted_by
LEFT JOIN users ru ON ru.id = wr.reviewed_by
WHERE wr.wiki_page_id = @page_id AND wr.deleted_at IS NULL
ORDER BY wr.created_at DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: CountWikiRevisionsByPage :one
SELECT COUNT(*) FROM wiki_revisions
WHERE wiki_page_id = @page_id AND deleted_at IS NULL;

-- name: InsertWikiRevision :one
-- Shared INSERT for SubmitRevision/SubmitAIRevision. content_hash is
-- computed in Go (Rule 12) and passed in.
INSERT INTO wiki_revisions (wiki_page_id, kind, content, ops, summary,
    target_entity_type, target_entity_id, status,
    submitted_by, ai_generated, ai_model, ai_prompt_hash, content_hash)
VALUES (@wiki_page_id, @kind, @content, @ops, @summary,
    @target_entity_type, @target_entity_id, 'pending',
    @submitted_by, @ai_generated, @ai_model, @ai_prompt_hash, @content_hash)
RETURNING id;

-- name: ApproveWikiRevision :one
-- Page-scoped approve (legacy route): flips a pending revision, stamps the
-- reviewer, returns the content for RAG indexing. The page's updated_at is
-- bumped by UpdateWikiPageTouched in the same tx.
UPDATE wiki_revisions
SET status = 'approved', reviewed_by = @reviewed_by, reviewed_at = now(),
    hitl_reviewed_by = @reviewed_by, hitl_reviewed_at = now(),
    hitl_review_notes = sqlc.narg(hitl_review_notes)
WHERE id = @id AND status = 'pending' AND deleted_at IS NULL
RETURNING content;

-- name: ApproveWikiRevisionByID :one
-- Revision-keyed approve (canonical; §80): same status flip but returns the
-- full row so the caller can apply ops and enqueue propagation.
UPDATE wiki_revisions
SET status = 'approved', reviewed_by = @reviewed_by, reviewed_at = now(),
    apply_error = NULL,
    hitl_reviewed_by = @reviewed_by, hitl_reviewed_at = now(),
    hitl_review_notes = sqlc.narg(hitl_review_notes)
WHERE id = @id AND status = 'pending' AND deleted_at IS NULL
RETURNING id, wiki_page_id, content, summary,
    kind, ops, target_entity_type, target_entity_id,
    status, submitted_by, reviewed_by, reviewed_at,
    apply_error,
    ai_generated, ai_model, ai_prompt_hash,
    hitl_reviewed_by, hitl_reviewed_at, hitl_review_notes,
    created_at;

-- name: UpdateWikiPageTouched :exec
UPDATE wiki_pages SET updated_at = now() WHERE id = @id;

-- name: RejectWikiRevision :execrows
-- Rows-affected variant (was :exec): a non-pending or missing revision must
-- surface ErrRevisionNotPending to the caller instead of a false 200.
UPDATE wiki_revisions
SET status = 'rejected', reviewed_by = @reviewed_by, reviewed_at = now(),
    hitl_reviewed_by = @reviewed_by, hitl_reviewed_at = now(),
    hitl_review_notes = sqlc.narg(hitl_review_notes)
WHERE id = @id AND status = 'pending' AND deleted_at IS NULL;

-- name: GetWikiRevision :one
SELECT wr.id, wr.wiki_page_id, wr.content, wr.summary,
       wr.kind, wr.ops, wr.target_entity_type, wr.target_entity_id,
       wr.status, wr.submitted_by, wr.reviewed_by, wr.reviewed_at,
       wr.apply_error,
       wr.ai_generated, wr.ai_model, wr.ai_prompt_hash,
       wr.hitl_reviewed_by, wr.hitl_reviewed_at, wr.hitl_review_notes,
       wr.created_at,
       COALESCE(u.public_username, u.username, '') AS submitted_by_name,
       COALESCE(ru.public_username, ru.username, '') AS reviewed_by_name
FROM wiki_revisions wr
LEFT JOIN users u ON u.id = wr.submitted_by
LEFT JOIN users ru ON ru.id = wr.reviewed_by
WHERE wr.id = @id;

-- name: SetWikiRevisionApplyError :exec
-- Records an ops-application failure outside the rolled-back approve tx —
-- the reason persists so reviewers see why the revision cannot be approved.
UPDATE wiki_revisions SET apply_error = @apply_error WHERE id = @id;

-- name: ListWikiRevisionsByTarget :many
SELECT wr.id, wr.wiki_page_id, wr.content, wr.summary,
       wr.kind, wr.ops, wr.target_entity_type, wr.target_entity_id,
       wr.status, wr.submitted_by, wr.reviewed_by, wr.reviewed_at,
       wr.apply_error,
       wr.ai_generated, wr.ai_model, wr.ai_prompt_hash,
       wr.hitl_reviewed_by, wr.hitl_reviewed_at, wr.hitl_review_notes,
       wr.created_at,
       COALESCE(u.public_username, u.username, '') AS submitted_by_name,
       COALESCE(ru.public_username, ru.username, '') AS reviewed_by_name
FROM wiki_revisions wr
LEFT JOIN users u ON u.id = wr.submitted_by
LEFT JOIN users ru ON ru.id = wr.reviewed_by
WHERE wr.target_entity_type = @target_entity_type
  AND wr.target_entity_id = @target_entity_id
  AND wr.deleted_at IS NULL
ORDER BY wr.created_at DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: CountWikiRevisionsByTarget :one
SELECT COUNT(*) FROM wiki_revisions
WHERE target_entity_type = @target_entity_type
  AND target_entity_id = @target_entity_id
  AND deleted_at IS NULL;

-- name: AdminListWikiRevisions :many
-- The admin UI's search box sends ?q= — matched against the revision
-- summary and the submitter's username (§80 review queue). ?deleted=only|all
-- toggles soft-deleted rows ('' keeps the queue to live revisions).
SELECT wr.id, wr.wiki_page_id, wr.content, wr.summary,
       wr.kind, wr.ops, wr.target_entity_type, wr.target_entity_id,
       wr.status, wr.submitted_by, wr.reviewed_by, wr.reviewed_at,
       wr.apply_error,
       wr.ai_generated, wr.ai_model, wr.ai_prompt_hash,
       wr.hitl_reviewed_by, wr.hitl_reviewed_at, wr.hitl_review_notes,
       wr.created_at, wr.deleted_at,
       COALESCE(u.username, '') AS submitted_by_name,
       COALESCE(ru.username, '') AS reviewed_by_name
FROM wiki_revisions wr
LEFT JOIN users u ON u.id = wr.submitted_by
LEFT JOIN users ru ON ru.id = wr.reviewed_by
WHERE (@kind::text = '' OR wr.kind = @kind)
  AND (@status::text = '' OR wr.status = @status)
  AND (@q::text = '' OR wr.summary ILIKE '%' || @q || '%'
       OR u.username ILIKE '%' || @q || '%'
       OR wr.target_entity_type ILIKE '%' || @q || '%')
  AND (CASE @deleted::text
         WHEN 'only' THEN wr.deleted_at IS NOT NULL
         WHEN 'all'  THEN TRUE
         ELSE wr.deleted_at IS NULL END)
ORDER BY
  CASE WHEN @sort = 'created_at:asc'    THEN wr.created_at END ASC NULLS LAST,
  CASE WHEN @sort = 'created_at:desc'   THEN wr.created_at END DESC NULLS LAST,
  CASE WHEN @sort = 'status:asc'        THEN wr.status END ASC NULLS LAST,
  CASE WHEN @sort = 'status:desc'       THEN wr.status END DESC NULLS LAST,
  CASE WHEN @sort = 'kind:asc'          THEN wr.kind END ASC NULLS LAST,
  CASE WHEN @sort = 'kind:desc'         THEN wr.kind END DESC NULLS LAST,
  CASE WHEN @sort = 'submitted_by:asc'  THEN u.username END ASC NULLS LAST,
  CASE WHEN @sort = 'submitted_by:desc' THEN u.username END DESC NULLS LAST,
  wr.created_at DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: AdminCountWikiRevisions :one
SELECT COUNT(*) FROM wiki_revisions wr
LEFT JOIN users u ON u.id = wr.submitted_by
WHERE (@kind::text = '' OR wr.kind = @kind)
  AND (@status::text = '' OR wr.status = @status)
  AND (@q::text = '' OR wr.summary ILIKE '%' || @q || '%'
       OR u.username ILIKE '%' || @q || '%'
       OR wr.target_entity_type ILIKE '%' || @q || '%')
  AND (CASE @deleted::text
         WHEN 'only' THEN wr.deleted_at IS NOT NULL
         WHEN 'all'  THEN TRUE
         ELSE wr.deleted_at IS NULL END);

-- name: CountPendingRevisionsByTarget :one
SELECT COUNT(*) FROM wiki_revisions
WHERE target_entity_type = @target_entity_type
  AND target_entity_id = @target_entity_id
  AND status = 'pending'
  AND deleted_at IS NULL;

-- name: SoftDeleteWikiRevision :execrows
-- Admin delete on a revision row (§80): marks deleted_at — every reader
-- (public history, approved-content lookup, moderation) excludes the row
-- via its deleted_at IS NULL predicate.
UPDATE wiki_revisions SET deleted_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: HardDeleteWikiRevision :execrows
-- Permanent delete — only for already-soft-deleted revisions
-- (admin.hard_delete-gated ?force=true path, Rule 11).
DELETE FROM wiki_revisions WHERE id = @id AND deleted_at IS NOT NULL;

-- name: WikiRevisionExists :one
-- Existence probe for the hard-delete path: distinguishes 404
-- revision_not_found from 409 revision_not_deleted.
SELECT EXISTS(SELECT 1 FROM wiki_revisions WHERE id = @id) AS exists;

-- =====================
-- Page writes & spaces
-- =====================

-- name: FindWikiHostPage :one
-- First active wiki page for a space+category — the "host page" an
-- entity-bound ops revision attaches to (§80).
SELECT id FROM wiki_pages
WHERE space_id = @space_id AND category = @category AND is_active = true
ORDER BY created_at ASC
LIMIT 1;

-- name: GetSpaceTypeRef :one
SELECT sp.space_type_id, COALESCE(st.slug, '') AS space_type_slug, sp.level
FROM spaces sp
LEFT JOIN space_types st ON st.id = sp.space_type_id
WHERE sp.id = @space_id;

-- name: UpdateWikiPage :execrows
-- Optional-field update: NULL params keep the current column value;
-- update_data_entity_type distinguishes "don't touch" from "set/clear" —
-- an empty string clears the binding to NULL (§80 data_entity_type).
UPDATE wiki_pages
SET is_locked = COALESCE(sqlc.narg(is_locked), is_locked),
    is_featured = COALESCE(sqlc.narg(is_featured), is_featured),
    is_active = COALESCE(sqlc.narg(is_active), is_active),
    position = COALESCE(sqlc.narg(position), position),
    data_entity_type = CASE WHEN @update_data_entity_type::bool
                        THEN NULLIF(@data_entity_type, '')
                        ELSE data_entity_type END,
    updated_at = now()
WHERE id = @id;

-- name: SoftDeleteWikiPage :execrows
-- Soft delete (§80): marks deleted_at and deactivates — every is_active=true
-- reader (public GETs, destination page counts, is_japabot_indexed) excludes
-- the row without a schema-wide predicate change.
UPDATE wiki_pages
SET deleted_at = now(), is_active = false, updated_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: HardDeleteWikiPage :execrows
-- Permanent delete — only for already-soft-deleted pages. Revisions cascade
-- via wiki_revisions_wiki_page_id_fkey (ON DELETE CASCADE).
DELETE FROM wiki_pages WHERE id = @id AND deleted_at IS NOT NULL;

-- name: WikiPageExists :one
-- Existence probe used by the hard-delete path to distinguish 404 from
-- 409 page_not_deleted.
SELECT EXISTS(SELECT 1 FROM wiki_pages WHERE id = @id) AS exists;

-- name: GetWikiPageIndexState :one
-- RAG lifecycle probe (§69/ADR-069): the generated is_japabot_indexed flag
-- (is_active AND NOT is_locked) decides whether the page's
-- content_embeddings chunks may be retrieved. The wiki admin handlers
-- read this after flag mutations to decide between PurgeEntity and
-- re-index.
SELECT wp.is_active, wp.is_locked, wp.is_japabot_indexed,
       wp.deleted_at IS NOT NULL AS is_deleted
FROM wiki_pages wp
WHERE wp.id = @page_id;

-- name: GetWikiSpaceBySlug :one
-- Resolves a wiki-bearing space slug: the space's type must have an enabled
-- wiki_type_configs row (§80 Wiki Types amendment). Returns the display name —
-- lazy template/category labels substitute it for the {name} token.
SELECT sp.id, sp.name
FROM spaces sp
JOIN wiki_type_configs wtc ON wtc.space_type_id = sp.space_type_id AND wtc.enabled = true
WHERE sp.slug = @slug AND sp.status = 'active'
 -- §05 amendment — education-typed spaces need level='university' for wiki.
 AND (sp.level = 'university' OR NOT EXISTS (
 SELECT 1 FROM space_types st WHERE st.id = sp.space_type_id AND st.slug = 'education'))
LIMIT 1;

-- name: GetWikiSpaceByID :one
SELECT sp.slug, sp.level
FROM spaces sp
JOIN wiki_type_configs wtc ON wtc.space_type_id = sp.space_type_id AND wtc.enabled = true
WHERE sp.id = @id AND sp.status = 'active'
 -- §05 amendment — education-typed spaces need level='university' for wiki.
 AND (sp.level = 'university' OR NOT EXISTS (
 SELECT 1 FROM space_types st WHERE st.id = sp.space_type_id AND st.slug = 'education'))
LIMIT 1;

-- name: ListWikiSeedSpaceIDs :many
-- Spaces whose type opts into stub seeding (seed_stubs=true) — the
-- wiki:seed_stub_pages fan-out set (§80). Replaces the country-only list.
SELECT s.id
FROM spaces s
JOIN wiki_type_configs wtc ON wtc.space_type_id = s.space_type_id AND wtc.seed_stubs = true
WHERE s.status = 'active' AND s.deleted_at IS NULL
 -- §05 amendment — education-typed spaces need level='university' for wiki.
 AND (s.level = 'university' OR NOT EXISTS (
 SELECT 1 FROM space_types st WHERE st.id = s.space_type_id AND st.slug = 'education'));

-- name: IsUniversitySpaceBySlug :one
SELECT (sp.level = 'university') AS is_university
FROM spaces sp
LEFT JOIN space_types st ON st.id = sp.space_type_id
WHERE sp.slug = @slug AND sp.status = 'active'
LIMIT 1;

-- name: CreateWikiPage :one
-- ON CONFLICT carries the partial-index predicate (deleted_at IS NULL) so
-- inference matches wiki_pages_space_id_slug_key — a soft-deleted page no
-- longer blocks the slug (000744).
INSERT INTO wiki_pages (space_id, slug, title, category, created_by,
    page_template_id, field_group_id, position)
VALUES (@space_id, @slug, @title, @category, sqlc.narg(created_by),
    sqlc.narg(page_template_id), sqlc.narg(field_group_id),
    COALESCE(sqlc.narg(position), 0))
ON CONFLICT (space_id, slug) WHERE deleted_at IS NULL DO NOTHING
RETURNING id;

-- name: InsertInitialWikiRevision :one
-- Initial pending revision for a page created with content
-- (CreatePageWithRevision) — kind='content', empty ops.
INSERT INTO wiki_revisions (wiki_page_id, kind, content, ops, summary,
    target_entity_type, target_entity_id, status,
    submitted_by, ai_generated, ai_model, ai_prompt_hash, content_hash)
VALUES (@wiki_page_id, 'content', @content, '[]'::jsonb, sqlc.narg(summary),
    NULL, NULL, 'pending', @submitted_by, false, NULL, NULL, @content_hash)
RETURNING id;

-- =====================
-- Admin page list
-- =====================

-- name: AdminListWikiPages :many
-- Filtered + whitelisted-sort admin list. Sort keys arrive as "<key>:<dir>"
-- and are applied via per-column CASE expressions (user input can never
-- reach ORDER BY). The latest revision's id/status/excerpt comes from a
-- LEFT JOIN LATERAL so pages without revisions still list.
SELECT wp.id, wp.space_id, wp.slug, wp.title, wp.category,
       wp.is_active, wp.is_featured, wp.is_locked, wp.created_by,
       wp.created_at, wp.updated_at, wp.is_japabot_indexed,
       wp.data_entity_type, wp.deleted_at,
       wp.page_template_id, wp.field_group_id, wp.position,
       sp.slug AS space_slug, sp.name AS space_name,
       lr.id AS latest_revision_id,
       COALESCE(lr.status, '') AS latest_revision_status,
       COALESCE(lr.excerpt, '') AS excerpt,
       COALESCE(lr.ai_generated, false) AS latest_revision_ai_generated,
       lr.ai_model AS latest_revision_ai_model,
       COALESCE(fg.slug, '') AS field_group_slug,
       COALESCE(fg.name, '') AS field_group_name
FROM wiki_pages wp
JOIN spaces sp ON sp.id = wp.space_id
-- The page's bound field group (§80) — the shared structured schema
-- surfaced in the admin view so reviewers can jump to the group editor.
LEFT JOIN custom_field_groups fg ON fg.id = wp.field_group_id
LEFT JOIN LATERAL (
    SELECT wr.id, wr.status, wr.ai_generated, wr.ai_model,
           -- content is TipTap JSONB; regexp_replace left raw JSON in the
           -- excerpt. Extract the text nodes instead (mirrors the admin
           -- UI's extractContentText walk).
           COALESCE(wr.summary, left(btrim(COALESCE(x.excerpt_text, '')), 200), '') AS excerpt
    FROM wiki_revisions wr
    LEFT JOIN LATERAL (
        SELECT string_agg(t.txt #>> '{}', ' ') AS excerpt_text
        FROM jsonb_path_query(wr.content, '$.**.text ? (@.type() == "string")') AS t(txt)
    ) x ON true
    WHERE wr.wiki_page_id = wp.id AND wr.deleted_at IS NULL
    ORDER BY wr.created_at DESC
    LIMIT 1
) lr ON true
WHERE (@space_id::uuid IS NULL OR wp.space_id = @space_id::uuid)
  AND (@dest_slug::text = '' OR sp.slug = @dest_slug)
  AND (@category::text = '' OR wp.category = @category)
  AND (sqlc.narg(is_locked)::bool IS NULL OR wp.is_locked = sqlc.narg(is_locked)::bool)
  AND (sqlc.narg(is_featured)::bool IS NULL OR wp.is_featured = sqlc.narg(is_featured)::bool)
  AND (sqlc.narg(is_active)::bool IS NULL OR wp.is_active = sqlc.narg(is_active)::bool)
  -- @status filters the LATEST revision's status (pending|approved|rejected)
  -- — the review queue's primary workflow is "pages with pending revisions".
  AND (@status::text = '' OR COALESCE(lr.status, '') = @status)
  AND (@q::text = '' OR wp.title ILIKE '%' || @q || '%'
       OR wp.slug ILIKE '%' || @q || '%'
       OR sp.name ILIKE '%' || @q || '%'
       OR sp.slug ILIKE '%' || @q || '%')
  -- Soft-deleted pages are excluded by default; ?deleted=only|all surfaces
  -- them so admins can force-delete (§80).
  AND CASE @deleted::text
        WHEN 'only' THEN wp.deleted_at IS NOT NULL
        WHEN 'all'  THEN true
        ELSE wp.deleted_at IS NULL
      END
ORDER BY
  CASE WHEN @sort = 'title:asc'        THEN wp.title END ASC NULLS LAST,
  CASE WHEN @sort = 'title:desc'       THEN wp.title END DESC NULLS LAST,
  CASE WHEN @sort = 'space_slug:asc'   THEN sp.slug END ASC NULLS LAST,
  CASE WHEN @sort = 'space_slug:desc'  THEN sp.slug END DESC NULLS LAST,
  CASE WHEN @sort = 'status:asc'       THEN lr.status END ASC NULLS LAST,
  CASE WHEN @sort = 'status:desc'      THEN lr.status END DESC NULLS LAST,
  CASE WHEN @sort = 'is_locked:asc'    THEN wp.is_locked::int END ASC NULLS LAST,
  CASE WHEN @sort = 'is_locked:desc'   THEN wp.is_locked::int END DESC NULLS LAST,
  CASE WHEN @sort = 'is_featured:asc'  THEN wp.is_featured::int END ASC NULLS LAST,
  CASE WHEN @sort = 'is_featured:desc' THEN wp.is_featured::int END DESC NULLS LAST,
  CASE WHEN @sort = 'is_active:asc'    THEN wp.is_active::int END ASC NULLS LAST,
  CASE WHEN @sort = 'is_active:desc'   THEN wp.is_active::int END DESC NULLS LAST,
  CASE WHEN @sort = 'created_at:asc'   THEN wp.created_at END ASC NULLS LAST,
  CASE WHEN @sort = 'created_at:desc'  THEN wp.created_at END DESC NULLS LAST,
  CASE WHEN @sort = 'updated_at:asc'   THEN wp.updated_at END ASC NULLS LAST,
  CASE WHEN @sort = 'updated_at:desc'  THEN wp.updated_at END DESC NULLS LAST,
  wp.updated_at DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: AdminCountWikiPages :one
SELECT COUNT(*)
FROM wiki_pages wp
JOIN spaces sp ON sp.id = wp.space_id
WHERE (@space_id::uuid IS NULL OR wp.space_id = @space_id::uuid)
  AND (@dest_slug::text = '' OR sp.slug = @dest_slug)
  AND (@category::text = '' OR wp.category = @category)
  AND (sqlc.narg(is_locked)::bool IS NULL OR wp.is_locked = sqlc.narg(is_locked)::bool)
  AND (sqlc.narg(is_featured)::bool IS NULL OR wp.is_featured = sqlc.narg(is_featured)::bool)
  AND (sqlc.narg(is_active)::bool IS NULL OR wp.is_active = sqlc.narg(is_active)::bool)
  -- Mirrors the list query's LATERAL "latest revision" semantics so the
  -- status filter counts the same rows the list returns.
  AND (@status::text = '' OR COALESCE((
         SELECT wr.status FROM wiki_revisions wr
         WHERE wr.wiki_page_id = wp.id AND wr.deleted_at IS NULL
         ORDER BY wr.created_at DESC
         LIMIT 1), '') = @status)
  AND (@q::text = '' OR wp.title ILIKE '%' || @q || '%'
       OR wp.slug ILIKE '%' || @q || '%'
       OR sp.name ILIKE '%' || @q || '%'
       OR sp.slug ILIKE '%' || @q || '%')
  AND CASE @deleted::text
        WHEN 'only' THEN wp.deleted_at IS NOT NULL
        WHEN 'all'  THEN true
        ELSE wp.deleted_at IS NULL
      END;

-- name: WikiPageSlugExists :one
-- Slug uniqueness probe scoped to (space_id, slug) — used by the intel
-- promoter's suffix loop before inserting a promoted page (§80 Intel
-- ingestion → wiki_pages). Mirrors the partial unique index: soft-deleted
-- pages no longer occupy their slug (000744).
SELECT EXISTS(SELECT 1 FROM wiki_pages WHERE space_id = @space_id AND slug = @slug AND deleted_at IS NULL) AS exists;

-- =====================
-- Page Feedback (§80 Reader UX)
-- =====================

-- name: UpsertWikiPageFeedback :one
-- Upsert the member's vote; the service pre-reads the current vote so a
-- repeated vote can retract instead (toggle semantics live in the service).
INSERT INTO wiki_page_feedback (wiki_page_id, user_id, vote)
VALUES (@wiki_page_id, @user_id, @vote)
ON CONFLICT (wiki_page_id, user_id)
DO UPDATE SET vote = EXCLUDED.vote, updated_at = now()
RETURNING id;

-- name: DeleteWikiPageFeedback :exec
DELETE FROM wiki_page_feedback
WHERE wiki_page_id = @wiki_page_id AND user_id = @user_id;

-- name: GetWikiPageViewerVote :one
SELECT vote FROM wiki_page_feedback
WHERE wiki_page_id = @wiki_page_id AND user_id = @user_id;

-- name: GetWikiPageFeedbackCounts :one
-- Aggregate votes for one page. Reads are per-page; the page_id index
-- covers it. The viewer's own vote is a separate GetWikiPageViewerVote
-- lookup — sqlc can't type a correlated subquery inside this aggregate.
SELECT
    COUNT(*) FILTER (WHERE f.vote = 'up') AS up_count,
    COUNT(*) FILTER (WHERE f.vote = 'down') AS down_count
FROM wiki_page_feedback f
WHERE f.wiki_page_id = @wiki_page_id;

-- =====================
-- Entity-bound pages (JCE ↔ Wiki mapping, 000899+)
-- =====================

-- name: GetWikiEntityKind :one
SELECT entity_kind, source_table, id_column, slug_column, label
FROM wiki_entity_kinds
WHERE entity_kind = @entity_kind;

-- name: GetWikiTypeConfigForKind :one
-- Resolve the wiki type config backing an entity-bound type key slug
-- (e.g. 'student_visa') — returns the entity kind + discriminator.
SELECT wtc.space_type_id, wtc.enabled, wtc.label, wtc.entity_kind,
       wtc.entity_discriminator, st.slug AS type_slug, st.name AS type_name
FROM wiki_type_configs wtc
JOIN space_types st ON st.id = wtc.space_type_id
WHERE st.slug = @type_slug AND wtc.enabled = true;

-- name: ListEntityWikiTypesByKind :many
-- All enabled entity-bound wiki types of a kind — used to resolve the
-- canonical type key for an entity's wiki pages: the discriminator-
-- matching type (e.g. 'student_visa') wins over the undiscriminated
-- generic type ('visa_route'). Deterministic order for stable picks.
SELECT st.slug, st.name AS type_name, wtc.entity_discriminator, wtc.position
FROM wiki_type_configs wtc
JOIN space_types st ON st.id = wtc.space_type_id
WHERE wtc.entity_kind = @entity_kind AND wtc.enabled = true
ORDER BY wtc.position, st.slug;

-- name: GetEntityWikiPage :one
SELECT wp.id, wp.space_id, wp.entity_type, wp.entity_id, wp.slug, wp.title, wp.category,
       wp.is_active, wp.is_featured, wp.is_locked, wp.created_by,
       wp.created_at, wp.updated_at, wp.is_japabot_indexed,
       wp.data_entity_type, wp.page_template_id, wp.field_group_id, wp.position
FROM wiki_pages wp
WHERE wp.entity_type = @entity_type AND wp.entity_id = @entity_id
  AND wp.slug = @page_slug
  AND wp.is_active = true AND wp.deleted_at IS NULL
LIMIT 1;

-- name: ListEntityWikiPages :many
SELECT wp.id, wp.entity_type, wp.entity_id, wp.slug, wp.title, wp.category,
       wp.is_active, wp.is_featured, wp.position
FROM wiki_pages wp
WHERE wp.entity_type = @entity_type AND wp.entity_id = @entity_id
  AND wp.is_active = true AND wp.deleted_at IS NULL
ORDER BY wp.category, wp.title;

-- name: EntityWikiPageRowExists :one
SELECT EXISTS (
    SELECT 1 FROM wiki_pages wp
    WHERE wp.entity_type = @entity_type AND wp.entity_id = @entity_id
      AND wp.slug = @page_slug
) AS exists;

-- name: CreateEntityWikiPage :one
INSERT INTO wiki_pages (space_id, entity_type, entity_id, slug, title, category,
                        is_active, created_by, page_template_id, field_group_id)
VALUES (NULL, @entity_type, @entity_id, @slug, @title, @category,
        true, @created_by, @page_template_id, @field_group_id)
RETURNING id, entity_type, entity_id, slug, title, category;

-- ---------------------------------------------------------------------------
-- Per-kind entity lookups — slug → row for page resolution, and children of
-- a host space for the destinations tree. One block per source table.
-- ---------------------------------------------------------------------------

-- name: GetVisaRouteForWiki :one
SELECT id, slug, name, destination_space_id, visa_category
FROM visa_routes
WHERE slug = @slug AND is_active = true
LIMIT 1;

-- name: ListVisaRoutesForWiki :many
-- Visa routes hosted by a destination space (country).
SELECT id, slug, name, visa_category
FROM visa_routes
WHERE destination_space_id = @space_id AND is_active = true
  AND (@visa_category::text = '' OR visa_category = @visa_category)
ORDER BY name;

-- name: GetOrganizationForWiki :one
SELECT id, slug, name, geo_space_id, org_type
FROM organizations
WHERE slug = @slug AND deleted_at IS NULL AND status = 'active'
LIMIT 1;

-- name: ListOrganizationsForWiki :many
-- Organizations (e.g. banks) hosted by a geo space, filtered by org_type.
SELECT id, slug, name, org_type
FROM organizations
WHERE geo_space_id = @space_id AND deleted_at IS NULL AND status = 'active'
  AND (@org_type::text = '' OR org_type = @org_type)
ORDER BY name;

-- name: GetBankForWiki :one
SELECT id, slug, name, geo_space_id
FROM banks
WHERE slug = @slug AND is_active = true
LIMIT 1;

-- name: ListBanksForWiki :many
-- Banks (informational entities — migration 001045) hosted by a geo space.
SELECT id, slug, name
FROM banks
WHERE geo_space_id = @space_id AND is_active = true
ORDER BY name;

-- name: GetBankBySlug :one
SELECT id, slug
FROM banks
WHERE slug = @slug AND is_active = true
LIMIT 1;

-- name: GetEmbassyForWiki :one
SELECT id, slug, name, host_space_id
FROM embassies
WHERE slug = @slug AND is_active = true
LIMIT 1;

-- name: ListEmbassiesForWiki :many
SELECT id, slug, name
FROM embassies
WHERE host_space_id = @space_id AND is_active = true
ORDER BY name;

-- name: GetCourseOfferingForWiki :one
SELECT o.id, o.slug, o.university_id, o.course_id,
       c.title AS name, c.slug AS program_slug
FROM university_course_offerings o
JOIN university_course_catalog c ON c.id = o.course_id
WHERE o.slug = @slug AND o.deleted_at IS NULL AND o.is_published = true
LIMIT 1;

-- name: ListCourseOfferingsForWiki :many
SELECT o.id, o.slug, c.title AS name, c.slug AS program_slug
FROM university_course_offerings o
JOIN university_course_catalog c ON c.id = o.course_id
WHERE o.university_id = @space_id AND o.deleted_at IS NULL AND o.is_published = true
ORDER BY c.title;

-- name: GetProgramForWiki :one
-- Canonical catalog course (program wiki entity) by slug.
SELECT id, slug, title AS name
FROM university_course_catalog
WHERE slug = @slug AND deleted_at IS NULL AND is_published = true
LIMIT 1;

-- name: ListUniversitiesForCourseOffering :many
-- Sibling published offerings of the same catalog course → their host
-- universities ("also offered at" grid on an offering wiki page).
-- offering_slug is the per-uni course wiki page link.
SELECT s.id, s.slug, s.name, s.geo_code, MIN(o2.slug)::text AS offering_slug
FROM university_course_offerings o
JOIN university_course_offerings o2
  ON o2.course_id = o.course_id AND o2.id <> o.id
 AND o2.deleted_at IS NULL AND o2.is_published = true
JOIN spaces s ON s.id = o2.university_id
 AND s.deleted_at IS NULL AND s.status = 'active'
WHERE o.slug = @slug AND o.deleted_at IS NULL AND o.is_published = true
GROUP BY s.id, s.slug, s.name, s.geo_code
ORDER BY s.name
LIMIT 200;

-- name: ListUniversitiesForProgram :many
-- Every published offering of a catalog course → host universities
-- ("universities offering this program" grid on a program wiki page).
SELECT s.id, s.slug, s.name, s.geo_code, MIN(o.slug)::text AS offering_slug
FROM university_course_catalog c
JOIN university_course_offerings o
  ON o.course_id = c.id AND o.deleted_at IS NULL AND o.is_published = true
JOIN spaces s ON s.id = o.university_id
 AND s.deleted_at IS NULL AND s.status = 'active'
WHERE c.slug = @slug AND c.deleted_at IS NULL AND c.is_published = true
GROUP BY s.id, s.slug, s.name, s.geo_code
ORDER BY s.name
LIMIT 200;

-- name: GetScholarshipForWiki :one
SELECT id, slug, title, country_space_id
FROM scholarships
WHERE slug = @slug;

-- name: ListScholarshipsForWiki :many
SELECT id, slug, title
FROM scholarships
WHERE country_space_id = @space_id
ORDER BY title
LIMIT 500;

-- name: GetWikiTypePageTemplateBySlug :one
-- Type-scoped template lookup for entity-bound lazy pages (the space-bound
-- variant joins through spaces; entity types have no space row).
SELECT t.id, t.category_id, t.slug, t.label, t.field_group_id, t.position,
       t.parent_template_id, t.section_label, t.preview_limit,
       t.is_system, t.created_at,
       c.category AS category_token
FROM wiki_type_page_templates t
JOIN wiki_type_categories c ON c.id = t.category_id
WHERE c.space_type_id = @space_type_id AND t.slug = @slug
ORDER BY c.position, t.position
LIMIT 1;

-- name: ListEntityWikiPageSiblings :many
-- Same-category sibling pages on one entity — the entity-bound counterpart
-- of ListPageSiblings (§80 Hierarchical UX): ordered position, created_at,
-- slug and INCLUDING the current row so the service can flag it for
-- Prev/Next derivation.
SELECT wp.id, wp.slug, wp.title, wp.category, wp.position
FROM wiki_pages wp
WHERE wp.entity_type = @entity_type AND wp.entity_id = @entity_id
  AND wp.category = @category
  AND wp.is_active = true AND wp.deleted_at IS NULL
ORDER BY wp.position, wp.created_at, wp.slug;

-- name: PruneWikiRevisions :exec
-- Retention: keep the 5 newest approved revisions per page — the current
-- edition plus the last 4 in history (same window as §44 amendment 5 for
-- doc pages). Only resolved rows prune — pending submissions are never
-- touched. Soft-delete per Rule 11.
UPDATE wiki_revisions wr
SET deleted_at = now()
WHERE wr.wiki_page_id = @page_id
  AND wr.status = 'approved'
  AND wr.deleted_at IS NULL
  AND wr.id NOT IN (
    SELECT w2.id FROM wiki_revisions w2
    WHERE w2.wiki_page_id = @page_id
      AND w2.status = 'approved'
      AND w2.deleted_at IS NULL
    ORDER BY w2.created_at DESC, w2.id DESC
    LIMIT 5
  );

-- name: ClearWikiRevisions :execrows
-- Clear a page's revision history (wiki.manage). Soft-deletes resolved
-- revisions only — pending submissions are never touched (Rule 11). Same
-- history-clearing parity as §44 amendment 5 doc pages.
UPDATE wiki_revisions
SET deleted_at = now()
WHERE wiki_page_id = @page_id
  AND status != 'pending'
  AND deleted_at IS NULL;

-- name: ListWikiEntityKinds :many
SELECT entity_kind, source_table, id_column, slug_column, label
FROM wiki_entity_kinds
ORDER BY entity_kind;

-- name: GetProductBySlug :one
SELECT p.id, p.product_type_id, p.slug, p.name, p.tagline, p.website_url,
       p.logo_id, p.pricing_model, p.price_from, p.rating_avg, p.review_count,
       p.geo_space_id, t.slug AS type_slug
FROM products p
JOIN product_types t ON t.id = p.product_type_id
WHERE p.slug = @slug AND p.deleted_at IS NULL;

-- name: GetProductByID :one
SELECT p.id, p.product_type_id, p.slug, p.name, p.tagline, p.website_url,
       p.logo_id, p.pricing_model, p.price_from, p.rating_avg, p.review_count,
       p.geo_space_id, t.slug AS type_slug
FROM products p
JOIN product_types t ON t.id = p.product_type_id
WHERE p.id = @id AND p.deleted_at IS NULL;

-- name: SearchProducts :many
SELECT p.id, p.slug, p.name, p.tagline, t.slug AS type_slug, t.label AS type_label,
       p.price_from, p.rating_avg, p.review_count,
       m.storage_path AS logo_storage_path, m.storage_bucket AS logo_storage_bucket,
       (SELECT count(*) FROM product_providers pp WHERE pp.product_id = p.id) AS provider_count
FROM products p
JOIN product_types t ON t.id = p.product_type_id
LEFT JOIN media m ON m.id = p.logo_id
WHERE p.deleted_at IS NULL
  AND ($1::text = '' OR t.slug = $1)
  AND ($2::text = '' OR p.name ILIKE '%' || $2 || '%' ESCAPE '\')
ORDER BY p.name, p.id
LIMIT @limit_count OFFSET @offset_count;

-- name: ListProductProviders :many
SELECT pp.provider_id, ap.name, ap.slug AS provider_slug, pp.price_from, pp.offer_url,
       EXISTS (SELECT 1 FROM coupons c WHERE c.provider_id = pp.provider_id AND c.status = 'active') AS has_deal
FROM product_providers pp
JOIN affiliate_providers ap ON ap.id = pp.provider_id
WHERE pp.product_id = @product_id
ORDER BY pp.position, ap.name;

-- name: GetProviderBySlug :one
SELECT id, slug, name, provider_type, logo_media_id, base_url, category,
       commission_rate, is_featured
FROM affiliate_providers
WHERE slug = @slug AND is_active = true;

-- name: GetProviderByID :one
SELECT id, slug, name, provider_type, logo_media_id, base_url, category,
       commission_rate, is_featured
FROM affiliate_providers
WHERE id = @id AND is_active = true;

-- name: SearchProviders :many
SELECT id, slug, name, provider_type, category
FROM affiliate_providers
WHERE is_active = true
  AND ($1::text = '' OR name ILIKE '%' || $1 || '%')
ORDER BY name
LIMIT @limit_count OFFSET @offset_count;

-- name: GetDealBySlug :one
SELECT c.id, c.slug, c.title, c.coupon_type, c.discount_type, c.discount_value,
       c.currency, c.status, c.expires_at, c.referral_url, ap.name AS provider_name
FROM coupons c
LEFT JOIN affiliate_providers ap ON ap.id = c.provider_id
WHERE c.slug = @slug AND c.status = 'active';

-- name: GetDealByID :one
SELECT c.id, c.slug, c.title, c.coupon_type, c.discount_type, c.discount_value,
       c.currency, c.status, c.expires_at, c.referral_url, ap.name AS provider_name
FROM coupons c
LEFT JOIN affiliate_providers ap ON ap.id = c.provider_id
WHERE c.id = @id AND c.status = 'active';

-- name: SearchDeals :many
SELECT c.id, c.slug, c.title, ap.name AS provider_name
FROM coupons c
LEFT JOIN affiliate_providers ap ON ap.id = c.provider_id
WHERE c.status = 'active'
  AND ($1::text = '' OR c.title ILIKE '%' || $1 || '%')
ORDER BY c.title
LIMIT @limit_count OFFSET @offset_count;

-- name: CreateProductType :one
INSERT INTO product_types (slug, label, icon, config)
VALUES (@slug, @label, @icon, @config)
RETURNING id, slug, label, icon, is_enabled, config;

-- name: GetProductTypeBySlug :one
SELECT id, slug, label, icon, is_enabled, config
FROM product_types
WHERE slug = @slug;

-- name: ListProductTypes :many
SELECT slug, label FROM product_types WHERE is_enabled = true ORDER BY label;

-- name: InsertWikiEntityKind :one
INSERT INTO wiki_entity_kinds (entity_kind, label, source_table, id_column, slug_column)
VALUES (@entity_kind, @label, @source_table, @id_column, @slug_column)
ON CONFLICT (entity_kind) DO NOTHING
RETURNING entity_kind;

-- name: CreateProduct :one
INSERT INTO products (product_type_id, slug, name, tagline, description, website_url, logo_id, pricing_model, price_from)
VALUES (@product_type_id, @slug, @name, sqlc.narg(tagline), sqlc.narg(description), sqlc.narg(website_url), sqlc.narg(logo_id), sqlc.narg(pricing_model), sqlc.narg(price_from))
RETURNING id, slug, name;

-- name: UpdateProduct :exec
-- COALESCE keeps omitted (NULL) params; clear_fields[] explicitly nulls
-- a named field — so a PATCH can both preserve and clear per key.
UPDATE products SET
    name = COALESCE(sqlc.narg(name), name),
    product_type_id = COALESCE(sqlc.narg(product_type_id), product_type_id),
    tagline = CASE WHEN 'tagline' = ANY(@clear_fields::text[]) THEN NULL ELSE COALESCE(sqlc.narg(tagline), tagline) END,
    description = CASE WHEN 'description' = ANY(@clear_fields::text[]) THEN NULL ELSE COALESCE(sqlc.narg(description), description) END,
    logo_id = CASE WHEN 'logo_id' = ANY(@clear_fields::text[]) THEN NULL ELSE COALESCE(sqlc.narg(logo_id), logo_id) END,
    website_url = CASE WHEN 'website_url' = ANY(@clear_fields::text[]) THEN NULL ELSE COALESCE(sqlc.narg(website_url), website_url) END,
    pricing_model = CASE WHEN 'pricing_model' = ANY(@clear_fields::text[]) THEN NULL ELSE COALESCE(sqlc.narg(pricing_model), pricing_model) END,
    price_from = CASE WHEN 'price_from' = ANY(@clear_fields::text[]) THEN NULL ELSE COALESCE(sqlc.narg(price_from), price_from) END,
    updated_at = now()
WHERE slug = @slug AND deleted_at IS NULL;

-- name: DeleteProduct :execrows
UPDATE products SET deleted_at = now() WHERE slug = @slug AND deleted_at IS NULL;

-- name: UpsertProductProvider :exec
INSERT INTO product_providers (product_id, provider_id, price_from, offer_url, position)
VALUES (@product_id, @provider_id, sqlc.narg(price_from), sqlc.narg(offer_url), @position)
ON CONFLICT (product_id, provider_id) DO UPDATE
SET price_from = EXCLUDED.price_from, offer_url = EXCLUDED.offer_url, position = EXCLUDED.position;

-- name: RemoveProductProvider :exec
DELETE FROM product_providers WHERE product_id = @product_id AND provider_id = @provider_id;

-- name: CreateWikiEntitySpaceType :one
-- space_types row for an entity-bound product type — wiki_only so it
-- renders through /wiki/e instead of the space flow.
INSERT INTO space_types (name, slug, description, icon, category, is_system, wiki_only)
VALUES (@name, @slug, sqlc.narg(description), sqlc.narg(icon), sqlc.narg(category), true, true)
RETURNING id, slug;

-- name: UpsertWikiEntityTypeConfig :one
-- wiki_type_configs row binding a space type to an entity kind +
-- discriminator (product types bind kind 'product', disc {type_slug}).
INSERT INTO wiki_type_configs (space_type_id, enabled, label, icon, entity_kind, entity_discriminator, seed_stubs, position)
VALUES (@space_type_id, true, sqlc.narg(label), sqlc.narg(icon), @entity_kind, @entity_discriminator::jsonb, false, @position)
ON CONFLICT (space_type_id) DO UPDATE
SET entity_kind = EXCLUDED.entity_kind,
    entity_discriminator = EXCLUDED.entity_discriminator,
    enabled = true
RETURNING space_type_id, entity_kind;

-- Products/providers/deals are de-wikied (001080) — no *ForWiki lookups:
-- they resolve through their JCE resolvers and the public /products API.
-- name: CreateProgramForWiki :one
-- Mirrors the admin CreateCourseCatalog field set (§39) — the wiki create
-- form exposes the same catalog fields.
INSERT INTO university_course_catalog (title, slug, description, degree_type,
    department, field_of_study, language, default_duration_years, is_published)
VALUES (@title, @slug, sqlc.narg(description), sqlc.narg(degree_type),
    sqlc.narg(department), sqlc.narg(field_of_study),
    COALESCE(sqlc.narg(language), 'English'), sqlc.narg(default_duration_years), true)
RETURNING slug;

-- name: GetSpaceBySlugKind :one
-- Resolve a hosting space by slug + type slug (university for offerings).
SELECT s.id FROM spaces s
JOIN space_types st ON st.id = s.space_type_id
WHERE s.slug = @slug AND st.slug = @type_slug AND s.deleted_at IS NULL;

-- name: CreateOfferingForWiki :one
-- Mirrors the admin CreateCourseOffering field set (§39) — the wiki create
-- form exposes the same offering fields.
INSERT INTO university_course_offerings (course_id, university_id, slug,
    tuition, currency, duration_years, study_mode, intake_months,
    entry_requirements, accreditation, is_published, editorial_overrides)
VALUES (@course_id, @university_id, @slug, sqlc.narg(tuition),
    sqlc.narg(currency), sqlc.narg(duration_years), sqlc.narg(study_mode),
    sqlc.narg(intake_months), sqlc.narg(entry_requirements),
    sqlc.narg(accreditation), true, '{}')
RETURNING slug;

-- name: CreateEmbassyForWiki :one
INSERT INTO embassies (name, slug, host_space_id, is_active)
VALUES (@name, @slug, @host_space_id, true)
RETURNING slug;

-- name: CreateBankForWiki :one
INSERT INTO banks (name, slug, geo_space_id, country_code, is_active)
VALUES (@name, @slug, @geo_space_id, @country_code, true)
RETURNING slug;

-- name: CreateVisaRouteForWiki :one
INSERT INTO visa_routes (name, slug, destination_space_id, visa_category,
    gov_source_url, status, ai_generated, is_active)
VALUES (@name, @slug, @destination_space_id, @visa_category,
    COALESCE(sqlc.narg(gov_source_url), ''), 'published', false, true)
RETURNING slug;

-- name: CreateScholarshipForWiki :one
INSERT INTO scholarships (title, slug, source_url, source_type, trust_level,
    country, fields_of_study, eligible_countries, funding,
    required_documents, language_requirements, last_verified_at)
VALUES (@title, @slug, COALESCE(sqlc.narg(source_url), ''), 'manual', 0,
    COALESCE(sqlc.narg(country), ''), '{}', '{}', '{}', '{}', '{}', now())
RETURNING slug;

-- name: ListSpacesByTypeKind :many
-- Picker source for entity create forms (universities, countries).
-- q filters by name so large sets (10k universities) stay searchable.
SELECT s.id, s.slug, s.name FROM spaces s
JOIN space_types st ON st.id = s.space_type_id
WHERE st.slug = @type_slug AND s.deleted_at IS NULL
  AND (@q::text = '' OR s.name ILIKE '%' || @q || '%')
ORDER BY s.name LIMIT 500;

-- name: GetWikiPageCreatorID :one
-- Author of the page's thread anchor — created_by is NULL on seeded pages.
SELECT created_by FROM wiki_pages WHERE id = @id;
