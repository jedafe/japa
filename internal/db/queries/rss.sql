-- RSS feed queries: content for RSS/Atom feeds.
-- All queries filter deleted_at IS NULL (Rule 11 / GDPR) and auto_hidden = false
-- (spam prevention), and order by published_at DESC (falling back to created_at
-- in the Go layer for items where published_at is NULL).

-- name: ListRSSFeedItems :many
SELECT c.id, c.title, c.slug, c.content_type, c.body_html, c.body, c.published_at, c.created_at, COALESCE(u.public_username, u.username, '') AS username
FROM contents c
JOIN users u ON c.author_id = u.id
WHERE c.status = 'published'
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
ORDER BY c.published_at DESC NULLS LAST
LIMIT @page_limit;

-- name: ListRSSFeedItemsByContentType :many
SELECT c.id, c.title, c.slug, c.content_type, c.body_html, c.body, c.published_at, c.created_at, COALESCE(u.public_username, u.username, '') AS username
FROM contents c
JOIN users u ON c.author_id = u.id
WHERE c.status = 'published'
 AND c.content_type = @content_type
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
ORDER BY c.published_at DESC NULLS LAST
LIMIT @page_limit;

-- name: ListRSSSpaceFeedItems :many
SELECT c.id, c.title, c.slug, c.content_type, c.body_html, c.body, c.published_at, c.created_at, COALESCE(u.public_username, u.username, '') AS username
FROM contents c
JOIN users u ON c.author_id = u.id
JOIN spaces s ON c.space_id = s.id
WHERE c.status = 'published'
 AND s.slug = @slug
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
ORDER BY c.published_at DESC NULLS LAST
LIMIT @page_limit;

-- name: ListRSSTagFeedItems :many
SELECT c.id, c.title, c.slug, c.content_type, c.body_html, c.body, c.published_at, c.created_at, COALESCE(u.public_username, u.username, '') AS username
FROM contents c
JOIN users u ON c.author_id = u.id
JOIN content_tags ct ON ct.content_id = c.id
JOIN tags t ON ct.tag_id = t.id
WHERE c.status = 'published'
 AND t.slug = @slug
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
ORDER BY c.published_at DESC NULLS LAST
LIMIT @page_limit;

-- name: ListRSSUserFeedItems :many
SELECT c.id, c.title, c.slug, c.content_type, c.body_html, c.body, c.published_at, c.created_at, COALESCE(u.public_username, u.username, '') AS username
FROM contents c
JOIN users u ON c.author_id = u.id
WHERE c.status = 'published'
 AND (u.username = @username OR lower(u.public_username) = lower(@username))
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
ORDER BY c.published_at DESC NULLS LAST
LIMIT @page_limit;

-- name: ListRSSForumFeedItems :many
-- Forum threads join their backing contents row for body_html/auto_hidden.
-- Only threads in public spaces are emitted: 'members'-space threads would
-- leak member-only bodies into a public feed, and 'private'-space threads
-- are invisible to anonymous readers entirely. is_anonymous strips the
-- author so the feed never de-anonymizes a thread.
-- Visibility mirrors the public thread gate (CreateThread): unscoped
-- threads are public immediately (their backing row stays 'draft'), while
-- space-scoped threads need a 'published' backing row so in-review threads
-- held for moderation never leak into the feed.
SELECT ft.id, ft.title, ft.slug,
 c.body_html, c.body, c.published_at, ft.created_at,
 CASE WHEN ft.is_anonymous THEN '' ELSE COALESCE(u.public_username, u.username, '') END::text AS username
FROM forum_threads ft
JOIN contents c ON c.id = ft.content_id
JOIN users u ON u.id = ft.author_user_id
LEFT JOIN spaces s ON s.id = ft.space_id
WHERE ft.deleted_at IS NULL
 AND (ft.space_id IS NULL OR c.status = 'published')
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
 AND (s.id IS NULL OR s.visibility = 'public')
ORDER BY ft.created_at DESC
LIMIT @page_limit;
