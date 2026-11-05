-- 001045_bank_entity_split.down.sql
-- Restores the moved rows as organizations and reverts the wiki binding.
-- Org-scoped dependents deleted on the way up (follows, claims, org DMs,
-- tips, bookings) are seed artifacts and are not restored.
BEGIN;

-- Rebind the 'bank' wiki type to the generic organization kind.
UPDATE wiki_type_configs
SET entity_kind = 'organization', entity_discriminator = '{"org_type":"bank"}'::jsonb
WHERE space_type_id = '00000000-0000-0000-0000-00000000007c';

DELETE FROM wiki_entity_kinds WHERE entity_kind = 'bank';
DELETE FROM custom_field_entity_types WHERE value = 'bank';

-- Recreate the org rows (ownerless and unverified — the original seed's
-- verified=true + admin ownership was the defect this migration fixes).
INSERT INTO organizations (id, name, slug, description, website_url, country_code, geo_space_id, org_type, verified, status, field_values, created_at, updated_at)
SELECT id, name, slug, description, website, country_code, geo_space_id, 'bank', false,
       CASE WHEN is_active THEN 'active' ELSE 'suspended' END, field_values, created_at, updated_at
FROM banks;

-- Retag bound wiki data back to 'organization'.
DELETE FROM custom_field_values a
USING custom_field_values b
WHERE a.entity_type = 'bank'
  AND b.entity_type = 'organization'
  AND a.field_id = b.field_id
  AND a.entity_id = b.entity_id
  AND a.entity_id IN (SELECT id FROM banks);

UPDATE custom_field_values SET entity_type = 'organization'
WHERE entity_type = 'bank' AND entity_id IN (SELECT id FROM banks);

UPDATE wiki_pages SET entity_type = 'organization'
WHERE entity_type = 'bank' AND entity_id IN (SELECT id FROM banks);

UPDATE wiki_revisions SET target_entity_type = 'organization'
WHERE target_entity_type = 'bank' AND target_entity_id IN (SELECT id FROM banks);

DROP TABLE banks;

COMMIT;
