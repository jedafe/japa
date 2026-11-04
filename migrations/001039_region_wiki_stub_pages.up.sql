-- 001039_region_wiki_stub_pages.up.sql
-- §wiki-seo — the stub-seed fan-out ran before 001038 added the region
-- categories, so materialize the stub pages the seeder would have made
-- (shape mirrors its rows: space_id + slug + title + category, rest
-- defaults). Titles follow the '{name} X' label convention.
BEGIN;

INSERT INTO wiki_pages (space_id, slug, title, category)
SELECT s.id, c.slug,
       REPLACE(c.label, '{name}', s.name),
       c.category
FROM spaces s
JOIN wiki_type_categories c
  ON c.space_type_id = '00000000-0000-0000-0000-00000000007f'
WHERE s.space_type_id = '00000000-0000-0000-0000-00000000007f'
  AND s.level = 'region'
  AND s.deleted_at IS NULL
  AND c.slug NOT IN ('overview', 'countries') -- seeded by the fan-out already
ON CONFLICT DO NOTHING;

COMMIT;
