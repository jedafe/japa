-- 000518_share_platforms_permission.down.sql
-- Revert the share-platforms manage permission. Note that removing the
-- permission does not remove it from role assignments; on rollback the
-- endpoints would fall back to the previously used node until re-migrated.

BEGIN;

DELETE FROM permissions WHERE key = 'content.share_platforms.manage';

COMMIT;
