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
SELECT sp.id, sp.slug, sp.name, sp.geo_code, sp.region, sp.level,
       sp.logo_media_id, sp.description,
       COUNT(wp.id) FILTER (WHERE wp.is_active = true) AS page_count
FROM spaces sp
JOIN wiki_type_configs wtc ON wtc.space_type_id = sp.space_type_id AND wtc.enabled = true
LEFT JOIN space_types st ON st.id = sp.space_type_id
LEFT JOIN wiki_pages wp ON wp.space_id = sp.id
WHERE sp.status = 'active'
  AND (@level::text = '' OR sp.level = @level OR st.slug = @level)
  AND (@region::text = '' OR sp.region = @region)
  AND (@search::text = '' OR sp.name ILIKE '%' || @search || '%')
  AND (@parent::text = '' OR sp.parent_id = (SELECT p.id FROM spaces p WHERE p.slug = @parent))
  -- A space is never "within" itself — also guards a cyclic link that
  -- would otherwise let the target match its own ancestor walk.
  AND (@within::text = '' OR sp.slug <> @within)
  AND (@within::text = '' OR EXISTS (
        -- §80 directories: ancestor walk (≤3 hops) over
        -- space_parent_spaces ∪ parent_id — the same edges
        -- ListWikiDirectoryChildren descends, so ?within= search and the
        -- embedded directory listing agree (the old form followed
        -- parent_id only past hop 1, silently missing junction-only
        -- grandchildren). Every hop's ancestor must be non-deleted;
        -- any linked ancestor slug scopes the list — the former
        -- geo-level guard excluded directory parents like university
        -- or custom types.
        SELECT 1
        FROM (
            SELECT sps.parent_space_id AS pid
            FROM space_parent_spaces sps
            WHERE sps.space_id = sp.id
            UNION
            SELECT sp.parent_id
            WHERE sp.parent_id IS NOT NULL
        ) l1
        JOIN spaces g ON g.id = l1.pid AND g.deleted_at IS NULL
        WHERE g.slug = @within
           OR EXISTS (
               SELECT 1
               FROM (
                   SELECT sps2.parent_space_id AS pid
                   FROM space_parent_spaces sps2
                   WHERE sps2.space_id = g.id
                   UNION
                   SELECT g.parent_id
                   WHERE g.parent_id IS NOT NULL
               ) l2
               JOIN spaces gp ON gp.id = l2.pid AND gp.deleted_at IS NULL
               WHERE gp.slug = @within
                  OR EXISTS (
                      SELECT 1
                      FROM (
                          SELECT sps3.parent_space_id AS pid
                          FROM space_parent_spaces sps3
                          WHERE sps3.space_id = gp.id
                          UNION
                          SELECT gp.parent_id
                          WHERE gp.parent_id IS NOT NULL
                      ) l3
                      JOIN spaces ggp ON ggp.id = l3.pid AND ggp.deleted_at IS NULL
                      WHERE ggp.slug = @within
                  )
           )
      ))
GROUP BY sp.id, sp.slug, sp.name, sp.geo_code, sp.region, sp.level,
         sp.logo_media_id, sp.description
HAVING (NOT @has_wiki::bool OR COUNT(wp.id) FILTER (WHERE wp.is_active = true) > 0)
-- §80 Reader UX: ?sort=name (default, A–Z) or page_count (most guides
-- first). Each non-selected CASE collapses to all-NULL, so only the
-- picked key orders; sp.name is the stable tiebreak.
ORDER BY
    CASE WHEN @sort::text = 'name' THEN sp.name END ASC,
    CASE WHEN @sort::text = 'page_count' THEN COUNT(wp.id) FILTER (WHERE wp.is_active = true) END DESC,
    sp.name ASC
LIMIT NULLIF(@page_limit::int, 0) OFFSET @page_offset;

