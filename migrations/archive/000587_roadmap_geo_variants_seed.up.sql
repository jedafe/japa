-- 000587_roadmap_geo_variants_seed.up.sql
-- Create placeholder roadmap guide variants for the most common origin →
-- destination relocation pairs. Each variant is linked to a published base guide
-- (identified by field_values->>'dimension') from any guide category and tagged
-- with the same dimension, guide_category='roadmap', source_country and
-- target_country. Status is 'draft' so the editorial team can review and fill
-- the body before publishing.
--
-- Ref: .jeos/adr/ADR-071-japa-readiness-assessment.md
--      .jeos/sections/71-readiness-assessment.md

BEGIN;

-- Tag published base guides from non-roadmap categories with a fallback
-- readiness dimension so they can be used as sources for roadmap variants.
UPDATE contents
SET field_values = COALESCE(field_values, '{}'::jsonb) ||
                  jsonb_build_object(
                      'dimension',
                      CASE guide_category
                          WHEN 'work_career' THEN 'professional'
                          WHEN 'scholarship' THEN 'financial'
                          WHEN 'relocation'  THEN 'logistical'
                          WHEN 'banking'     THEN 'financial'
                      END
                  )
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND deleted_at IS NULL
  AND auto_hidden = false
  AND status = 'published'
  AND visibility IN ('public', 'members')
  AND content_type = 'guide'
  AND source_country IS NULL
  AND target_country IS NULL
  AND field_values->>'dimension' IS NULL
  AND guide_category IN ('work_career', 'scholarship', 'relocation', 'banking');

WITH origins AS (
    SELECT unnest(ARRAY['NG','GH','KE']) AS code
),
destinations AS (
    SELECT unnest(ARRAY['CA','US','GB','IE','DE','FI']) AS code
),
base_guides AS (
    SELECT
        c.id,
        c.slug,
        c.title,
        c.body,
        c.field_values->>'dimension' AS dimension
    FROM contents c
    WHERE c.tenant_id = '00000000-0000-0000-0000-000000000001'
      AND c.deleted_at IS NULL
      AND c.auto_hidden = false
      AND c.status = 'published'
      AND c.visibility IN ('public', 'members')
      AND c.field_values->>'dimension' IS NOT NULL
      AND c.source_country IS NULL
      AND c.target_country IS NULL
),
admin_user AS (
    SELECT u.id AS user_id
    FROM users u
    JOIN user_group_memberships ugm ON u.id = ugm.user_id
    JOIN user_groups ug ON ugm.group_id = ug.id
    JOIN group_roles gr ON gr.group_id = ug.id
    JOIN user_roles ur ON gr.role_id = ur.id
    WHERE u.tenant_id = '00000000-0000-0000-0000-000000000001'
      AND ug.slug = 'super_admin'
      AND ur.slug = 'super_admin'
    LIMIT 1
)
INSERT INTO contents (
    tenant_id,
    author_id,
    content_type,
    title,
    slug,
    body,
    body_html,
    field_values,
    content_hash,
    status,
    visibility,
    guide_category,
    target_country,
    source_country,
    published_at,
    created_at,
    updated_at
)
SELECT
    '00000000-0000-0000-0000-000000000001',
    au.user_id,
    'guide',
    format('%s — %s to %s', bg.title, o.code, d.code),
    format('%s-%s-%s', bg.slug, lower(d.code), lower(o.code)),
    format('Placeholder guide for %s → %s. %s Replace with accurate, up-to-date advice before publishing.',
           o.code, d.code, bg.body),
    '',
    jsonb_build_object('placeholder_variant', 'true', 'dimension', bg.dimension),
    encode(digest(lower(regexp_replace(
        format('Placeholder guide for %s → %s. %s Replace with accurate, up-to-date advice before publishing.',
               o.code, d.code, bg.body),
        '\s+', ' ', 'g'
    ))::text, 'sha256'), 'hex'),
    'draft',
    'public',
    'roadmap',
    d.code,
    o.code,
    NULL,
    now(),
    now()
FROM base_guides bg
CROSS JOIN origins o
CROSS JOIN destinations d
CROSS JOIN admin_user au
WHERE au.user_id IS NOT NULL
ON CONFLICT DO NOTHING;

COMMIT;
