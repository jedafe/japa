-- 000799_wiki_attraction_field_permissions.up.sql
-- Backfills custom_field_permissions for the attraction wiki field groups
-- seeded by 000795. That migration created the groups, assignments, and
-- fields but no permission rows — and the 000743 backfill had already run,
-- so every attraction field fails closed (invisible and uneditable) for
-- non-wiki.fields.manage users. Without this grant the public Details
-- panel, hero gallery strip, and suggest-edit flow never render field
-- values on attraction wiki pages.
--
-- Same semantics as 000743: fields in a group assigned to an entity class
-- are the community-editable surface — can_view for every user group
-- (guests read the public panel) and can_edit for every non-guest group.
-- ON CONFLICT DO NOTHING preserves explicit admin grants/denials.

BEGIN;

INSERT INTO custom_field_permissions (field_id, user_group_id, can_view, can_edit)
SELECT cf.id, ug.id, true, ug.slug <> 'guest'
FROM custom_fields cf
JOIN custom_field_groups g ON g.id = cf.group_id
CROSS JOIN user_groups ug
WHERE g.slug IN ('wiki_attraction_overview', 'wiki_attraction_plan_your_visit')
ON CONFLICT (field_id, user_group_id) DO NOTHING;

COMMIT;
