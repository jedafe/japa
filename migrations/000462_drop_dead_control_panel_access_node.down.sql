-- 000462_drop_dead_control_panel_access_node.down.sql
-- Reverses the up migration: re-creates the dead control_panel.access node.
-- This is only needed for migration reversibility; the node is not enforced
-- by any backend route and will not be granted to any role.

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('control_panel.access', 'DEAD NODE — replaced by control.panel.access; do not use')
ON CONFLICT (key) DO NOTHING;

COMMIT;
