BEGIN;

DROP TABLE IF EXISTS thread_anchors;

-- Restore legacy /forum URLs in stored config (mirrors the up-migration).
UPDATE public.pages
SET blocks = REPLACE(blocks::text, '"/thread"', '"/forum"')::jsonb
WHERE blocks::text LIKE '%"/thread"%';

UPDATE public.next_action_rules
SET payload = jsonb_set(payload, '{cta_url}', '"/forum"')
WHERE payload->>'cta_url' = '/thread';

UPDATE public.sidebars SET page_pattern = '/forum/*' WHERE slug = 'main-forum-right' AND page_pattern = '/thread/*';

UPDATE public.menu_items SET url = '/forum' WHERE url = '/thread';

DELETE FROM public.spaces WHERE id = '9c7c2a1d-2c47-4e3b-9f9d-7a1b9e6a2f01' AND creator_user_id IS NULL;

COMMIT;
