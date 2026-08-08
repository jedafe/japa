-- Re-soft-delete the geo spaces that were reactivated by 000287.
-- We cannot know the original deleted_at timestamp, so we set it to now().
UPDATE spaces
SET status = 'deleted', deleted_at = now()
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND level IN ('country', 'state')
  AND status = 'active'
  AND slug = 'liberia';
