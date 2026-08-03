-- 000252_jobs_board.up.sql
-- Jobs Board (Section 63, ADR-081).
-- Creates the job_applications table for on-platform application tracking,
-- and seeds jobs.* system_settings + RBAC nodes.
-- Job listings themselves are contents rows with content_type='job_listing'
-- and job-specific data in custom_field_values — no new jobs table.
--
-- Ref: japaconnect-engineering/sections/63-jobs-board.md
--      japaconnect-engineering/adr/ADR-081-jobs-board.md

BEGIN;

-- =====================================================================
-- job_applications — on-platform application tracking with status flow.
-- Links to form_submissions for the structured application form data.
-- =====================================================================
CREATE TABLE IF NOT EXISTS job_applications (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    job_id            uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    applicant_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    submission_id     uuid REFERENCES form_submissions(id) ON DELETE SET NULL,
    status            text NOT NULL DEFAULT 'applied'
                        CHECK (status IN ('applied','reviewing','shortlisted','rejected','offered','withdrawn')),
    employer_notes    text,
    applied_at        timestamptz NOT NULL DEFAULT now(),
    status_updated_at timestamptz,
    UNIQUE(tenant_id, job_id, applicant_id)
);

CREATE INDEX IF NOT EXISTS idx_job_apps_job
    ON job_applications (job_id, status);

CREATE INDEX IF NOT EXISTS idx_job_apps_applicant
    ON job_applications (applicant_id, tenant_id);

CREATE INDEX IF NOT EXISTS idx_job_apps_tenant_status
    ON job_applications (tenant_id, status);

-- =====================================================================
-- system_settings — 2 jobs.* keys (Section 63, ADR-081)
-- =====================================================================
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'jobs.enabled',         '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'jobs.featured_credits', '"1000"')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- =====================================================================
-- RBAC permission nodes — 4 jobs.* nodes (Section 63, ADR-081)
-- =====================================================================
INSERT INTO permissions (key, description) VALUES
    ('jobs.browse', 'Browse job listings (all users incl. anonymous)'),
    ('jobs.apply',  'Apply to job listings (authenticated users)'),
    ('jobs.post',   'Post a job listing (org admins)'),
    ('jobs.manage', 'Admin: manage all listings and applications')
ON CONFLICT (key) DO NOTHING;

-- Grant jobs.manage to super_admin.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key = 'jobs.manage'
ON CONFLICT DO NOTHING;

-- Grant jobs.browse + jobs.apply + jobs.post to the default 'member' role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key IN ('jobs.browse', 'jobs.apply', 'jobs.post')
ON CONFLICT DO NOTHING;

COMMIT;
