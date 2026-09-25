-- 000233_create_user_content_views.up.sql
-- Creates the user_content_views table for deduplicated content view tracking
-- (Section 14 — Content Engagement Schemas). The PK deduplicates per
-- user/session per day; country_code is resolved server-side via GeoIP.
-- Ref: japaconnect-engineering/sections/14-content-type-system.md

BEGIN;

CREATE TABLE IF NOT EXISTS user_content_views (
  id          uuid NOT NULL DEFAULT gen_random_uuid(),
  content_id  uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
  user_id     uuid REFERENCES users(id) ON DELETE CASCADE,
  session_id  text,
  country_code text,
  viewed_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (id)
);

-- Deduplicate per user/session per day (Section 14).
-- date_trunc on timestamptz is not IMMUTABLE (timezone-dependent), so cast
-- to timestamp via AT TIME ZONE 'UTC' first to make the expression immutable.
CREATE UNIQUE INDEX IF NOT EXISTS idx_ucv_dedup
  ON user_content_views (content_id, COALESCE(user_id::text, session_id), date_trunc('day', viewed_at AT TIME ZONE 'UTC'));

CREATE INDEX IF NOT EXISTS idx_ucv_content ON user_content_views (content_id, viewed_at DESC);

COMMIT;
