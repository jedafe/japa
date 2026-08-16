-- Bookmark folders (catalog: GET/POST /me/bookmark-folders, PATCH/DELETE
-- /me/bookmark-folders/:id). Distinct from bookmarks.folder (free-text,
-- migration 000103) — this table is a first-class named entity with an ID.

-- name: CreateBookmarkFolder :one
INSERT INTO bookmark_folders (tenant_id, user_id, name)
VALUES (@tenant_id, @user_id, @name)
RETURNING id, tenant_id, user_id, name, created_at, updated_at, position;

-- name: ListBookmarkFoldersByUser :many
SELECT bf.id, bf.tenant_id, bf.user_id, bf.name, bf.created_at, bf.updated_at,
       bf.position, COALESCE(bm.cnt, 0) AS bookmark_count
FROM bookmark_folders bf
LEFT JOIN (
    SELECT folder_id, COUNT(*) AS cnt
    FROM bookmarks
    WHERE folder_id IS NOT NULL
    GROUP BY folder_id
) bm ON bm.folder_id = bf.id
WHERE bf.tenant_id = @tenant_id AND bf.user_id = @user_id
ORDER BY bf.position, bf.name;

-- name: UpdateBookmarkFolder :one
UPDATE bookmark_folders
SET name = @name, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND user_id = @user_id
RETURNING id, tenant_id, user_id, name, created_at, updated_at, position;

-- name: DeleteBookmarkFolder :execrows
DELETE FROM bookmark_folders
WHERE id = @id AND tenant_id = @tenant_id AND user_id = @user_id;

-- name: GetBookmarkFolder :one
-- Validates that a folder belongs to the given user and tenant (BL-013).
SELECT id, tenant_id, user_id, name, created_at, updated_at, position
FROM bookmark_folders
WHERE id = @id AND tenant_id = @tenant_id AND user_id = @user_id;

-- name: UpdateBookmarkFolderPosition :exec
-- BUG-008: sets the position of a single folder. Called in a loop by the
-- reorder handler (PATCH /me/bookmark-folders/reorder) to persist the
-- caller's desired ordering. Scoped to tenant + owner.
UPDATE bookmark_folders
SET position = @position, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND user_id = @user_id;
