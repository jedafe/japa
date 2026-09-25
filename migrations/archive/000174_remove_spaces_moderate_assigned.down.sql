-- 000174_remove_spaces_moderate_assigned.down.sql
-- Reverts 000174: re-seeds the spaces.moderate.assigned permission node.
-- Note: role_permissions grants are not restored; re-assign them manually if needed.

INSERT INTO permissions (key, description)
VALUES ('spaces.moderate.assigned', 'Spaces Moderate Assigned')
ON CONFLICT (key) DO NOTHING;
