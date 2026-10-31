-- name: UpsertFormDraft :one
INSERT INTO form_drafts (user_id, kind, draft_key, data)
VALUES (@user_id::uuid, @kind, @draft_key, @data::jsonb)
ON CONFLICT (user_id, kind, draft_key)
DO UPDATE SET data = EXCLUDED.data, updated_at = now()
RETURNING id, user_id, kind, draft_key, data, created_at, updated_at;

-- name: GetFormDraft :one
SELECT id, user_id, kind, draft_key, data, created_at, updated_at
FROM form_drafts
WHERE user_id = @user_id::uuid AND kind = @kind AND draft_key = @draft_key;

-- name: ListFormDraftsByUser :many
SELECT id, user_id, kind, draft_key, data, created_at, updated_at
FROM form_drafts
WHERE user_id = @user_id::uuid
ORDER BY updated_at DESC
LIMIT 50;

-- name: DeleteFormDraft :exec
DELETE FROM form_drafts
WHERE user_id = @user_id::uuid AND kind = @kind AND draft_key = @draft_key;

-- name: UserHasDraftOfKind :one
SELECT EXISTS(
    SELECT 1 FROM form_drafts
    WHERE user_id = @user_id::uuid
      AND (@kind::text = '' OR kind = @kind)
);
