-- Seed the three system custom fields for the tag_fields group (Section 06).
-- The tag_fields group (00000000-0000-0000-0000-000000000046) was created in
-- migration 000008 and assigned to entity_class 'tag' in migration 000389,
-- but no actual fields were seeded. This migration adds the three spec-required
-- fields: Wiki Content, Related Tags, External Link.
-- Ref: japaconnect-engineering/sections/06-custom-field-builder-form-builder.md

INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, is_system, is_required, show_in_frontend, position)
VALUES
    ('00000000-0000-0000-0000-000000000053',
     '00000000-0000-0000-0000-000000000046',
     '00000000-0000-0000-0000-000000000001',
     'Wiki Content',
     'wiki_content',
     'rich_text',
     true, false, true, 0),
    ('00000000-0000-0000-0000-000000000054',
     '00000000-0000-0000-0000-000000000046',
     '00000000-0000-0000-0000-000000000001',
     'Related Tags',
     'related_tags',
     'text',
     true, false, true, 1),
    ('00000000-0000-0000-0000-000000000055',
     '00000000-0000-0000-0000-000000000046',
     '00000000-0000-0000-0000-000000000001',
     'External Link',
     'external_link',
     'url',
     true, false, true, 2)
ON CONFLICT (id) DO NOTHING;

-- Mark content.max_tags as a public setting so the main-app composer can read
-- it from GET /site/settings/public instead of hardcoding the limit (Rule 3/16).
-- The catalog entry in settings_catalog.go is updated in the same change; this
-- backfills existing DB rows so the is_public flag takes effect immediately.
UPDATE system_settings
SET is_public = true
WHERE key = 'content.max_tags';
