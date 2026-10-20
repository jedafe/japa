-- Wiki types & category templates (§80 Wiki Types & Field-Group Pages
-- amendment, migration 000739). A wiki type is a space_type with an enabled
-- wiki_type_configs row; wiki_type_parents declares allowed nesting; each
-- type's wiki_type_categories template may bind a custom field group.
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md

-- =====================
-- Wiki type configs
-- =====================

-- name: ListWikiTypes :many
-- All wiki types (admin view) with parent edges aggregated per type.
SELECT wtc.space_type_id, st.slug AS space_type_slug, st.name AS space_type_name,
       wtc.enabled, wtc.label, wtc.icon, wtc.seed_stubs, wtc.position, wtc.created_at,
       COALESCE(array_agg(wtp.parent_space_type_id::text) FILTER (WHERE wtp.parent_space_type_id IS NOT NULL), '{}')::text[] AS parent_type_ids
FROM wiki_type_configs wtc
JOIN space_types st ON st.id = wtc.space_type_id
LEFT JOIN wiki_type_parents wtp ON wtp.space_type_id = wtc.space_type_id
GROUP BY wtc.space_type_id, st.slug, st.name,
         wtc.enabled, wtc.label, wtc.icon, wtc.seed_stubs, wtc.position, wtc.created_at
ORDER BY wtc.position, st.name;

-- name: ListEnabledWikiTypes :many
-- Public list — enabled types only, for navigation/destination filters.
SELECT wtc.space_type_id, st.slug AS space_type_slug, st.name AS space_type_name,
       COALESCE(wtc.label, st.name) AS label, wtc.icon, wtc.seed_stubs, wtc.position,
       COALESCE(array_agg(DISTINCT wtp.parent_space_type_id::text)
                FILTER (WHERE wtp.parent_space_type_id IS NOT NULL),
                '{}')::text[] AS parent_type_ids
FROM wiki_type_configs wtc
JOIN space_types st ON st.id = wtc.space_type_id
LEFT JOIN wiki_type_parents wtp ON wtp.space_type_id = wtc.space_type_id
WHERE wtc.enabled = true
GROUP BY wtc.space_type_id, st.slug, st.name, wtc.label, wtc.icon, wtc.seed_stubs, wtc.position
ORDER BY wtc.position, st.name;

-- name: GetWikiTypeConfig :one
SELECT wtc.space_type_id, wtc.enabled, wtc.label, wtc.icon, wtc.seed_stubs,
       wtc.position, wtc.created_at,
       st.slug AS space_type_slug, st.name AS space_type_name
FROM wiki_type_configs wtc
JOIN space_types st ON st.id = wtc.space_type_id
WHERE wtc.space_type_id = @space_type_id;

-- name: UpsertWikiTypeConfig :one
-- Create or re-enable a wiki type for a space type (admin).
INSERT INTO wiki_type_configs (space_type_id, enabled, label, icon, seed_stubs, position)
VALUES (@space_type_id, @enabled, sqlc.narg(label), sqlc.narg(icon), @seed_stubs, @position)
ON CONFLICT (space_type_id) DO UPDATE
SET enabled = EXCLUDED.enabled,
    label = EXCLUDED.label,
    icon = EXCLUDED.icon,
    seed_stubs = EXCLUDED.seed_stubs,
    position = EXCLUDED.position
RETURNING space_type_id, enabled, label, icon, seed_stubs, position, created_at;

-- name: UpdateWikiTypeConfig :execrows
-- Optional-field update; parent edges are replaced separately.
UPDATE wiki_type_configs
SET enabled = COALESCE(sqlc.narg(enabled), enabled),
    label = COALESCE(sqlc.narg(label), label),
    icon = COALESCE(sqlc.narg(icon), icon),
    seed_stubs = COALESCE(sqlc.narg(seed_stubs), seed_stubs),
    position = COALESCE(sqlc.narg(position), position)
WHERE space_type_id = @space_type_id;

-- name: DeleteWikiTypeConfig :execrows
DELETE FROM wiki_type_configs WHERE space_type_id = @space_type_id;

