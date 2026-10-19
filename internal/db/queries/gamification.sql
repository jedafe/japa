-- gamification.sql — Advanced Gamification queries (Section 64, ADR-082).
-- Covers badge definitions, user badges, challenge definitions, progress,
-- and leaderboard queries.
-- Ref: japaconnect-engineering/sections/64-gamification.md
--      japaconnect-engineering/adr/ADR-082-advanced-gamification.md

-- =====================================================================
-- Badge Definitions
-- =====================================================================

-- name: ListBadgeDefinitions :many
SELECT * FROM badge_definitions
WHERE (@active_only::boolean = false OR is_active = true)
ORDER BY display_order ASC, name ASC;

-- name: ListBadgeDefinitionsPaginated :many
SELECT * FROM badge_definitions
WHERE (@active_only::boolean = false OR is_active = true)
ORDER BY display_order ASC, name ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountBadgeDefinitions :one
SELECT COUNT(*) AS count
FROM badge_definitions
WHERE (@active_only::boolean = false OR is_active = true);

-- name: GetBadgeDefinitionByID :one
SELECT * FROM badge_definitions
WHERE id = @id;

-- name: CreateBadgeDefinition :one
INSERT INTO badge_definitions (slug, name, description, icon_media_id, trigger_type, trigger_value, trigger_activity_key, is_active, is_system, display_order) VALUES (@slug, @name, sqlc.narg(description), sqlc.narg(icon_media_id), @trigger_type, sqlc.narg(trigger_value), sqlc.narg(trigger_activity_key), @is_active, @is_system, @display_order)

RETURNING *;

-- name: UpdateBadgeDefinition :one
UPDATE badge_definitions
SET
 name = COALESCE(sqlc.narg(name), name),
 description = COALESCE(sqlc.narg(description), description),
 icon_media_id = COALESCE(sqlc.narg(icon_media_id), icon_media_id),
 trigger_type = COALESCE(sqlc.narg(trigger_type), trigger_type),
 trigger_value = COALESCE(sqlc.narg(trigger_value), trigger_value),
 trigger_activity_key = COALESCE(sqlc.narg(trigger_activity_key), trigger_activity_key),
 is_active = COALESCE(sqlc.narg(is_active), is_active),
 display_order = COALESCE(sqlc.narg(display_order), display_order)
WHERE id = @id
RETURNING *;

-- name: DeleteBadgeDefinition :execrows
DELETE FROM badge_definitions
WHERE id = @id AND is_system = false;

-- =====================================================================
-- User Badges
-- =====================================================================

-- name: AwardBadge :one
INSERT INTO user_badges (user_id, badge_id, awarded_by) VALUES (@user_id, @badge_id, sqlc.narg(awarded_by))

ON CONFLICT (user_id, badge_id) DO NOTHING
RETURNING *;

-- name: ListUserBadges :many
SELECT
 ub.*,
 bd.name AS badge_name,
 bd.slug AS badge_slug,
 bd.description AS badge_description,
 bd.icon_media_id AS badge_icon_media_id
FROM user_badges ub
JOIN badge_definitions bd ON bd.id = ub.badge_id
WHERE ub.user_id = @user_id
ORDER BY ub.awarded_at DESC;

-- name: ListUserBadgesByUsername :many
SELECT
 ub.*,
 bd.name AS badge_name,
 bd.slug AS badge_slug,
 bd.description AS badge_description
FROM user_badges ub
JOIN badge_definitions bd ON bd.id = ub.badge_id
JOIN users u ON u.id = ub.user_id
WHERE (u.username = @username OR lower(u.public_username) = lower(@username))
ORDER BY ub.awarded_at DESC;

-- name: GetUserBadge :one
SELECT * FROM user_badges
WHERE user_id = @user_id AND badge_id = @badge_id;

-- =====================================================================
-- Challenge Definitions
-- =====================================================================

-- name: ListActiveChallenges :many
SELECT * FROM challenge_definitions
WHERE is_active = true
 AND starts_at <= now()
 AND (ends_at IS NULL OR ends_at > now())
