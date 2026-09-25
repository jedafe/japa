-- 000218_university_submission_tracking.up.sql
-- Adds marketplace submission tracking columns to learning_tracks so the
-- creator dashboard and admin reject flow have an explicit submission_status
-- lifecycle (draft → pending → approved/rejected) instead of inferring it
-- from is_published / is_published_to_university booleans.
-- Also seeds the university.payout.view_all RBAC node for read-only admin
-- payout endpoints (BUG-014).
-- Ref: ADR-063 §5, §9; conventions/permission-key-naming.md (Japa University)

BEGIN;

-- 1. Submission tracking columns on learning_tracks.
ALTER TABLE learning_tracks
    ADD COLUMN IF NOT EXISTS university_submission_status text NOT NULL DEFAULT 'draft'
        CHECK (university_submission_status IN ('draft','pending','approved','rejected')),
    ADD COLUMN IF NOT EXISTS university_submitted_at  timestamptz,
    ADD COLUMN IF NOT EXISTS university_reviewed_at   timestamptz,
    ADD COLUMN IF NOT EXISTS university_rejection_reason text,
    ADD COLUMN IF NOT EXISTS university_featured boolean NOT NULL DEFAULT false;

-- Backfill: any track already published to the university is 'approved'.
UPDATE learning_tracks
SET university_submission_status = 'approved',
    university_reviewed_at = updated_at
WHERE is_published_to_university = true AND university_submission_status = 'draft';

CREATE INDEX IF NOT EXISTS idx_learning_tracks_submission_status
    ON learning_tracks(tenant_id, instructor_id, university_submission_status)
    WHERE instructor_id IS NOT NULL;

COMMENT ON COLUMN learning_tracks.university_submission_status IS
    'Marketplace submission lifecycle: draft|pending|approved|rejected (ADR-063 §5).';
COMMENT ON COLUMN learning_tracks.university_rejection_reason IS
    'Admin rejection reason; cleared on re-submit (ADR-063 §5).';

-- 2. Seed university.payout.view_all RBAC node (BUG-014).
-- Read-only admin payout list/get endpoints should not require the write
-- university.payout.process node.
INSERT INTO permissions (key, description) VALUES
    ('university.payout.view_all', 'View all instructor payouts (admin read-only)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin_role'
  AND p.key = 'university.payout.view_all'
ON CONFLICT DO NOTHING;

COMMIT;
