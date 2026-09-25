-- 000349_create_mentorship_matching_tables.up.sql
-- Mentorship Matching (§102, ADR-110).
-- Adds mentor opt-in columns to user_profiles and creates the
-- mentorship_connections table with the active_mentee_count trigger.
--
-- Adaptation: §102 references sites(id), BIGSERIAL, and geo_countries(id).
-- Per ADR-053, tenancy uses tenants(id) (uuid), and countries are spaces
-- with level='country' — destination_country_id becomes destination_space_id
-- referencing spaces(id).
--
-- Ref: japaconnect-engineering/sections/102-mentorship-matching.md
--      japaconnect-engineering/adr/ADR-110-mentorship-matching.md

BEGIN;

-- =====================================================================
-- 1. Mentor opt-in columns on user_profiles (ADR-110 decision: columns,
--    not a separate table).
-- =====================================================================
ALTER TABLE user_profiles ADD COLUMN IF NOT EXISTS is_mentor           boolean     DEFAULT false;
ALTER TABLE user_profiles ADD COLUMN IF NOT EXISTS mentor_capacity     smallint    DEFAULT 3;
ALTER TABLE user_profiles ADD COLUMN IF NOT EXISTS active_mentee_count smallint    DEFAULT 0;
ALTER TABLE user_profiles ADD COLUMN IF NOT EXISTS mentor_areas        text[];
ALTER TABLE user_profiles ADD COLUMN IF NOT EXISTS mentor_bio          text;
ALTER TABLE user_profiles ADD COLUMN IF NOT EXISTS mentor_since        timestamptz;
ALTER TABLE user_profiles ADD COLUMN IF NOT EXISTS mentor_embedding    vector(1536);

CREATE INDEX IF NOT EXISTS idx_user_profiles_is_mentor
    ON user_profiles (tenant_id, is_mentor) WHERE is_mentor = true;

-- =====================================================================
-- 2. mentorship_connections table.
--    Status flow:
--      pending → active (mentor accepts)
--      pending → declined (mentor declines)
--      active → completed (either party)
--      active → cancelled (either party)
-- =====================================================================
CREATE TABLE IF NOT EXISTS mentorship_connections (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    mentor_id             uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mentee_id             uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status                text NOT NULL DEFAULT 'pending',
    goals                 text,
    destination_space_id  uuid REFERENCES spaces(id) ON DELETE SET NULL,
    responded_at          timestamptz,
    completed_at          timestamptz,
    cancelled_at          timestamptz,
    cancel_reason         text,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    UNIQUE (mentor_id, mentee_id, status)
);

CREATE INDEX IF NOT EXISTS idx_mentorship_conn_mentor_status
    ON mentorship_connections (mentor_id, status);
CREATE INDEX IF NOT EXISTS idx_mentorship_conn_mentee_status
    ON mentorship_connections (mentee_id, status);
CREATE INDEX IF NOT EXISTS idx_mentorship_conn_tenant_status
    ON mentorship_connections (tenant_id, status);

-- =====================================================================
-- 3. active_mentee_count trigger (ADR-110: maintained by DB trigger,
--    never computed at query time). Fires on INSERT, UPDATE of status,
--    and DELETE to keep the count accurate in all cases.
-- =====================================================================
CREATE OR REPLACE FUNCTION trg_update_active_mentee_count()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
  -- On DELETE, use OLD.mentor_id; on INSERT/UPDATE, use NEW.mentor_id.
  -- COALESCE handles both cases: NEW is NULL on DELETE, OLD is NULL on INSERT.
  UPDATE user_profiles
  SET active_mentee_count = (
    SELECT COUNT(*) FROM mentorship_connections
    WHERE mentor_id = COALESCE(NEW.mentor_id, OLD.mentor_id)
    AND status = 'active'
  )
  WHERE user_id = COALESCE(NEW.mentor_id, OLD.mentor_id);
  RETURN COALESCE(NEW, OLD);
END;
$$;

DROP TRIGGER IF EXISTS trg_mentee_count ON mentorship_connections;
CREATE TRIGGER trg_mentee_count
AFTER INSERT OR UPDATE OF status OR DELETE ON mentorship_connections
FOR EACH ROW EXECUTE FUNCTION trg_update_active_mentee_count();

COMMIT;
