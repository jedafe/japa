-- 000725_recount_user_follower_counts.down.sql
-- The up migration only recomputes a denormalized counter from the source of
-- truth (follows); there is no meaningful rollback.
SELECT 1;
