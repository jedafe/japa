-- 001148_atproto_conformance_runs.up.sql
-- Admin-triggered A3 release-gate results: POST /admin/atproto/conformance
-- runs the same appview readback checks as cmd/atproto-conformance and
-- persists each run so the /atproto admin page can show when indexing
-- last broke. run_by is nullable so a later GDPR hard-delete of the
-- admin's user row can't cascade the history away.
--
-- Ref: japaconnect-engineering/sections/127-atproto-native.md (Admin)

BEGIN;

CREATE TABLE IF NOT EXISTS atproto_conformance_runs (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor      text NOT NULL,               -- handle or DID the run targeted
    did        text NOT NULL DEFAULT '',    -- resolved DID ('' when unresolved)
    checks     jsonb NOT NULL,              -- [{name, pass, detail}]
    passed     boolean NOT NULL,
    run_by     uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_atproto_conformance_runs_created
    ON atproto_conformance_runs (created_at DESC);

COMMIT;
