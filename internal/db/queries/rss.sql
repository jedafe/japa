-- RSS feed queries: content for RSS/Atom feeds.

-- name: ListRSSFeedItems :many
SELECT c.title, c.slug, LEFT(c.body, 200), c.created_at, u.username
FROM contents c
JOIN users u ON c.author_user_id = u.id
WHERE c.tenant_id = @tenant_id AND c.status = 'published'
ORDER BY c.created_at DESC
LIMIT 20;

-- name: ListRSSFeedItemsByContentType :many
SELECT c.title, c.slug, LEFT(c.body, 200), c.created_at, u.username
FROM contents c
JOIN users u ON c.author_user_id = u.id
WHERE c.tenant_id = @tenant_id AND c.status = 'published' AND c.content_type = @content_type
ORDER BY c.created_at DESC
LIMIT 20;

-- name: ListRSSSpaceFeedItems :many
SELECT c.title, c.slug, LEFT(c.body, 200), c.created_at, u.username
FROM contents c
JOIN users u ON c.author_user_id = u.id
JOIN spaces s ON c.space_id = s.id
WHERE c.tenant_id = @tenant_id AND c.status = 'published' AND s.slug = @slug
ORDER BY c.created_at DESC
LIMIT 20;

-- name: ListRSSTagFeedItems :many
SELECT c.title, c.slug, LEFT(c.body, 200), c.created_at, u.username
FROM contents c
JOIN users u ON c.author_user_id = u.id
JOIN content_tags ct ON ct.content_id = c.id
JOIN tags t ON ct.tag_id = t.id
WHERE c.tenant_id = @tenant_id AND c.status = 'published' AND t.slug = @slug
ORDER BY c.created_at DESC
LIMIT 20;

-- name: ListRSSUserFeedItems :many
SELECT c.title, c.slug, LEFT(c.body, 200), c.created_at, u.username
FROM contents c
JOIN users u ON c.author_user_id = u.id
WHERE c.tenant_id = @tenant_id AND c.status = 'published' AND u.username = @username
ORDER BY c.created_at DESC
LIMIT 20;
