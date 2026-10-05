-- Reverses 000796: drops the roadmap tables and removes the seeded
-- permission grants, permission nodes, and notification type.
-- roadmap_items is dropped before categories (category_id FK) and after
-- votes (votes cascade, but explicit order is clearer).

BEGIN;

DELETE FROM role_permissions WHERE permission_id IN
    (SELECT id FROM permissions WHERE key LIKE 'roadmap.%');
DELETE FROM permissions WHERE key LIKE 'roadmap.%';
DELETE FROM notification_types WHERE key = 'roadmap_shipped';

DROP TABLE IF EXISTS roadmap_votes;
DROP TABLE IF EXISTS roadmap_items;
DROP TABLE IF EXISTS roadmap_categories;

COMMIT;