-- name: CountActivePagesBySpaceType :one
-- Delete-type guard: spaces of this type that still have active wiki pages.
SELECT COUNT(*)
FROM wiki_pages wp
JOIN spaces sp ON sp.id = wp.space_id
WHERE sp.space_type_id = @space_type_id AND wp.is_active = true;

-- =====================
-- Wiki type parents
-- =====================

-- name: DeleteWikiTypeParents :exec
DELETE FROM wiki_type_parents WHERE space_type_id = @space_type_id;

-- name: InsertWikiTypeParent :exec
INSERT INTO wiki_type_parents (space_type_id, parent_space_type_id)
VALUES (@space_type_id, @parent_space_type_id)
ON CONFLICT DO NOTHING;

-- name: WikiTypeConfigExists :one
SELECT EXISTS(SELECT 1 FROM wiki_type_configs WHERE space_type_id = @space_type_id) AS exists;

-- =====================
-- Wiki type categories
-- =====================

-- name: ListWikiTypeCategories :many
-- Category template for a type, with the bound field group's identity for
-- admin display. Ordered by position.
SELECT wtc.id, wtc.space_type_id, wtc.slug, wtc.category, wtc.label, wtc.field_group_id,
       wtc.position, wtc.is_system, wtc.rollup, wtc.icon, wtc.created_at,
       cfg.slug AS field_group_slug, cfg.name AS field_group_name
FROM wiki_type_categories wtc
LEFT JOIN custom_field_groups cfg ON cfg.id = wtc.field_group_id
WHERE wtc.space_type_id = @space_type_id
ORDER BY wtc.position, wtc.slug;

-- name: GetWikiCategoryForSpace :one
-- Resolve a page slug for a concrete space: space → space_type_id →
-- category template row (+ bound group). Used by the page route to attach
-- the group's schema to the response.
SELECT wtc.id, wtc.space_type_id, wtc.slug, wtc.category, wtc.label, wtc.field_group_id,
       wtc.position, wtc.is_system, wtc.rollup, wtc.icon, wtc.created_at,
       cfg.slug AS field_group_slug, cfg.name AS field_group_name
FROM wiki_type_categories wtc
JOIN spaces sp ON sp.space_type_id = wtc.space_type_id
LEFT JOIN custom_field_groups cfg ON cfg.id = wtc.field_group_id
WHERE sp.id = @space_id AND (wtc.slug = @slug OR wtc.category = @slug)
LIMIT 1;

-- name: ListWikiCategoriesBySpaceSlug :many
-- Public category list for a space (replaces the hardcoded geo/university
-- slices): resolves the space's type template. field_group_slug lets the
-- index scope its fields panel to the overview category's bound group.
SELECT wtc.id, wtc.slug, wtc.category, wtc.label, wtc.field_group_id, wtc.position, wtc.is_system, wtc.rollup, wtc.icon,
       st.slug AS space_type_slug, fg.slug AS field_group_slug
FROM wiki_type_categories wtc
JOIN spaces sp ON sp.space_type_id = wtc.space_type_id
JOIN space_types st ON st.id = wtc.space_type_id
LEFT JOIN custom_field_groups fg ON fg.id = wtc.field_group_id
WHERE sp.slug = @space_slug AND sp.status = 'active'
ORDER BY wtc.position, wtc.slug;

-- name: ListSeedCategoriesBySpaceType :many
-- Category slugs/labels the stub seeder materializes for a space of this
-- type (the former hardcoded 7-page geo set, now template-driven).
SELECT wtc.slug, wtc.category, wtc.label
FROM wiki_type_categories wtc
WHERE wtc.space_type_id = @space_type_id
ORDER BY wtc.position, wtc.slug;

-- name: CreateWikiTypeCategory :one
INSERT INTO wiki_type_categories (space_type_id, slug, category, label, field_group_id, position, is_system, rollup, icon)
VALUES (@space_type_id, @slug, @category, @label, @field_group_id, @position, @is_system, sqlc.narg(rollup), sqlc.narg(icon))
RETURNING id, space_type_id, slug, category, label, field_group_id, position, is_system, rollup, icon, created_at;

