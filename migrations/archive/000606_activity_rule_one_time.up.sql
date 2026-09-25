-- Activity point rules: allow admins to mark a rule as one-time.
-- When one_time = true, only the first matching activity per user awards points.
-- Subsequent matching activities are skipped.
--
-- Ref: japaconnect-engineering/sections/28-wallet-credits.md

ALTER TABLE activity_point_rules
    ADD COLUMN IF NOT EXISTS one_time BOOLEAN NOT NULL DEFAULT false;
