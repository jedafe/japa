-- 001224_housing_guides_status_default.down.sql
-- Restores the baseline 'published' column default (see up file for why
-- that default bypasses the ADR-088 HITL gate).

ALTER TABLE housing_guides ALTER COLUMN status SET DEFAULT 'published';
