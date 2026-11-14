-- 001145_atproto_sync.down.sql — drop the A3 firehose sequencer.
BEGIN;
DROP TABLE IF EXISTS atproto_repo_events;
COMMIT;
