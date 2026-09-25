-- 000480_pagebuilder_page_type_and_template_columns.up.sql
-- BUG-001: The pages.page_type CHECK constraint (added in 000217, extended
-- in 000268) only allows ('page', 'sidebar', 'landing'). Section 16 and the
-- admin UI i18n keys (pageTemplates.categoryAbout/Service/Event/Custom)
-- reference 'about', 'service', 'event', 'custom' page types. Creating a
-- page with one of those types currently fails with a constraint violation.
-- This migration extends the CHECK to include all spec-defined page types.
--
-- BUG-002: The pagebuilder_templates table (000055) is missing four columns
-- that Section 16 specifies for page_templates: is_active (distinct from
-- is_system — a system template can be inactive, and a non-system template
-- can be active), thumbnail_media_id (preview image for the template
-- picker), created_by (audit trail), and schema_version (block schema
-- versioning for forward-compatible migrations). This migration adds them.
--
-- Ref: japaconnect-engineering/sections/16-page-builder.md
-- Pattern: 000268_pages_page_type_landing.up.sql

BEGIN;

-- BUG-001: extend page_type CHECK to include all spec-defined types.
ALTER TABLE pages DROP CONSTRAINT IF EXISTS pages_page_type_check;
ALTER TABLE pages ADD CONSTRAINT pages_page_type_check
    CHECK (page_type IN ('page', 'sidebar', 'landing', 'about', 'service', 'event', 'custom'));

-- BUG-002: add missing spec columns to pagebuilder_templates.
ALTER TABLE pagebuilder_templates
    ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS thumbnail_media_id UUID REFERENCES media(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS schema_version INT NOT NULL DEFAULT 1;

-- Index the new is_active column for template-list filtering.
CREATE INDEX IF NOT EXISTS idx_pagebuilder_templates_active
    ON pagebuilder_templates(tenant_id, is_active);

COMMIT;
