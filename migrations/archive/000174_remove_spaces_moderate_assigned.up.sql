-- 000174_remove_spaces_moderate_assigned.up.sql
-- AUDIT-FRESH-NAME-001: spaces.moderate.assigned (seeded 000099, 000109) and
-- spaces.moderator.assign (seeded 000109, 000125) were both used for the same
-- moderator-assign operation. Consolidated on spaces.moderator.assign.
-- This migration removes the duplicate spaces.moderate.assigned node and any
-- role_permissions grants referencing it. Code now uses spaces.moderator.assign
-- exclusively (server.go, rbac.go, rbac/middleware.go).

DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key = 'spaces.moderate.assigned'
);
DELETE FROM permissions WHERE key = 'spaces.moderate.assigned';