-- name: UpdateWikiTypeCategory :execrows
-- Slug is immutable (page URLs are /wiki/{entity}/{slug}); label, bound
-- group, position, and icon are editable.
UPDATE wiki_type_categories
SET label = COALESCE(sqlc.narg(label), label),
    field_group_id = CASE WHEN @update_field_group::bool
                      THEN sqlc.narg(field_group_id)
                      ELSE field_group_id END,
    position = COALESCE(sqlc.narg(position), position),
    rollup = CASE WHEN @update_rollup::bool
             THEN sqlc.narg(rollup)::jsonb
             ELSE rollup END,
    icon = CASE WHEN @update_icon::bool
           THEN sqlc.narg(icon)
           ELSE icon END
WHERE id = @id;

-- name: DeleteWikiTypeCategory :execrows
-- Non-system categories only; blocked while prose exists (service guard).
DELETE FROM wiki_type_categories
WHERE id = @id AND is_system = false;

-- name: CountCategoryProsePages :one
-- Delete-category guard: pages in this category (any space of the type)
-- that carry an approved content/mixed revision — i.e. real prose.
SELECT COUNT(*)
FROM wiki_pages wp
JOIN spaces sp ON sp.id = wp.space_id
WHERE wp.category = @category AND sp.space_type_id = @space_type_id
  AND EXISTS (
      SELECT 1 FROM wiki_revisions wr
      WHERE wr.wiki_page_id = wp.id AND wr.status = 'approved'
        AND wr.kind IN ('content', 'mixed') AND wr.deleted_at IS NULL
  );

-- name: GetWikiTypeCategory :one
SELECT id, space_type_id, slug, category, label, field_group_id,
       position, is_system, rollup, icon, created_at
FROM wiki_type_categories
WHERE id = @id;

-- name: GetSpaceTypeIDBySlug :one
SELECT id FROM space_types WHERE slug = @slug;

-- name: ListWikiTypeParentIDs :many
SELECT parent_space_type_id
FROM wiki_type_parents
WHERE space_type_id = @space_type_id;

-- name: GetSpaceSeedStubs :one
SELECT COALESCE(wtc.seed_stubs, false) AS seed_stubs
FROM spaces sp
JOIN wiki_type_configs wtc ON wtc.space_type_id = sp.space_type_id
WHERE sp.id = @space_id;

-- name: ListAllWikiCategories :many
SELECT wtc.slug, wtc.category, wtc.label, wtc.field_group_id, wtc.position, wtc.is_system, wtc.rollup, wtc.icon,
       st.slug AS space_type_slug, fg.slug AS field_group_slug
FROM wiki_type_categories wtc
JOIN wiki_type_configs cfg ON cfg.space_type_id = wtc.space_type_id AND cfg.enabled = true
JOIN space_types st ON st.id = wtc.space_type_id
LEFT JOIN custom_field_groups fg ON fg.id = wtc.field_group_id
-- Deterministic tiebreak: the handler dedupes the union by category token,
-- so the winner's fields (label, field_group_id) must not flap on equal
-- (position, slug) across types.
ORDER BY wtc.position, wtc.slug, st.slug;

-- name: ResolveWikiTypeCategoryToken :one
-- Canonicalizes an externally-supplied category token (e.g. an intel
-- extraction category) to the managed wiki_type_categories row for a wiki
-- type. Accepts either spelling: the stored category token
-- ('cost_of_living') or the page slug ('cost-of-living') — hyphenated slugs
-- are compared underscore-normalized so both forms resolve.
-- Ref: §80 Intel ingestion → wiki_pages.
SELECT id, space_type_id, slug, category, label, field_group_id,
       position, is_system, created_at, rollup, icon
FROM wiki_type_categories
WHERE space_type_id = @space_type_id
  AND (lower(category) = lower(@token)
       OR lower(replace(slug, '-', '_')) = lower(@token))
ORDER BY position ASC
LIMIT 1;

-- =====================
-- Wiki page templates (§80 Hierarchical UX & Itemized Pricing amendment)
-- =====================

