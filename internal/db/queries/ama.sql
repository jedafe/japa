-- Ask an Expat (§88, ADR-096) — AMA queries on forum_threads.
-- AMA threads are forum_threads with ama_config IS NOT NULL.
-- Ref: japaconnect-engineering/sections/88-ask-an-expat.md

-- =====================
-- Creation
-- =====================

-- name: CreateAMA :one
INSERT INTO forum_threads (
    tenant_id, space_id, author_user_id, title, slug, body,
    is_anonymous, edit_window_minutes, ama_config
) VALUES (
    @tenant_id, sqlc.narg(space_id), @author_user_id, @title, @slug, @body,
    @is_anonymous, @edit_window_minutes, @ama_config
)
RETURNING id, tenant_id, content_id, space_id, author_user_id, parent_reply_id,
          title, slug, body, is_anonymous, solved, solved_at, solved_by_user_id,
          vote_score, reply_count, view_count, bookmark_count, edit_window_minutes,
          is_edited, created_at, updated_at, deleted_at, ama_config;

-- =====================
-- Retrieval
-- =====================

-- name: GetAMAByID :one
SELECT ft.id, ft.tenant_id, ft.content_id, ft.space_id, ft.author_user_id,
       ft.parent_reply_id, ft.title, ft.slug, ft.body, ft.is_anonymous,
       ft.solved, ft.solved_at, ft.solved_by_user_id, ft.vote_score,
       ft.reply_count, ft.view_count, ft.bookmark_count, ft.edit_window_minutes,
       ft.is_edited, ft.created_at, ft.updated_at, ft.deleted_at, ft.ama_config,
       CASE WHEN ft.is_anonymous THEN NULL ELSE u.username END AS author_username,
       CASE WHEN ft.is_anonymous THEN NULL ELSE up.avatar_url END AS author_avatar_url
FROM forum_threads ft
LEFT JOIN users u ON u.id = ft.author_user_id
LEFT JOIN user_profiles up ON up.user_id = ft.author_user_id
WHERE ft.id = @id AND ft.tenant_id = @tenant_id
  AND ft.ama_config IS NOT NULL AND ft.deleted_at IS NULL;

-- name: ListActiveAMAs :many
SELECT ft.id, ft.tenant_id, ft.content_id, ft.space_id, ft.author_user_id,
       ft.parent_reply_id, ft.title, ft.slug, ft.body, ft.is_anonymous,
       ft.solved, ft.solved_at, ft.solved_by_user_id, ft.vote_score,
       ft.reply_count, ft.view_count, ft.bookmark_count, ft.edit_window_minutes,
       ft.is_edited, ft.created_at, ft.updated_at, ft.deleted_at, ft.ama_config,
       CASE WHEN ft.is_anonymous THEN NULL ELSE u.username END AS author_username,
       CASE WHEN ft.is_anonymous THEN NULL ELSE up.avatar_url END AS author_avatar_url
FROM forum_threads ft
LEFT JOIN users u ON u.id = ft.author_user_id
LEFT JOIN user_profiles up ON up.user_id = ft.author_user_id
WHERE ft.tenant_id = @tenant_id
  AND ft.ama_config IS NOT NULL
  AND ft.deleted_at IS NULL
  AND (ft.ama_config->>'closes_at' IS NULL
       OR (ft.ama_config->>'closes_at')::timestamptz > now())
  AND (sqlc.narg(dest)::text IS NULL
       OR ft.ama_config->>'host_dest_country_id' = @dest)
  AND (sqlc.narg(profession)::text IS NULL
       OR ft.ama_config->>'host_profession' ILIKE '%' || @profession || '%')
  AND (sqlc.narg(cursor_created_at)::timestamptz IS NULL
       OR ft.created_at < @cursor_created_at)
ORDER BY ft.created_at DESC, ft.id DESC
LIMIT @page_limit;

-- =====================
-- Authorization helpers
-- =====================

-- name: GetAMAAuthorID :one
SELECT author_user_id FROM forum_threads
WHERE id = @id AND tenant_id = @tenant_id
  AND ama_config IS NOT NULL AND deleted_at IS NULL;

-- =====================
-- Rate limiting
-- =====================

-- name: HasActiveAMA :one
SELECT EXISTS(
    SELECT 1 FROM forum_threads
    WHERE author_user_id = @author_user_id
      AND tenant_id = @tenant_id
      AND ama_config IS NOT NULL
      AND deleted_at IS NULL
      AND (ama_config->>'closes_at' IS NULL
           OR (ama_config->>'closes_at')::timestamptz > now())
) AS has_active;

-- name: CheckAMACooldown :one
SELECT EXISTS(
    SELECT 1 FROM forum_threads
    WHERE author_user_id = @author_user_id
      AND tenant_id = @tenant_id
      AND ama_config IS NOT NULL
      AND deleted_at IS NULL
      AND created_at > now() - make_interval(days => @cooldown_days)
) AS in_cooldown;

-- =====================
-- Closing
-- =====================

-- name: CloseAMA :exec
UPDATE forum_threads
SET ama_config = jsonb_set(ama_config, '{closes_at}', to_jsonb(now()::timestamptz), true),
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
  AND ama_config IS NOT NULL AND deleted_at IS NULL;
