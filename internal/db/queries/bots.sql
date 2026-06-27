-- Bot & Crawler Management (Section 43)
-- Known bots registry, robots.txt generation, and bot detection.

-- =====================
-- Known Bots
-- =====================

-- name: CreateKnownBot :one
INSERT INTO known_bots (tenant_id, name, user_agent_pattern, bot_type, policy, is_system, crawl_delay)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, tenant_id, name, user_agent_pattern, bot_type, policy, is_system, crawl_delay, updated_at, created_at;

-- name: GetKnownBotByID :one
SELECT id, tenant_id, name, user_agent_pattern, bot_type, policy, is_system, crawl_delay, updated_at, created_at
FROM known_bots
WHERE id = $1 AND tenant_id = $2;

-- name: ListKnownBots :many
SELECT id, tenant_id, name, user_agent_pattern, bot_type, policy, is_system, crawl_delay, updated_at, created_at
FROM known_bots
WHERE tenant_id = $1
  AND (@bot_type = '' OR bot_type = @bot_type)
ORDER BY name ASC;

-- name: UpdateKnownBot :one
UPDATE known_bots
SET name = $3,
    user_agent_pattern = $4,
    bot_type = $5,
    policy = $6,
    crawl_delay = $7,
    updated_at = now()
WHERE id = $1 AND tenant_id = $2
RETURNING id, tenant_id, name, user_agent_pattern, bot_type, policy, is_system, crawl_delay, updated_at, created_at;

-- name: DeleteKnownBot :exec
DELETE FROM known_bots
WHERE id = $1 AND tenant_id = $2;

-- name: GetBotsByUserAgent :many
SELECT id, tenant_id, name, user_agent_pattern, bot_type, policy, is_system, crawl_delay, updated_at, created_at
FROM known_bots
WHERE tenant_id = $1
  AND $2 ILIKE '%' || user_agent_pattern || '%';