-- name: ListWikiPageTemplates :many
-- The category's FULL template set — nested topic templates (§80 Wiki
-- Topic Pages) included. Callers scope by parent_template_id: top-level
-- pages are parent IS NULL, a topic's siblings share its parent.
SELECT t.id, t.category_id, t.slug, t.label, t.field_group_id, t.position,
       t.parent_template_id, t.section_label, t.preview_limit,
       t.is_system, t.created_at,
       cfg.slug AS field_group_slug, cfg.name AS field_group_name
FROM wiki_type_page_templates t
LEFT JOIN custom_field_groups cfg ON cfg.id = t.field_group_id
WHERE t.category_id = @category_id
ORDER BY t.position, t.slug;

-- name: ListWikiPageTemplatesBySpaceType :many
-- All page templates across a type's categories — admin editor needs the
-- full set when rendering a category's template list.
SELECT t.id, t.category_id, t.slug, t.label, t.field_group_id, t.position,
       t.parent_template_id, t.section_label, t.preview_limit,
       t.is_system, t.created_at,
       cfg.slug AS field_group_slug, cfg.name AS field_group_name
FROM wiki_type_page_templates t
JOIN wiki_type_categories c ON c.id = t.category_id
LEFT JOIN custom_field_groups cfg ON cfg.id = t.field_group_id
WHERE c.space_type_id = @space_type_id
ORDER BY c.position, t.position, t.slug;

-- name: GetWikiPageTemplate :one
SELECT t.id, t.category_id, t.slug, t.label, t.field_group_id, t.position,
       t.parent_template_id, t.section_label, t.preview_limit,
       t.is_system, t.created_at
FROM wiki_type_page_templates t
WHERE t.id = @id;

-- name: GetWikiPageTemplateForSpace :one
-- Resolve a page slug to the space type's page template: space →
-- space_type_id → the page's category row → template. The category match
-- uses the same token normalization as ResolveCategoryTokenWikiType
-- (category token or dash/underscore-normalized slug) so 'cost_of_living'
-- and 'cost-of-living' both bind. Two categories on one type may declare
-- the same template slug — the category scope keeps resolution
-- deterministic. Returns NULL row (ErrNoRows) when the category has no
-- template for the slug — the page is then a freeform page.
SELECT t.id, t.category_id, t.slug, t.label, t.field_group_id, t.position,
       t.parent_template_id, t.section_label, t.preview_limit,
       t.is_system, t.created_at
FROM wiki_type_page_templates t
JOIN wiki_type_categories c ON c.id = t.category_id
JOIN spaces sp ON sp.space_type_id = c.space_type_id
WHERE sp.id = @space_id AND t.slug = @slug
  AND (lower(c.category) = lower(@category)
       OR lower(replace(c.slug, '-', '_')) = lower(replace(@category, '-', '_')))
LIMIT 1;

-- name: GetWikiPageTemplateBySlug :one
-- Lazy-page resolution (§80): GET /wiki/{space}/{page} renders an
-- unmaterialized template page without a wiki_pages row. The URL carries
-- only the slug, so the template resolves slug-only across the space
-- type's categories — c.position, t.position ordering keeps the pick
-- deterministic when two categories reuse a slug. category_token is the
-- category's wiki_pages.category token (the lazy page's category).
SELECT t.id, t.category_id, t.slug, t.label, t.field_group_id, t.position,
       t.parent_template_id, t.section_label, t.preview_limit,
       t.is_system, t.created_at,
       c.category AS category_token,
       cfg.slug AS field_group_slug
FROM wiki_type_page_templates t
JOIN wiki_type_categories c ON c.id = t.category_id
JOIN spaces sp ON sp.space_type_id = c.space_type_id
LEFT JOIN custom_field_groups cfg ON cfg.id = t.field_group_id
WHERE sp.id = @space_id AND t.slug = @slug
ORDER BY c.position, t.position
LIMIT 1;

