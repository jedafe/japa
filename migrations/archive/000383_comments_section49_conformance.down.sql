-- 000383_comments_section49_conformance.down.sql
-- Reverses 000383: drops content_thread_links, the comments.pin grant/node,
-- and the Section 49 columns added to comments.

BEGIN;

-- RBAC: remove comments.pin grants + node.
DELETE FROM role_permissions
WHERE permission_id = (SELECT id FROM permissions WHERE key = 'comments.pin');
DELETE FROM permissions WHERE key = 'comments.pin';

-- content_thread_links
DROP INDEX IF EXISTS idx_content_thread_links_thread;
DROP INDEX IF EXISTS idx_content_thread_links_content;
DROP TABLE IF EXISTS content_thread_links;

-- comments columns
DROP INDEX IF EXISTS idx_comments_thread;
ALTER TABLE comments DROP COLUMN IF EXISTS removed_reason;
ALTER TABLE comments DROP COLUMN IF EXISTS removed_by;
ALTER TABLE comments DROP COLUMN IF EXISTS status;
ALTER TABLE comments DROP COLUMN IF EXISTS author_profile_id;
ALTER TABLE comments DROP COLUMN IF EXISTS reply_count;
ALTER TABLE comments DROP COLUMN IF EXISTS is_anonymous;

COMMIT;
