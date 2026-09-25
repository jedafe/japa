-- 000425_drop_dead_university_rbac_nodes.up.sql
-- university audit BUG-008/BUG-009: the permission nodes
-- `university.review.moderate` and `university.enrollment.view_own` were
-- seeded in 000168 but are never referenced by any RequirePermission call
-- in the server.
--
--   - university.review.moderate: course review moderation is handled by
--     the generic /admin/reviews/{id} routes gated on `reviews.moderate`
--     (Section 29 reuse per ADR-063 §2). The university-specific node is
--     redundant and dead.
--   - university.enrollment.view_own: the /me/enrollments route was
--     intentionally changed to RequireAuth-only (server.go line 3940-3943)
--     because the gate caused 403 for regular Learn users breaking the
--     track-detail enrollment state fetch. The node remains in the
--     permissions table with no route.
--
-- This migration drops the role_permissions grants and the permission rows
-- themselves. Idempotent: safe to re-run.
-- Ref: japaconnect-engineering/conventions/permission-key-naming.md (Japa University)

BEGIN;

-- 1. Drop role_permissions grants referencing the two dead nodes.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN ('university.review.moderate', 'university.enrollment.view_own')
);

-- 2. Drop the dead permission rows.
DELETE FROM permissions
WHERE key IN ('university.review.moderate', 'university.enrollment.view_own');

COMMIT;
