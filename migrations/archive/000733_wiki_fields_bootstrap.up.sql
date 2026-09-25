-- 000733_wiki_fields_bootstrap
-- §80 community field ops follow-ups:
--   1. wiki_revisions.apply_error records the last ops-application failure
--      on a pending revision so reviewers can see why an approve attempt
--      failed instead of the revision silently staying pending.
--   2. Seed custom_field_permissions so the community field surface is not
--      inert: fields in groups assigned to an entity class are the
--      community-editable surface (assignment is the opt-in) and get
--      can_view+can_edit for every non-guest user group; all other
--      non-system fields are viewable but not editable. Field-level grants
--      never bypass the wiki.fields.* RBAC nodes checked at submit.
--      Existing rows are never overwritten (ON CONFLICT DO NOTHING) — an
--      explicit admin denial survives the seed.

ALTER TABLE wiki_revisions ADD COLUMN apply_error text;

INSERT INTO custom_field_permissions (field_id, user_group_id, can_view, can_edit)
SELECT DISTINCT cf.id, ug.id, true, true
FROM custom_fields cf
JOIN custom_field_group_assignments a ON a.group_id = cf.group_id
JOIN user_groups ug ON ug.slug <> 'guest'
WHERE cf.is_system = false
ON CONFLICT (field_id, user_group_id) DO NOTHING;

INSERT INTO custom_field_permissions (field_id, user_group_id, can_view, can_edit)
SELECT cf.id, ug.id, true, false
FROM custom_fields cf
CROSS JOIN user_groups ug
WHERE cf.is_system = false
ON CONFLICT (field_id, user_group_id) DO NOTHING;
