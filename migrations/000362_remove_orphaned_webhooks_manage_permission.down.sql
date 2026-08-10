-- Reverse migration 000362: re-add the orphaned webhooks.manage permission node.
INSERT INTO permissions (key, description)
VALUES ('webhooks.manage', 'Manage webhooks')
ON CONFLICT (key) DO NOTHING;
