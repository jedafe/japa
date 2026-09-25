-- Reverse: re-add NOT NULL constraint.
-- Note: any rows with NULL reviewer_user_id must be backfilled before this
-- down migration can apply.
ALTER TABLE content_reviews ALTER COLUMN reviewer_user_id SET NOT NULL;
