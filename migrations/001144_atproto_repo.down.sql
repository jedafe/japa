-- 001144_atproto_repo.down.sql — drops the §127 A2 repo engine tables.

BEGIN;

DROP TABLE IF EXISTS atproto_blobs;
DROP TABLE IF EXISTS atproto_records;
DROP TABLE IF EXISTS atproto_repo_blocks;
DROP TABLE IF EXISTS atproto_repo_commits;

COMMIT;
