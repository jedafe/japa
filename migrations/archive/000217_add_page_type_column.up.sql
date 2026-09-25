-- 000217 — Add page_type column to distinguish regular pages from sidebar pages.
--
-- Sidebar pages are stored as rows in the pages table with page_type='sidebar'
-- and reserved slugs (_sidebar_left, _sidebar_right). This reuses the existing
-- blocks JSONB column to store ordered sidebar widgets, avoiding a new table.
--
-- Per-page sidebar visibility is controlled via the meta JSONB field:
--   {"show_left_sidebar": true, "show_right_sidebar": false}

ALTER TABLE pages ADD COLUMN IF NOT EXISTS page_type TEXT NOT NULL DEFAULT 'page';

-- Allow 'page' and 'sidebar' page types. Future types (e.g. 'header', 'footer')
-- can be added here.
ALTER TABLE pages DROP CONSTRAINT IF EXISTS pages_page_type_check;
ALTER TABLE pages ADD CONSTRAINT pages_page_type_check CHECK (page_type IN ('page', 'sidebar'));

-- Index for filtering sidebar pages efficiently.
CREATE INDEX IF NOT EXISTS idx_pages_page_type ON pages(tenant_id, page_type);

-- Seed default sidebar pages for all existing tenants.
-- These are empty (no blocks) and published so the public endpoint can serve them.
INSERT INTO pages (tenant_id, title, slug, blocks, meta, status, page_type)
SELECT t.id, 'Left Sidebar', '_sidebar_left', '[]'::jsonb,
       '{"show_left_sidebar": true, "show_right_sidebar": true}'::jsonb,
       'published', 'sidebar'
FROM tenants t
WHERE NOT EXISTS (
    SELECT 1 FROM pages p
    WHERE p.tenant_id = t.id AND p.slug = '_sidebar_left' AND p.page_type = 'sidebar'
)
ON CONFLICT DO NOTHING;

INSERT INTO pages (tenant_id, title, slug, blocks, meta, status, page_type)
SELECT t.id, 'Right Sidebar', '_sidebar_right', '[]'::jsonb,
       '{"show_left_sidebar": true, "show_right_sidebar": true}'::jsonb,
       'published', 'sidebar'
FROM tenants t
WHERE NOT EXISTS (
    SELECT 1 FROM pages p
    WHERE p.tenant_id = t.id AND p.slug = '_sidebar_right' AND p.page_type = 'sidebar'
)
ON CONFLICT DO NOTHING;
