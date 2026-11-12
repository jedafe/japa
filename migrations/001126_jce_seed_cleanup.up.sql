-- JCE/Wiki seed cleanup pass:
--  1. Remove test-fixture wiki categories ('t-cr-*','t-upd-*','t-sys-*',
--     labels "Created"/"After"/"System Cat") left on the country space type
--     by test runs — they leak into real country wiki pages and pool lists.
--  2. Deprecate the legacy `visa-types` comparison type — a duplicate of
--     `visa-routes` (identical bound group + dimension pool + item refs).
--     can_fill stays true so existing comparisons keep rendering.

BEGIN;

DELETE FROM wiki_revisions
WHERE wiki_page_id IN (
    SELECT id FROM wiki_pages WHERE field_group_id IN (
        SELECT id FROM custom_field_groups
        WHERE slug IN ('wiki_country_t_cr_b4e69ff3', 't-grp-3f7975c7', 't-grp-d0ff1696')
    )
);

DELETE FROM wiki_pages
WHERE field_group_id IN (
    SELECT id FROM custom_field_groups
    WHERE slug IN ('wiki_country_t_cr_b4e69ff3', 't-grp-3f7975c7', 't-grp-d0ff1696')
);

DELETE FROM wiki_type_categories
WHERE slug IN ('t-cr-b4e69ff3', 't-upd-b3485100', 't-sys-12531364');

DELETE FROM custom_field_groups
WHERE slug IN ('wiki_country_t_cr_b4e69ff3', 't-grp-3f7975c7', 't-grp-d0ff1696');

INSERT INTO comparison_type_access (type_id, user_group_id, can_create, can_fill, ai_can_fill)
SELECT td.id, NULL, false, true, false
FROM comparison_type_definitions td
WHERE td.slug = 'visa-types'
  AND NOT EXISTS (
      SELECT 1 FROM comparison_type_access ta WHERE ta.type_id = td.id
  );

COMMIT;