ORDER BY created_at DESC;

-- name: ListAllChallenges :many
SELECT * FROM challenge_definitions

ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAllChallenges :one
SELECT COUNT(*) AS count
FROM challenge_definitions
;

-- name: GetChallengeByID :one
SELECT * FROM challenge_definitions
WHERE id = @id;

-- name: CreateChallenge :one
INSERT INTO challenge_definitions (slug, name, description, image_media_id, activity_key, target_count, reward_points, reward_badge_id, starts_at, ends_at, is_active) VALUES (@slug, @name, sqlc.narg(description), sqlc.narg(image_media_id), @activity_key, @target_count, @reward_points, sqlc.narg(reward_badge_id), @starts_at, sqlc.narg(ends_at), @is_active)

RETURNING *;

-- name: UpdateChallenge :one
UPDATE challenge_definitions
SET
 name = COALESCE(sqlc.narg(name), name),
 description = COALESCE(sqlc.narg(description), description),
 image_media_id = COALESCE(sqlc.narg(image_media_id), image_media_id),
 activity_key = COALESCE(sqlc.narg(activity_key), activity_key),
 target_count = COALESCE(sqlc.narg(target_count), target_count),
 reward_points = COALESCE(sqlc.narg(reward_points), reward_points),
 reward_badge_id = COALESCE(sqlc.narg(reward_badge_id), reward_badge_id),
 starts_at = COALESCE(sqlc.narg(starts_at), starts_at),
 ends_at = COALESCE(sqlc.narg(ends_at), ends_at),
 is_active = COALESCE(sqlc.narg(is_active), is_active)
WHERE id = @id
RETURNING *;

-- name: DeleteChallenge :execrows
DELETE FROM challenge_definitions
WHERE id = @id;

-- name: ExpireChallengesPastEndDate :execrows
UPDATE challenge_definitions
SET is_active = false
WHERE is_active = true
 AND ends_at IS NOT NULL
 AND ends_at < now();

-- =====================================================================
-- User Challenge Progress
-- =====================================================================

-- completed/completed_at must be computed on the INSERT path too — the
-- conflict path alone leaves a target_count=1 challenge un-completed after
-- the first event (off-by-one). completed_at is set once, on the first
-- transition to completed — later increments must not drift the timestamp.
-- name: UpsertChallengeProgress :one
INSERT INTO user_challenge_progress (user_id, challenge_id, current_count, completed, completed_at) VALUES (@user_id, @challenge_id, @current_count, @current_count >= (SELECT target_count FROM challenge_definitions WHERE id = @challenge_id), CASE WHEN @current_count >= (SELECT target_count FROM challenge_definitions WHERE id = @challenge_id)
 THEN now() ELSE NULL END)

ON CONFLICT (user_id, challenge_id)
DO UPDATE SET
 current_count = user_challenge_progress.current_count + EXCLUDED.current_count,
 completed = CASE
 WHEN user_challenge_progress.current_count + EXCLUDED.current_count >= (
 SELECT target_count FROM challenge_definitions WHERE id = @challenge_id
 ) THEN true
 ELSE false
 END,
 completed_at = CASE
 WHEN user_challenge_progress.completed THEN user_challenge_progress.completed_at
 WHEN user_challenge_progress.current_count + EXCLUDED.current_count >= (
 SELECT target_count FROM challenge_definitions WHERE id = @challenge_id
 ) THEN now()
 ELSE user_challenge_progress.completed_at
 END
RETURNING *;

-- name: ListUserChallengeProgress :many
SELECT
 ucp.*,
 cd.name AS challenge_name,
 cd.slug AS challenge_slug,
 cd.description AS challenge_description,
 cd.target_count,
 cd.reward_points,
 cd.ends_at
FROM user_challenge_progress ucp
JOIN challenge_definitions cd ON cd.id = ucp.challenge_id
WHERE ucp.user_id = @user_id
ORDER BY ucp.completed ASC, cd.ends_at ASC NULLS FIRST;

