-- Section 35 (Mentorship): Mentorship program and match tracking tables
-- Supports mentorship programs with mentor-mentee matching.

CREATE TABLE IF NOT EXISTS mentorship_programs (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        text NOT NULL,
    description text NOT NULL DEFAULT '',
    is_active   boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_mentorship_programs_tenant ON mentorship_programs(tenant_id, is_active);

CREATE TABLE IF NOT EXISTS mentorship_matches (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    program_id uuid NOT NULL REFERENCES mentorship_programs(id) ON DELETE CASCADE,
    mentor_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mentee_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status     text NOT NULL DEFAULT 'active',  -- 'active' | 'completed' | 'cancelled'
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_mentorship_matches_tenant ON mentorship_matches(tenant_id, status);
CREATE INDEX idx_mentorship_matches_program ON mentorship_matches(program_id);
CREATE INDEX idx_mentorship_matches_mentor ON mentorship_matches(mentor_id);
CREATE INDEX idx_mentorship_matches_mentee ON mentorship_matches(mentee_id);
