-- 000911_wiki_rollup_pages_dedupe.up.sql
-- Review fix: 000901 appended the new COL subpage slugs onto the city
-- cost-of-living category's rollup->'pages' array that already listed
-- them — the duplicates surface as keyed-each crashes in WikiRollupPanel
-- (two rows sharing row.page.slug). Rewrite every rollup->'pages' array
-- de-duplicated, preserving first-seen order.

BEGIN;

UPDATE wiki_type_categories c
SET rollup = jsonb_set(c.rollup, '{pages}', dedup.pages)
FROM (
    SELECT c.id,
           (SELECT jsonb_agg(x ORDER BY first_idx)
            FROM (
                SELECT x, min(idx) AS first_idx
                FROM jsonb_array_elements_text(c.rollup->'pages')
                     WITH ORDINALITY AS t(x, idx)
                GROUP BY x
            ) d) AS pages
    FROM wiki_type_categories c
    WHERE c.rollup ? 'pages'
      AND c.rollup->'pages' IS NOT NULL
) dedup
WHERE c.id = dedup.id
  AND dedup.pages IS NOT NULL;

COMMIT;
