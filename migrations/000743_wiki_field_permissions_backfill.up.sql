-- 000743_wiki_field_permissions_backfill.up.sql
-- Backfills custom_field_permissions rows that the 000733 bootstrap skipped:
-- its assigned-group seed carried `WHERE cf.is_system = false`, but the
-- seeded wiki/university field groups are is_system = true, so the actual
-- community-editable surface received no permission rows and every field
-- failed closed (invisible and uneditable) for non-manage users.
--
-- Corrected semantics (§8.1a, matching the 000733 header comment): a field
-- in a group assigned to an entity class is the community-editable surface —
-- assignment is the opt-in — so it gets can_view for every user group
-- (guests read the public Details panel) and can_edit for every non-guest
-- user group regardless of is_system. All other non-system fields stay
-- view-only for every group. ON CONFLICT DO NOTHING preserves explicit
-- admin grants/denials and makes this idempotent.
--
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md
-- Ref: japaconnect-engineering/sections/06-custom-fields.md

BEGIN;

-- Assigned-group fields: can_view for every group (guests read the public
-- Details panel), can_edit for non-guest groups.
INSERT INTO custom_field_permissions (field_id, user_group_id, can_view, can_edit)
SELECT DISTINCT cf.id, ug.id, true, ug.slug <> 'guest'
FROM custom_fields cf
JOIN custom_field_group_assignments a ON a.group_id = cf.group_id
CROSS JOIN user_groups ug
ON CONFLICT (field_id, user_group_id) DO NOTHING;

-- All other non-system fields: view-only for every group (covers
-- community fields approved before runtime permission seeding existed).
INSERT INTO custom_field_permissions (field_id, user_group_id, can_view, can_edit)
SELECT cf.id, ug.id, true, false
FROM custom_fields cf
CROSS JOIN user_groups ug
WHERE cf.is_system = false
ON CONFLICT (field_id, user_group_id) DO NOTHING;

COMMIT;
