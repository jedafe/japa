-- 000928_seed_roadmap_agent_tools.down.sql
--
-- Remove the six roadmap agent tools. The reused permission nodes
-- (roadmap.view / .create / .edit / .moderate) are left in place — they
-- predate this migration and gate the REST admin routes.

DELETE FROM public.ai_tools
WHERE name IN (
    'roadmap.categories.list',
    'roadmap.items.list',
    'roadmap.item.get',
    'roadmap.item.create',
    'roadmap.item.update',
    'roadmap.item.set_status'
);
