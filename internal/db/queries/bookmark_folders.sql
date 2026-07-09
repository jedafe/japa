-- Bookmark folders (catalog: GET/POST /me/bookmark-folders, PATCH/DELETE
-- /me/bookmark-folders/:id). Distinct from bookmarks.folder (free-text,
-- migration 000103) — this table is a first-class named entity with an ID.

-- name: CreateBookmarkFolder :one
INSERT INTO bookmark_folders (tenant_id, user_id, name)
VALUES (@tenant_id, @user_id, @name)
RETURNING id, tenant_id, user_id, name, created_at, updated_at;

-- name: ListBookmarkFoldersByUser :many
SELECT id, tenant_id, user_id, name, created_at, updated_at
FROM bookmark_folders
WHERE tenant_id = @tenant_id AND user_id = @user_id
ORDER BY name;

-- name: UpdateBookmarkFolder :one
UPDATE bookmark_folders
SET name = @name, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND user_id = @user_id
RETURNING id, tenant_id, user_id, name, created_at, updated_at;

-- name: DeleteBookmarkFolder :execrows
DELETE FROM bookmark_folders
WHERE id = @id AND tenant_id = @tenant_id AND user_id = @user_id;
