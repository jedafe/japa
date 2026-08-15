-- 000494_content_status_in_review.up.sql
-- Align contents.status with §14 spec: rename 'pending' → 'in_review' and
-- add the CHECK constraint that was missing from migration 000010.
--
-- The §14 spec defines:
--   CHECK (status IN ('draft','in_review','scheduled','published','archived'))
--
-- The codebase previously used 'pending' for the moderation-hold state
-- (content.StatusPending). This migration renames existing rows and adds
-- the spec-defined CHECK constraint so the database enforces the enum.

-- 1. Rename existing 'pending' rows to 'in_review'.
UPDATE contents SET status = 'in_review' WHERE status = 'pending';

-- 2. Add the spec-defined CHECK constraint on contents.status.
--    Drop first (IF EXISTS) for idempotency, then add.
ALTER TABLE contents DROP CONSTRAINT IF EXISTS contents_status_check;
ALTER TABLE contents ADD CONSTRAINT contents_status_check
    CHECK (status IN ('draft', 'in_review', 'scheduled', 'published', 'archived'));

-- 3. Add the spec-defined CHECK constraint on contents.visibility (also
--    missing from migration 000010). Idempotent.
ALTER TABLE contents DROP CONSTRAINT IF EXISTS contents_visibility_check;
ALTER TABLE contents ADD CONSTRAINT contents_visibility_check
    CHECK (visibility IN ('public', 'members', 'private'));
