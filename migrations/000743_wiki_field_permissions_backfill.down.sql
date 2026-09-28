-- Reverses 000743_wiki_field_permissions_backfill. The seed cannot be
-- perfectly inverted (pre-existing rows were left untouched by ON
-- CONFLICT), so the down migration removes only rows that match the seeded
-- patterns — the same convention as 000733's down migration.

BEGIN;

-- Seeded can_edit grants on entity-assigned group fields (non-guest groups).
DELETE FROM custom_field_permissions cfp
USING custom_fields cf, user_groups ug
WHERE cfp.field_id = cf.id
  AND cfp.user_group_id = ug.id
  AND ug.slug <> 'guest'
  AND cfp.can_edit = true
  AND EXISTS (
      SELECT 1 FROM custom_field_group_assignments a
      WHERE a.group_id = cf.group_id
  );

-- Seeded guest view-only grants on entity-assigned group fields.
DELETE FROM custom_field_permissions cfp
USING custom_fields cf, user_groups ug
WHERE cfp.field_id = cf.id
  AND cfp.user_group_id = ug.id
  AND ug.slug = 'guest'
  AND cfp.can_view = true
  AND cfp.can_edit = false
  AND EXISTS (
      SELECT 1 FROM custom_field_group_assignments a
      WHERE a.group_id = cf.group_id
  );

-- Seeded view-only rows on non-system fields.
DELETE FROM custom_field_permissions cfp
USING custom_fields cf
WHERE cfp.field_id = cf.id
  AND cf.is_system = false
  AND cfp.can_view = true
  AND cfp.can_edit = false;

COMMIT;
