-- RSS feed queries: content for RSS/Atom feeds.
-- All queries filter deleted_at IS NULL (Rule 11 / GDPR) and auto_hidden = false
-- (spam prevention), and order by published_at DESC (falling back to created_at
-- in the Go layer for items where published_at is NULL).

-- name: ListRSSFeedItems :many
SELECT c.id, c.title, c.slug, c.body_html, c.body, c.published_at, c.created_at, u.username
FROM contents c
JOIN users u ON c.author_id = u.id
WHERE c.status = 'published'
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
ORDER BY c.published_at DESC NULLS LAST
LIMIT @page_limit;

-- name: ListRSSFeedItemsByContentType :many
SELECT c.id, c.title, c.slug, c.body_html, c.body, c.published_at, c.created_at, u.username
FROM contents c
JOIN users u ON c.author_id = u.id
WHERE c.status = 'published'
 AND c.content_type = @content_type
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
ORDER BY c.published_at DESC NULLS LAST
LIMIT @page_limit;

-- name: ListRSSSpaceFeedItems :many
SELECT c.id, c.title, c.slug, c.body_html, c.body, c.published_at, c.created_at, u.username
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
SELECT c.id, c.title, c.slug, c.body_html, c.body, c.published_at, c.created_at, u.username
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
SELECT c.id, c.title, c.slug, c.body_html, c.body, c.published_at, c.created_at, u.username
FROM contents c
JOIN users u ON c.author_id = u.id
WHERE c.status = 'published'
 AND u.username = @username
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
ORDER BY c.published_at DESC NULLS LAST
LIMIT @page_limit;