-- name: CreateWikiPageTemplate :one
-- parent_template_id nests the template under a same-category top-level
-- template (§80 Wiki Topic Pages); section_label is the label used when
-- the child renders as a composite preview section on the parent page;
-- preview_limit caps the preview rows (0 = all, default 5).
INSERT INTO wiki_type_page_templates
    (category_id, slug, label, field_group_id, parent_template_id,
     section_label, preview_limit, position, is_system)
VALUES (@category_id, @slug, @label, @field_group_id,
        @parent_template_id::uuid, sqlc.narg(section_label),
        COALESCE(sqlc.narg(preview_limit)::int, 5), @position, @is_system)
RETURNING id, category_id, slug, label, field_group_id, position,
          parent_template_id, section_label, preview_limit,
          is_system, created_at;

-- name: UpdateWikiPageTemplate :execrows
-- Slug is immutable (page URLs); label, bound group, position, parentage,
-- and preview config are editable. update_field_group distinguishes
-- "don't touch" from "rebind"; update_parent does the same for
-- parent_template_id (null clears → top-level); update_section_label does
-- the same for section_label (null clears → the child's label is used).
UPDATE wiki_type_page_templates
SET label = COALESCE(sqlc.narg(label), label),
    field_group_id = CASE WHEN @update_field_group::bool
                     THEN @field_group_id
                     ELSE field_group_id END,
    position = COALESCE(sqlc.narg(position), position),
    parent_template_id = CASE WHEN @update_parent::bool
                     THEN @parent_template_id::uuid
                     ELSE parent_template_id END,
    section_label = CASE WHEN @update_section_label::bool
                     THEN sqlc.narg(section_label)
                     ELSE section_label END,
    preview_limit = COALESCE(sqlc.narg(preview_limit), preview_limit)
WHERE id = @id;

-- name: DeleteWikiPageTemplate :execrows
-- Non-system templates only; the service blocks deletion while bound pages
-- carry approved prose or field values (CountTemplateContentPages).
DELETE FROM wiki_type_page_templates
WHERE id = @id AND is_system = false;

-- name: CountTemplateContentPages :one
-- Delete-template guard (§80: "blocked while bound pages carry
-- prose/values"). Counts pages bound to this template (any space) that
-- carry an approved revision of any kind — prose (content/mixed) or field
-- ops (fields/mixed) — OR whose space holds a stored value for any field
-- in the template's bound group (admin SetFieldValue writes bypass
-- revisions entirely).
SELECT COUNT(*)
FROM wiki_pages wp
WHERE wp.page_template_id = @id AND wp.deleted_at IS NULL
  AND (
      EXISTS (
          SELECT 1 FROM wiki_revisions wr
          WHERE wr.wiki_page_id = wp.id AND wr.status = 'approved'
            AND wr.deleted_at IS NULL
      )
      OR EXISTS (
          SELECT 1
          FROM wiki_type_page_templates t
          JOIN custom_fields cf ON cf.group_id = t.field_group_id
          JOIN custom_field_values cfv
            ON cfv.field_id = cf.id
           AND cfv.entity_type = 'space'
           AND cfv.entity_id = wp.space_id
          WHERE t.id = @id AND t.field_group_id IS NOT NULL
            AND (cfv.value_text IS NOT NULL OR cfv.value_number IS NOT NULL
                 OR cfv.value_bool IS NOT NULL OR cfv.value_date IS NOT NULL
                 OR cfv.value_json IS NOT NULL)
      )
  );

-- name: CountPagesByPageTemplate :one
SELECT COUNT(*) FROM wiki_pages wp
WHERE wp.page_template_id = @id AND wp.deleted_at IS NULL;

-- name: ListWikiTypeChildIDs :many
-- Declared child types of a type — the reverse edge of
-- ListWikiTypeParentIDs. Drives rollup descendant scope and the landing
-- chooser's drill-down (§80 Hierarchical UX amendment).
SELECT space_type_id
FROM wiki_type_parents
WHERE parent_space_type_id = @space_type_id;

-- name: ListWikiSpaceParentLinks :many
-- One hop up the actual space hierarchy: parents via the
-- space_parent_spaces junction ∪ spaces.parent_id, each with its type
-- identity and whether it is the primary (parent_id) link. The service
-- walks this per level (depth ≤3) and arbitrates multi-parent chains via
-- the declared wiki_type_parents edges.
SELECT DISTINCT s.id, s.slug, s.name, s.level, s.space_type_id,
       COALESCE(st.slug, '') AS space_type_slug,
       COALESCE(st.name, '') AS space_type_name,
       COALESCE(wtc.label, st.name, '') AS space_type_label,
       -- COALESCE+cast: a junction-only child has spaces.parent_id NULL →
       -- the scalar subquery is NULL → s.id = NULL is NULL, not false; the
       -- cast pins sqlc's column type to bool.
       COALESCE(s.id = (SELECT sp2.parent_id FROM spaces sp2 WHERE sp2.id = @space_id), false)::bool AS is_primary
FROM (
    SELECT parent_space_id AS pid FROM space_parent_spaces WHERE space_id = @space_id
    UNION
    SELECT parent_id FROM spaces WHERE id = @space_id AND parent_id IS NOT NULL
) links
JOIN spaces s ON s.id = links.pid
LEFT JOIN space_types st ON st.id = s.space_type_id
LEFT JOIN wiki_type_configs wtc ON wtc.space_type_id = s.space_type_id
WHERE s.deleted_at IS NULL;

-- name: ListWikiDescendantSpaces :many
-- Transitive descendants of a space (depth ≤3) whose space_type is in
-- @type_ids — the rollup row set. Declared type edges supply which types
-- are eligible; this query supplies the actual spaces via
-- space_parent_spaces ∪ parent_id (the same edges ?within= uses).
-- Postgres allows a single recursive self-reference: the seed and step
-- terms each union the two link sources (junction ∪ parent_id) inside one
-- recursive arm.
WITH RECURSIVE descendants AS (
    SELECT kid.id, 1 AS depth
    FROM (
        SELECT sps.space_id AS id FROM space_parent_spaces sps WHERE sps.parent_space_id = @space_id
        UNION
        SELECT s.id FROM spaces s WHERE s.parent_id = @space_id
    ) kid
    UNION
    SELECT kid.id, d.depth + 1
    FROM descendants d
    JOIN LATERAL (
        SELECT sps.space_id AS id FROM space_parent_spaces sps WHERE sps.parent_space_id = d.id
        UNION
        SELECT s2.id FROM spaces s2 WHERE s2.parent_id = d.id
    ) kid ON true
    WHERE d.depth < 3
)
SELECT DISTINCT ON (s.id) s.id, s.slug, s.name, s.level, s.space_type_id,
       COALESCE(st.slug, '') AS space_type_slug, d.depth
FROM descendants d
JOIN spaces s ON s.id = d.id
LEFT JOIN space_types st ON st.id = s.space_type_id
-- The root excludes itself — a cyclic junction edge must not list a space
-- as its own descendant.
WHERE s.id <> @space_id AND s.deleted_at IS NULL AND s.status = 'active'
  AND (cardinality(@type_ids::uuid[]) = 0 OR s.space_type_id = ANY(@type_ids::uuid[]))
ORDER BY s.id, d.depth;

-- name: ListWikiDirectoryChildren :many
-- Directory-mode category listing (§80): the descendant-space walk of
-- ListWikiDescendantSpaces, but the page needs a directory card per child —
-- geo_code + active wiki page_count — and every child must have an enabled
-- wiki type (a configured level that isn't a wiki type produces dead
-- links). LEFT JOIN wiki_pages so children with zero pages still list —
-- unlike rollup rows, directories don't require field values.
WITH RECURSIVE descendants AS (
    SELECT kid.id, 1 AS depth
    FROM (
        SELECT sps.space_id AS id FROM space_parent_spaces sps WHERE sps.parent_space_id = @space_id
        UNION
        SELECT s.id FROM spaces s WHERE s.parent_id = @space_id
    ) kid
    UNION
    SELECT kid.id, d.depth + 1
    FROM descendants d
    JOIN LATERAL (
        SELECT sps.space_id AS id FROM space_parent_spaces sps WHERE sps.parent_space_id = d.id
        UNION
        SELECT s2.id FROM spaces s2 WHERE s2.parent_id = d.id
    ) kid ON true
    WHERE d.depth < 3
)
SELECT s.id, s.slug, s.name, s.level, s.space_type_id,
       COALESCE(st.slug, '') AS space_type_slug,
       MIN(d.depth)::int AS depth, s.geo_code,
       s.header_image_id, s.logo_media_id,
       -- §80 directory cards: blurb prefers the authored description, then
       -- the Wikipedia summary's short description, then a truncated extract.
       COALESCE(NULLIF(s.description, ''),
                NULLIF(s.wiki_content->>'description', ''),
                LEFT(s.wiki_content->>'extract', 280),
                '')::text AS blurb,
       COUNT(wp.id) FILTER (WHERE wp.is_active = true)::bigint AS page_count
FROM descendants d
JOIN spaces s ON s.id = d.id
JOIN wiki_type_configs wtc ON wtc.space_type_id = s.space_type_id AND wtc.enabled = true
LEFT JOIN space_types st ON st.id = s.space_type_id
LEFT JOIN wiki_pages wp ON wp.space_id = s.id
-- The root excludes itself — a cyclic junction edge must not list a space
-- as its own descendant.
WHERE s.id <> @space_id AND s.deleted_at IS NULL AND s.status = 'active'
  AND (cardinality(@type_ids::uuid[]) = 0 OR s.space_type_id = ANY(@type_ids::uuid[]))
GROUP BY s.id, s.slug, s.name, s.level, s.space_type_id, st.slug, s.geo_code,
         s.header_image_id, s.logo_media_id, s.description, s.wiki_content
ORDER BY MIN(d.depth), s.name, s.slug;

-- name: WikiSpaceHasDirectoryChildren :one
-- §80 directories: existence-only variant of ListWikiDirectoryChildren —
-- true when the space has at least one active, wiki-typed descendant in
-- @type_ids. Powers `empty` on GET /wiki/categories?space= so directory
-- categories with no matching children aren't advertised as tabs/cards.
WITH RECURSIVE descendants AS (
    SELECT kid.id, 1 AS depth
    FROM (
        SELECT sps.space_id AS id FROM space_parent_spaces sps WHERE sps.parent_space_id = @space_id
        UNION
        SELECT s.id FROM spaces s WHERE s.parent_id = @space_id
    ) kid
    UNION
    SELECT kid.id, d.depth + 1
    FROM descendants d
    JOIN LATERAL (
        SELECT sps.space_id AS id FROM space_parent_spaces sps WHERE sps.parent_space_id = d.id
        UNION
        SELECT s2.id FROM spaces s2 WHERE s2.parent_id = d.id
    ) kid ON true
    WHERE d.depth < 3
)
SELECT EXISTS (
    SELECT 1
    FROM descendants d
    JOIN spaces s ON s.id = d.id
    JOIN wiki_type_configs wtc ON wtc.space_type_id = s.space_type_id AND wtc.enabled = true
    WHERE s.id <> @space_id AND s.deleted_at IS NULL AND s.status = 'active'
      AND (cardinality(@type_ids::uuid[]) = 0 OR s.space_type_id = ANY(@type_ids::uuid[]))
);

-- name: ListEntityWikiTypesForParent :many
-- Entity-bound wiki types parented to a host space type (JCE ↔ wiki
-- mapping): drives the index's entity-children section — a country's visa
-- routes/banks/embassies, a university's courses/scholarships.
SELECT st.slug AS type_slug, st.name AS type_name, wtc.icon,
       wtc.entity_kind, wtc.entity_discriminator, wtc.position
FROM wiki_type_parents p
JOIN wiki_type_configs wtc ON wtc.space_type_id = p.space_type_id AND wtc.enabled = true
JOIN space_types st ON st.id = wtc.space_type_id
WHERE p.parent_space_type_id = @parent_space_type_id
  AND wtc.entity_kind IS NOT NULL
ORDER BY wtc.position;
