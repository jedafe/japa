-- Reverses 000799_wiki_attraction_field_permissions. The seed cannot be
-- perfectly inverted (pre-existing rows were left untouched by ON
-- CONFLICT), so the down migration removes only rows matching the seeded
-- patterns — the same convention as 000743's down migration.

BEGIN;

-- Seeded can_edit grants on attraction group fields (non-guest groups).
DELETE FROM custom_field_permissions cfp
USING custom_fields cf, custom_field_groups g, user_groups ug
WHERE cfp.field_id = cf.id
  AND cf.group_id = g.id
  AND cfp.user_group_id = ug.id
  AND g.slug IN ('wiki_attraction_overview', 'wiki_attraction_plan_your_visit')
  AND ug.slug <> 'guest'
  AND cfp.can_edit = true;

-- Seeded guest view-only grants on attraction group fields.
DELETE FROM custom_field_permissions cfp
USING custom_fields cf, custom_field_groups g, user_groups ug
WHERE cfp.field_id = cf.id
  AND cf.group_id = g.id
  AND cfp.user_group_id = ug.id
  AND g.slug IN ('wiki_attraction_overview', 'wiki_attraction_plan_your_visit')
  AND ug.slug = 'guest'
  AND cfp.can_view = true
  AND cfp.can_edit = false;

COMMIT;
