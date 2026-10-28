-- 000948_donations_tips.down.sql
-- Reverses §125 schema + seeds. Order: indexes die with their tables;
-- FK dependents (tip_external_links, tips) drop before tip_settings.

BEGIN;

DELETE FROM public.notification_types WHERE key = 'tip.received';

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN ('tips.send', 'tips.receive', 'tips.moderate')
);
DELETE FROM permissions WHERE key IN ('tips.send', 'tips.receive', 'tips.moderate');

DROP TABLE IF EXISTS public.tips;
DROP TABLE IF EXISTS public.tip_external_links;
DROP TABLE IF EXISTS public.tip_settings;

COMMIT;
