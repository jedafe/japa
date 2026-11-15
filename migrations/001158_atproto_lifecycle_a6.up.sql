-- 001158_atproto_lifecycle_a6.up.sql
-- A6-1 lifecycle completion (§127):
--   * atproto_accounts.disabled_reason gains 'admin' — an admin
--     force-disable the member cannot self-re-enable and lifecycle sync
--     never auto-reactivates (only 'lifecycle' rows do).
--   * atproto_repo_events.kind gains 'tombstone' — the #tombstone
--     subscribeRepos frame emitted after a repo's retraction commit on
--     GDPR erasure and admin force-tombstone.
--   * Seeds atproto.accounts.manage (super_admin) — gates the new
--     POST /control/atproto/accounts/{id}/{disable,reactivate,tombstone}.
--
-- Ref: japaconnect-engineering/sections/127-atproto-native.md (A6-1)

BEGIN;

ALTER TABLE atproto_accounts
    DROP CONSTRAINT IF EXISTS atproto_accounts_disabled_reason_check;

ALTER TABLE atproto_accounts
    ADD CONSTRAINT atproto_accounts_disabled_reason_check
    CHECK (disabled_reason IS NULL OR disabled_reason IN ('member', 'lifecycle', 'admin'));

ALTER TABLE atproto_repo_events
    DROP CONSTRAINT IF EXISTS atproto_repo_events_kind_check;

ALTER TABLE atproto_repo_events
    ADD CONSTRAINT atproto_repo_events_kind_check
    CHECK (kind IN ('commit','identity','account','tombstone'));

INSERT INTO permissions (key, description) VALUES
    ('atproto.accounts.manage', 'Manage atproto account lifecycle (force-disable, reactivate, tombstone)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
CROSS JOIN permissions p
WHERE r.slug = 'super_admin' AND p.key = 'atproto.accounts.manage'
ON CONFLICT DO NOTHING;

COMMIT;
