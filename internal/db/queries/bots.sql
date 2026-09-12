-- Bot & Crawler Management (Section 43)
-- Known bots registry, robots.txt generation, and bot detection.

-- =====================
-- Known Bots
-- =====================

-- name: CreateKnownBot :one
INSERT INTO known_bots (name, user_agent_pattern, bot_type, policy, is_system, crawl_delay) VALUES ($1, $2, $3, $4, $5, $6)

RETURNING *;

-- name: GetKnownBotByID :one
SELECT * FROM known_bots WHERE id = $1;

-- name: ListKnownBots :many
SELECT * FROM known_bots
WHERE (@bot_type = '' OR bot_type = @bot_type)
ORDER BY name ASC
LIMIT @page_size
OFFSET @page_offset;

-- name: CountKnownBots :one
SELECT count(*) FROM known_bots
WHERE (@bot_type = '' OR bot_type = @bot_type);

-- name: UpdateKnownBot :one
UPDATE known_bots
SET name = $2,
 user_agent_pattern = $3,
 bot_type = $4,
 policy = $5,
 crawl_delay = $6,
 updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteKnownBot :exec
DELETE FROM known_bots
WHERE id = $1;

-- name: GetBotsByUserAgent :many
SELECT * FROM known_bots
WHERE $1 ILIKE '%' || user_agent_pattern || '%';