-- name: GetChallengeProgress :one
SELECT * FROM user_challenge_progress
WHERE user_id = @user_id AND challenge_id = @challenge_id;

-- name: MarkChallengeRewardClaimed :exec
UPDATE user_challenge_progress
SET reward_claimed = true
WHERE id = @id;

-- =====================================================================
-- Leaderboard — computed from wallet_ledger. The all_time leaderboard
-- sums positive 'points' earns per user (lifetime_points semantics per
-- Section 28 — never decreases). The period leaderboard aggregates the
-- same ledger filtered by created_at for weekly/monthly windows.
-- NOTE: wallet_ledger.operation values are earn|spend|purchase|convert|
-- refund|clawback|admin_adjust — there is no 'credit' operation. Lifetime
-- points = SUM(amount) over positive entries, matching GetLifetimePoints.
-- space_id is nullable: when set, only members of that space rank
-- (per-space scope, Section 64).
-- =====================================================================

-- name: LeaderboardAllTime :many
SELECT
 wl.user_id,
 COALESCE(SUM(wl.amount) FILTER (WHERE wl.amount > 0), 0)::bigint AS lifetime_points,
 COALESCE(u.public_username, u.username, '') AS username,
 up.display_name,
 up.avatar_url,
 u.profile_type
FROM wallet_ledger wl
JOIN users u ON u.id = wl.user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE wl.ledger_type = 'points'
 AND u.is_bot = false
 AND u.is_synthetic = false
 AND u.status = 'active'
 AND (sqlc.narg(space_id)::uuid IS NULL OR EXISTS (
 SELECT 1 FROM space_members sm
 WHERE sm.space_id = sqlc.narg(space_id)::uuid
 AND sm.user_id = wl.user_id))
GROUP BY wl.user_id, u.username, up.display_name, up.avatar_url, u.profile_type
ORDER BY lifetime_points DESC
LIMIT @top_n;

-- name: LeaderboardByPeriod :many
SELECT
 wl.user_id,
 COALESCE(SUM(wl.amount) FILTER (WHERE wl.amount > 0), 0)::bigint AS period_points,
 COALESCE(u.public_username, u.username, '') AS username,
 up.display_name,
 up.avatar_url,
 u.profile_type
FROM wallet_ledger wl
JOIN users u ON u.id = wl.user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE wl.ledger_type = 'points'
 AND wl.created_at >= @from_date
 AND u.is_bot = false
 AND u.is_synthetic = false
 AND u.status = 'active'
 AND (sqlc.narg(space_id)::uuid IS NULL OR EXISTS (
 SELECT 1 FROM space_members sm
 WHERE sm.space_id = sqlc.narg(space_id)::uuid
 AND sm.user_id = wl.user_id))
GROUP BY wl.user_id, u.username, up.display_name, up.avatar_url, u.profile_type
ORDER BY period_points DESC
LIMIT @top_n;

-- =====================================================================
-- Badge evaluation helpers — used by wallet:process_activity_event task.
-- =====================================================================

-- name: GetBadgesByActivityTrigger :many
SELECT * FROM badge_definitions
WHERE is_active = true
 AND trigger_type = 'activity_count'
 AND trigger_activity_key = @activity_key;

-- name: GetBadgesByPointsTrigger :many
SELECT * FROM badge_definitions
WHERE is_active = true
 AND trigger_type = 'lifetime_points';

-- name: CountUserActivityByKey :one
-- activity_events is the canonical record of "user did activity X" — the
-- wallet activity queue, deduped by UNIQUE(activity_key,
-- reference). platform_events is the analytics event stream (page_view etc.)
-- and never receives activity keys, so counting it always returned 0.
SELECT COUNT(*) AS count
FROM activity_events
WHERE user_id = @user_id
 AND activity_key = @activity_key;

-- name: GetChallengesByActivityKey :many
SELECT * FROM challenge_definitions
WHERE is_active = true
 AND activity_key = @activity_key
 AND starts_at <= now()
 AND (ends_at IS NULL OR ends_at > now());
