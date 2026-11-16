-- 001173: Newsletter send lease (§62, ADR-080 §3) — send fencing column.
--
-- Every atomic send claim stamps a send lease identifying its owner: the
-- asynq task ID for worker-claimed sends (stable across retries — the task
-- itself is the lease identity), or a caller-generated ID for endpoint-
-- claimed sends (carried to the worker in the task payload). The worker
-- re-checks (status='sending' AND send_id=mine) per page and on the sent
-- stamp: a cancelled, reset (sending→draft), or re-claimed campaign fails
-- the check, so a superseded task self-terminates instead of mailing — or
-- stamping 'sent' — on behalf of an abandoned send. TEXT (not UUID) so it
-- can hold asynq ULID task IDs.
ALTER TABLE newsletter_campaigns
    ADD COLUMN IF NOT EXISTS send_id text;
