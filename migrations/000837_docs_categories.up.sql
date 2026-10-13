-- 000837_docs_categories
-- Seed the managed docs-portal category catalog (docs.categories) from the
-- categories/subcategories already in use on doc pages, so admins start
-- from the live taxonomy in Settings → Documentation rather than a blank
-- editor. Absent key = derived-only behavior, so the insert is skipped
-- when no categories exist or the key is already set.

BEGIN;

INSERT INTO system_settings (key, value, is_public)
SELECT 'docs.categories', catalog.v, true
FROM (
    SELECT jsonb_agg(row ORDER BY row ->> 'name') AS v
    FROM (
        SELECT jsonb_build_object(
            'name', c.category,
            'subcategories', COALESCE(
                (
                    SELECT jsonb_agg(s ORDER BY s)
                    FROM (
                        SELECT DISTINCT p2.subcategory AS s
                        FROM doc_pages p2
                        WHERE p2.category = c.category
                          AND p2.subcategory IS NOT NULL
                          AND p2.subcategory <> ''
                    ) subs
                ),
                '[]'::jsonb
            )
        ) AS row
        FROM (
            SELECT DISTINCT category
            FROM doc_pages
            WHERE category IS NOT NULL AND category <> ''
        ) c
    ) rows
) catalog
WHERE catalog.v IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM system_settings WHERE system_settings.key = 'docs.categories');

COMMIT;
