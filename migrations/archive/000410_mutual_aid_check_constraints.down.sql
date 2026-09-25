-- 000410_mutual_aid_check_constraints.down.sql
-- Revert CHECK constraints and expires_at NOT NULL change.

BEGIN;

ALTER TABLE mutual_aid_posts DROP CONSTRAINT IF EXISTS mutual_aid_posts_status_check;
ALTER TABLE mutual_aid_posts DROP CONSTRAINT IF EXISTS mutual_aid_posts_category_check;
ALTER TABLE mutual_aid_posts DROP CONSTRAINT IF EXISTS mutual_aid_posts_post_type_check;

ALTER TABLE mutual_aid_posts ALTER COLUMN expires_at DROP NOT NULL;

-- Revert 'removed' back to 'moderated' (legacy value).
UPDATE mutual_aid_posts SET status = 'moderated' WHERE status = 'removed';

COMMIT;
