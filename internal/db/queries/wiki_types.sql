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
       COALESCE(array_agg(wtp.parent_space_type_id::text) FILTER (WHERE wtp.parent_space_type_id IS NOT NULL), '{}') AS parent_type_ids
FROM wiki_type_configs wtc
JOIN space_types st ON st.id = wtc.space_type_id
LEFT JOIN wiki_type_parents wtp ON wtp.space_type_id = wtc.space_type_id
GROUP BY wtc.space_type_id, st.slug, st.name,
         wtc.enabled, wtc.label, wtc.icon, wtc.seed_stubs, wtc.position, wtc.created_at
ORDER BY wtc.position, st.name;

-- name: ListEnabledWikiTypes :many
-- Public list — enabled types only, for navigation/destination filters.
SELECT wtc.space_type_id, st.slug AS space_type_slug, st.name AS space_type_name,
       COALESCE(wtc.label, st.name) AS label, wtc.icon, wtc.position
FROM wiki_type_configs wtc
JOIN space_types st ON st.id = wtc.space_type_id
WHERE wtc.enabled = true
ORDER BY wtc.position, st.name;

-- name: GetWikiTypeConfig :one
SELECT wtc.space_type_id, wtc.enabled, wtc.label, wtc.icon, wtc.seed_stubs,
       wtc.position, wtc.created_at
FROM wiki_type_configs wtc
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
       wtc.position, wtc.is_system, wtc.created_at,
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
       wtc.position, wtc.is_system, wtc.created_at,
       cfg.slug AS field_group_slug, cfg.name AS field_group_name
FROM wiki_type_categories wtc
JOIN spaces sp ON sp.space_type_id = wtc.space_type_id
LEFT JOIN custom_field_groups cfg ON cfg.id = wtc.field_group_id
WHERE sp.id = @space_id AND (wtc.slug = @slug OR wtc.category = @slug)
LIMIT 1;

-- name: ListWikiCategoriesBySpaceSlug :many
-- Public category list for a space (replaces the hardcoded geo/university
-- slices): resolves the space's type template.
SELECT wtc.slug, wtc.category, wtc.label, wtc.field_group_id, wtc.position, wtc.is_system,
       st.slug AS space_type_slug
FROM wiki_type_categories wtc
JOIN spaces sp ON sp.space_type_id = wtc.space_type_id
JOIN space_types st ON st.id = wtc.space_type_id
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
INSERT INTO wiki_type_categories (space_type_id, slug, category, label, field_group_id, position, is_system)
VALUES (@space_type_id, @slug, @category, @label, sqlc.narg(field_group_id), @position, @is_system)
RETURNING id, space_type_id, slug, category, label, field_group_id, position, is_system, created_at;

-- name: UpdateWikiTypeCategory :execrows
-- Slug is immutable (page URLs are /wiki/{entity}/{slug}); label, bound
-- group, and position are editable.
UPDATE wiki_type_categories
SET label = COALESCE(sqlc.narg(label), label),
    field_group_id = CASE WHEN @update_field_group::bool
                      THEN sqlc.narg(field_group_id)
                      ELSE field_group_id END,
    position = COALESCE(sqlc.narg(position), position)
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
        AND wr.kind IN ('content', 'mixed')
  );

-- name: GetWikiTypeCategory :one
SELECT id, space_type_id, slug, category, label, field_group_id,
       position, is_system, created_at
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
SELECT wtc.slug, wtc.category, wtc.label, wtc.field_group_id, wtc.position, wtc.is_system,
       st.slug AS space_type_slug
FROM wiki_type_categories wtc
JOIN wiki_type_configs cfg ON cfg.space_type_id = wtc.space_type_id AND cfg.enabled = true
JOIN space_types st ON st.id = wtc.space_type_id
ORDER BY wtc.position, wtc.slug;
