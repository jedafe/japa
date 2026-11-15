-- 001158_atproto_lifecycle_a6.down.sql

BEGIN;

DELETE FROM role_permissions rp
USING user_roles r, permissions p
WHERE rp.role_id = r.id AND rp.permission_id = p.id
  AND r.slug = 'super_admin' AND p.key = 'atproto.accounts.manage';

DELETE FROM permissions WHERE key = 'atproto.accounts.manage';

ALTER TABLE atproto_repo_events
    DROP CONSTRAINT IF EXISTS atproto_repo_events_kind_check;

ALTER TABLE atproto_repo_events
    ADD CONSTRAINT atproto_repo_events_kind_check
    CHECK (kind IN ('commit','identity','account'));

ALTER TABLE atproto_accounts
    DROP CONSTRAINT IF EXISTS atproto_accounts_disabled_reason_check;

ALTER TABLE atproto_accounts
    ADD CONSTRAINT atproto_accounts_disabled_reason_check
    CHECK (disabled_reason IS NULL OR disabled_reason IN ('member', 'lifecycle'));

COMMIT;
