-- 001239_mentor_applications.up.sql
-- §102 Mentorship Matching amendment (2026-11-28): member mentor
-- applications. Members without mentorship.become_mentor submit a
-- mentor_applications row; admin approval adds them to the new `mentors`
-- system group (backing role `mentor-role` carrying exactly
-- mentorship.become_mentor + mentorship.respond). Revocation removes the
-- membership — rbac.Store.RemoveUserFromGroup fires the existing
-- RetireMentorWithoutBecomeMentorPerm sweep automatically.
--
-- Also seeds the mentorship.apply node (member+ = every role holding
-- mentorship.request) and the two notification types used by the
-- mentorship:notify_application_decision Asynq task.
--
-- Ref: .jeos/sections/102-mentorship-matching.md (2026-11-28 amendment)
--      .jeos/sections/03-rbac.md

BEGIN;

-- 1) Application table. user_id UNIQUE — one live application per member;
--    resubmission after a decline resets the same row to pending.
CREATE TABLE mentor_applications (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE UNIQUE,
    status           TEXT NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending','approved','declined')),
    motivation       TEXT NOT NULL,
    mentor_areas     TEXT[] NOT NULL DEFAULT '{}',
    reviewer_user_id UUID REFERENCES users(id),
    reviewed_at      TIMESTAMPTZ,
    decline_reason   TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_mentor_applications_status
    ON mentor_applications (status, created_at DESC);
-- user_id needs no extra index — UNIQUE above already covers lookups.

-- 2) mentors system group + backing role (001049 convention: the backing
--    role exists so the group renders correctly in the RBAC admin UI).
INSERT INTO user_groups (id, name, slug, is_system, max_spaces_creatable)
VALUES (gen_random_uuid(), 'Mentors', 'mentors', true, -1)
ON CONFLICT (slug) DO UPDATE SET
    is_system = EXCLUDED.is_system;

INSERT INTO user_roles (id, name, slug, created_at, updated_at)
VALUES (gen_random_uuid(), 'Mentor Role', 'mentor-role', now(), now())
ON CONFLICT (slug) DO NOTHING;

INSERT INTO group_roles (group_id, role_id)
SELECT g.id, r.id
FROM user_groups g
JOIN user_roles r ON r.slug = 'mentor-role'
WHERE g.slug = 'mentors'
ON CONFLICT DO NOTHING;

-- The mentors grant is exactly the two nodes a working mentor needs:
-- opt-in surface + request inbox. Nothing else.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r, permissions p
WHERE r.slug = 'mentor-role'
  AND p.key IN ('mentorship.become_mentor', 'mentorship.respond')
ON CONFLICT DO NOTHING;

-- 3) mentorship.apply — member+ semantics: grant it to every role that
--    already carries mentorship.request so the apply surface mirrors the
--    request surface wherever it was deployed.
INSERT INTO permissions (key, description) VALUES
    ('mentorship.apply', 'Apply to become a mentor')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT DISTINCT rp.role_id, p.id
FROM role_permissions rp
JOIN permissions src ON src.id = rp.permission_id AND src.key = 'mentorship.request'
JOIN permissions p ON p.key = 'mentorship.apply'
ON CONFLICT DO NOTHING;

-- 4) Notification types for mentorship:notify_application_decision
--    (§35 categories; in-app + push on, email off like other review
--    decisions).
INSERT INTO notification_types (key, label, description, enabled, default_in_app, default_email, default_push, category) VALUES
    ('mentorship_application_approved', 'Mentor Application Approved',
     'Your mentor application was approved', true, true, false, true, 'system'),
    ('mentorship_application_declined', 'Mentor Application Declined',
     'Your mentor application was declined', true, true, false, true, 'system')
ON CONFLICT (key) DO NOTHING;

COMMIT;
