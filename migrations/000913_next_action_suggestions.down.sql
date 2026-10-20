-- 000913_next_action_suggestions.down.sql

BEGIN;

DELETE FROM role_permissions WHERE permission_id IN
    (SELECT id FROM permissions WHERE key IN ('nextactions.manage','nextactions.view'));
DELETE FROM permissions WHERE key IN ('nextactions.manage','nextactions.view');
DELETE FROM notification_types WHERE key = 'next_action.suggested';
DROP TABLE IF EXISTS public.user_next_actions;
DROP TABLE IF EXISTS public.next_action_rules;

COMMIT;
