-- Editor block permissions.
-- Controls which block types each user group can insert into the content editor.

-- name: GetEnabledBlocksForGroups :many
SELECT DISTINCT block_type
FROM editor_block_permissions
WHERE user_group_id = ANY(@group_ids::uuid[])
 AND enabled = true;

-- name: UpsertEditorBlockPermission :exec
INSERT INTO editor_block_permissions (block_type, user_group_id, enabled) VALUES (@block_type, @user_group_id, @enabled)

ON CONFLICT (block_type, user_group_id) DO UPDATE
SET enabled = @enabled;

-- name: DeleteEditorBlockPermission :exec
DELETE FROM editor_block_permissions
WHERE block_type = @block_type AND user_group_id = @user_group_id;
