-- 001146_atproto_commit_blocks.down.sql — drop the per-commit block index.
BEGIN;
DROP TABLE IF EXISTS atproto_commit_blocks;
COMMIT;