-- name: CountWikiDestinations :one
-- §80 Reader UX: total matching the ListWikiDestinations filters (pre-
-- pagination) so the response can carry `total` for "Showing N" counts
-- and numbered pagination. Same predicates, same within-CTE.
SELECT COUNT(*) FROM (
    SELECT sp.id
    FROM spaces sp
    JOIN wiki_type_configs wtc ON wtc.space_type_id = sp.space_type_id AND wtc.enabled = true
    LEFT JOIN space_types st ON st.id = sp.space_type_id
    LEFT JOIN wiki_pages wp ON wp.space_id = sp.id
    WHERE sp.status = 'active'
      AND (@level::text = '' OR sp.level = @level OR st.slug = @level)
      AND (@region::text = '' OR sp.region = @region)
      AND (@search::text = '' OR sp.name ILIKE '%' || @search || '%')
      AND (@parent::text = '' OR sp.parent_id = (SELECT p.id FROM spaces p WHERE p.slug = @parent))
      AND (@within::text = '' OR sp.slug <> @within)
      AND (@within::text = '' OR EXISTS (
            SELECT 1
            FROM (
                SELECT sps.parent_space_id AS pid
                FROM space_parent_spaces sps
                WHERE sps.space_id = sp.id
                UNION
                SELECT sp.parent_id
                WHERE sp.parent_id IS NOT NULL
            ) l1
            JOIN spaces g ON g.id = l1.pid AND g.deleted_at IS NULL
            WHERE g.slug = @within
               OR EXISTS (
                   SELECT 1
                   FROM (
                       SELECT sps2.parent_space_id AS pid
                       FROM space_parent_spaces sps2
                       WHERE sps2.space_id = g.id
                       UNION
                       SELECT g.parent_id
                       WHERE g.parent_id IS NOT NULL
                   ) l2
                   JOIN spaces gp ON gp.id = l2.pid AND gp.deleted_at IS NULL
                   WHERE gp.slug = @within
                      OR EXISTS (
                          SELECT 1
                          FROM (
                              SELECT sps3.parent_space_id AS pid
                              FROM space_parent_spaces sps3
                              WHERE sps3.space_id = gp.id
                              UNION
                              SELECT gp.parent_id
                              WHERE gp.parent_id IS NOT NULL
                          ) l3
                          JOIN spaces ggp ON ggp.id = l3.pid AND ggp.deleted_at IS NULL
                          WHERE ggp.slug = @within
                      )
               )
          ))
    GROUP BY sp.id
    HAVING (NOT @has_wiki::bool OR COUNT(wp.id) FILTER (WHERE wp.is_active = true) > 0)
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
       COALESCE(u.username, '') AS submitted_by_name,
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
       COALESCE(u.username, '') AS submitted_by_name,
       COALESCE(ru.username, '') AS reviewed_by_name
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
       COALESCE(u.username, '') AS submitted_by_name,
       COALESCE(ru.username, '') AS reviewed_by_name
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
       COALESCE(u.username, '') AS submitted_by_name,
       COALESCE(ru.username, '') AS reviewed_by_name
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
SELECT sp.space_type_id, COALESCE(st.slug, '') AS space_type_slug
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

-- name: GetWikiSpaceBySlug :one
-- Resolves a wiki-bearing space slug: the space's type must have an enabled
-- wiki_type_configs row (§80 Wiki Types amendment). Returns the display name —
-- lazy template/category labels substitute it for the {name} token.
SELECT sp.id, sp.name
FROM spaces sp
JOIN wiki_type_configs wtc ON wtc.space_type_id = sp.space_type_id AND wtc.enabled = true
WHERE sp.slug = @slug AND sp.status = 'active'
LIMIT 1;

-- name: GetWikiSpaceByID :one
SELECT sp.slug, sp.level
FROM spaces sp
JOIN wiki_type_configs wtc ON wtc.space_type_id = sp.space_type_id AND wtc.enabled = true
WHERE sp.id = @id AND sp.status = 'active'
LIMIT 1;

-- name: ListWikiSeedSpaceIDs :many
-- Spaces whose type opts into stub seeding (seed_stubs=true) — the
-- wiki:seed_stub_pages fan-out set (§80). Replaces the country-only list.
SELECT s.id
FROM spaces s
JOIN wiki_type_configs wtc ON wtc.space_type_id = s.space_type_id AND wtc.seed_stubs = true
WHERE s.status = 'active' AND s.deleted_at IS NULL;

-- name: IsUniversitySpaceBySlug :one
SELECT (sp.level = 'university' OR st.slug = 'university') AS is_university
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
