-- Seed the users.username_cooldown_days system setting (BUG-005, Rule 3).
-- Replaces the hardcoded `const usernameCooldownDays = 30` in
-- internal/users/service.go with a configurable system_settings value.
-- Default tenant UUID: '00000000-0000-0000-0000-000000000001'.
-- Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
INSERT INTO system_settings (tenant_id, key, value)
VALUES ('00000000-0000-0000-0000-000000000001', 'users.username_cooldown_days', '"30"')
ON CONFLICT (tenant_id, key) DO NOTHING;
