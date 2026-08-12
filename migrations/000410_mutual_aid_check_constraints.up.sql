-- 000410_mutual_aid_check_constraints.up.sql
-- Add CHECK constraints for mutual_aid_posts post_type, category, and
-- status to match §85 spec, and make expires_at NOT NULL (the service
-- always sets it). Also migrate any legacy 'moderated' status rows to
-- 'removed' per the spec.
-- Ref: japaconnect-engineering/sections/85-mutual-aid-board.md

BEGIN;

-- Migrate legacy status values before adding the CHECK constraint.
-- 'moderated' was the pre-spec value; the spec uses 'removed'.
UPDATE mutual_aid_posts SET status = 'removed' WHERE status = 'moderated';

-- Make expires_at NOT NULL (BUG-018). The service always sets it on
-- create and renew, so no existing NULL rows should exist. If any do,
-- backfill with created_at + 30 days before altering.
UPDATE mutual_aid_posts
SET expires_at = created_at + interval '30 days'
WHERE expires_at IS NULL;

ALTER TABLE mutual_aid_posts ALTER COLUMN expires_at SET NOT NULL;

-- Add CHECK constraints matching the §85 spec enums (BUG-006).
ALTER TABLE mutual_aid_posts
    ADD CONSTRAINT mutual_aid_posts_post_type_check
    CHECK (post_type IN ('need', 'offer', 'skill_swap'));

ALTER TABLE mutual_aid_posts
    ADD CONSTRAINT mutual_aid_posts_category_check
    CHECK (category IN ('housing', 'transport', 'documents', 'job_search', 'language', 'social', 'other'));

ALTER TABLE mutual_aid_posts
    ADD CONSTRAINT mutual_aid_posts_status_check
    CHECK (status IN ('active', 'expired', 'fulfilled', 'removed'));

COMMIT;
