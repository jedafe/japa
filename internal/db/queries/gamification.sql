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
INSERT INTO badge_definitions (slug, name, description, icon_media_id, trigger_type, trigger_value, trigger_activity_key, is_active, is_system, display_order, category, rarity, tier, badge_group, is_secret, is_limited, is_featured, points_reward, available_from, available_until) VALUES (@slug, @name, sqlc.narg(description), sqlc.narg(icon_media_id), @trigger_type, sqlc.narg(trigger_value), sqlc.narg(trigger_activity_key), @is_active, @is_system, @display_order, sqlc.narg(category), @rarity, sqlc.narg(tier), sqlc.narg(badge_group), @is_secret, @is_limited, @is_featured, @points_reward, sqlc.narg(available_from), sqlc.narg(available_until))

RETURNING *;

-- name: UpdateBadgeDefinition :one
UPDATE badge_definitions
SET
 name = COALESCE(sqlc.narg(name), name),
 description = COALESCE(sqlc.narg(description), description),
 icon_media_id = CASE WHEN sqlc.narg(clear_icon_media_id)::boolean THEN NULL ELSE COALESCE(sqlc.narg(icon_media_id), icon_media_id) END,
 trigger_type = COALESCE(sqlc.narg(trigger_type), trigger_type),
 trigger_value = CASE WHEN sqlc.narg(clear_trigger_value)::boolean THEN NULL ELSE COALESCE(sqlc.narg(trigger_value), trigger_value) END,
 trigger_activity_key = CASE WHEN sqlc.narg(clear_trigger_activity_key)::boolean THEN NULL ELSE COALESCE(sqlc.narg(trigger_activity_key), trigger_activity_key) END,
 is_active = COALESCE(sqlc.narg(is_active), is_active),
 display_order = COALESCE(sqlc.narg(display_order), display_order),
 category = CASE WHEN sqlc.narg(clear_category)::boolean THEN NULL ELSE COALESCE(sqlc.narg(category), category) END,
 rarity = COALESCE(sqlc.narg(rarity), rarity),
 tier = CASE WHEN sqlc.narg(clear_tier)::boolean THEN NULL ELSE COALESCE(sqlc.narg(tier), tier) END,
 badge_group = CASE WHEN sqlc.narg(clear_badge_group)::boolean THEN NULL ELSE COALESCE(sqlc.narg(badge_group), badge_group) END,
 is_secret = COALESCE(sqlc.narg(is_secret), is_secret),
 is_limited = COALESCE(sqlc.narg(is_limited), is_limited),
 is_featured = COALESCE(sqlc.narg(is_featured), is_featured),
 points_reward = COALESCE(sqlc.narg(points_reward), points_reward),
 available_from = CASE WHEN sqlc.narg(clear_available_from)::boolean THEN NULL ELSE COALESCE(sqlc.narg(available_from), available_from) END,
 available_until = CASE WHEN sqlc.narg(clear_available_until)::boolean THEN NULL ELSE COALESCE(sqlc.narg(available_until), available_until) END
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
 bd.icon_media_id AS badge_icon_media_id,
 bd.category AS badge_category,
 bd.rarity AS badge_rarity,
 bd.tier AS badge_tier,
 bd.badge_group AS badge_group
FROM user_badges ub
JOIN badge_definitions bd ON bd.id = ub.badge_id
WHERE ub.user_id = @user_id
ORDER BY ub.awarded_at DESC;

-- name: ListUserBadgesByUsername :many
SELECT
 ub.*,
 bd.name AS badge_name,
 bd.slug AS badge_slug,
 bd.description AS badge_description,
 bd.category AS badge_category,
 bd.rarity AS badge_rarity,
 bd.tier AS badge_tier,
 bd.badge_group AS badge_group
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
 image_media_id = CASE WHEN sqlc.narg(clear_image_media_id)::boolean THEN NULL ELSE COALESCE(sqlc.narg(image_media_id), image_media_id) END,
 activity_key = COALESCE(sqlc.narg(activity_key), activity_key),
 target_count = COALESCE(sqlc.narg(target_count), target_count),
 reward_points = COALESCE(sqlc.narg(reward_points), reward_points),
 reward_badge_id = CASE WHEN sqlc.narg(clear_reward_badge_id)::boolean THEN NULL ELSE COALESCE(sqlc.narg(reward_badge_id), reward_badge_id) END,
 starts_at = COALESCE(sqlc.narg(starts_at), starts_at),
 ends_at = CASE WHEN sqlc.narg(clear_ends_at)::boolean THEN NULL ELSE COALESCE(sqlc.narg(ends_at), ends_at) END,
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
-- Leaderboard — computed from wallet_ledger. Ranks on the current points
-- balance (SUM of all signed entries) so penalty entries (negative
-- point_value rules: spam, downvotes, upheld reports) demote users —
-- ADR-136. The period leaderboard aggregates the same ledger filtered by
-- created_at for weekly/monthly windows. lifetime_points (positive earns
-- only, GetLifetimePoints) remains the badge-threshold metric.
-- space_id is nullable: when set, only members of that space rank
-- (per-space scope, Section 64).
-- =====================================================================

-- name: LeaderboardAllTime :many
SELECT
 wl.user_id,
 COALESCE(SUM(wl.amount), 0)::bigint AS points_balance,
 COALESCE(u.public_username, u.username, '') AS username,
 up.display_name,
 up.avatar_url,
 u.profile_type
FROM wallet_ledger wl
JOIN users u ON u.id = wl.user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
LEFT JOIN user_privacy_settings ups ON ups.user_id = u.id
WHERE wl.ledger_type = 'points'
 AND u.is_bot = false
 AND u.is_synthetic = false
 AND u.status = 'active'
 -- §28 consent model: show_points_publicly is opt-out (migration 000978 —
 -- DEFAULT true), so users who disabled it are excluded from ranking;
 -- a missing privacy-settings row counts as consenting.
 AND COALESCE(ups.show_points_publicly, true) = true
 AND (sqlc.narg(space_id)::uuid IS NULL OR EXISTS (
 SELECT 1 FROM space_members sm
 WHERE sm.space_id = sqlc.narg(space_id)::uuid
 AND sm.user_id = wl.user_id))
GROUP BY wl.user_id, u.username, u.public_username, up.display_name, up.avatar_url, u.profile_type
ORDER BY points_balance DESC
LIMIT @top_n;

-- name: LeaderboardByPeriod :many
SELECT
 wl.user_id,
 COALESCE(SUM(wl.amount), 0)::bigint AS period_points,
 COALESCE(u.public_username, u.username, '') AS username,
 up.display_name,
 up.avatar_url,
 u.profile_type
FROM wallet_ledger wl
JOIN users u ON u.id = wl.user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
LEFT JOIN user_privacy_settings ups ON ups.user_id = u.id
WHERE wl.ledger_type = 'points'
 AND wl.created_at >= @from_date
 AND u.is_bot = false
 AND u.is_synthetic = false
 AND u.status = 'active'
 AND COALESCE(ups.show_points_publicly, true) = true
 AND (sqlc.narg(space_id)::uuid IS NULL OR EXISTS (
 SELECT 1 FROM space_members sm
 WHERE sm.space_id = sqlc.narg(space_id)::uuid
 AND sm.user_id = wl.user_id))
GROUP BY wl.user_id, u.username, u.public_username, up.display_name, up.avatar_url, u.profile_type
ORDER BY period_points DESC
LIMIT @top_n;

-- =====================================================================
-- Badge evaluation helpers — used by wallet:process_activity_event task.
-- =====================================================================

-- name: GetBadgesByActivityTrigger :many
SELECT * FROM badge_definitions
WHERE is_active = true
 AND trigger_type = 'activity_count'
 AND trigger_activity_key = @activity_key
 AND (available_from IS NULL OR available_from <= now())
 AND (available_until IS NULL OR available_until > now());

-- name: GetBadgesByPointsTrigger :many
SELECT * FROM badge_definitions
WHERE is_active = true
 AND trigger_type = 'lifetime_points'
 AND (available_from IS NULL OR available_from <= now())
 AND (available_until IS NULL OR available_until > now());

-- name: GetBadgesByStreakTrigger :many
SELECT * FROM badge_definitions
WHERE is_active = true
 AND trigger_type = 'streak_days'
 AND trigger_activity_key = @activity_key
 AND (available_from IS NULL OR available_from <= now())
 AND (available_until IS NULL OR available_until > now());

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

-- =====================================================================
-- Streak evaluation — consecutive UTC days of a given activity_key ending
-- at the user's most recent active day (gamification v2, streak_days
-- trigger). Length of the latest unbroken run; a gap before the most
-- recent day resets it.
-- =====================================================================

-- name: GetActivityDayStreak :one
WITH days AS (
    SELECT DISTINCT (created_at AT TIME ZONE 'UTC')::date AS d
    FROM activity_events
    WHERE user_id = @user_id
      AND activity_key = @activity_key
),
runs AS (
    SELECT d, d - (ROW_NUMBER() OVER (ORDER BY d))::int AS grp
    FROM days
)
SELECT COUNT(*)::int AS streak
FROM runs
WHERE grp = (SELECT grp FROM runs ORDER BY d DESC LIMIT 1);

-- =====================================================================
-- Badge progress — per-user progress toward locked badges (v2).
-- =====================================================================

-- name: UpsertBadgeProgress :exec
INSERT INTO badge_progress (user_id, badge_id, current_count) VALUES (@user_id, @badge_id, @current_count)
ON CONFLICT (user_id, badge_id)
DO UPDATE SET current_count = EXCLUDED.current_count, updated_at = now();

-- =====================================================================
-- Badge award events — auditable award history (v2).
-- =====================================================================

-- name: InsertBadgeAwardEvent :one
INSERT INTO badge_award_events (user_id, badge_id, source, activity_event_id, awarded_by) VALUES (@user_id, @badge_id, @source, sqlc.narg(activity_event_id), sqlc.narg(awarded_by))
RETURNING *;

-- name: ListBadgeAwardEventsForUser :many
SELECT
 bae.*,
 bd.name AS badge_name,
 bd.slug AS badge_slug
FROM badge_award_events bae
JOIN badge_definitions bd ON bd.id = bae.badge_id
WHERE bae.user_id = @user_id
ORDER BY bae.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================================================================
-- Achievement centre collection — full catalogue joined with the user's
-- earned badges and progress rows in a single read (v2).
-- =====================================================================

-- name: ListBadgeCollectionForUser :many
SELECT
 bd.*,
 ub.awarded_at AS user_awarded_at,
 ub.id AS user_badge_id,
 bp.current_count AS progress_count
FROM badge_definitions bd
LEFT JOIN user_badges ub ON ub.badge_id = bd.id AND ub.user_id = @user_id
LEFT JOIN badge_progress bp ON bp.badge_id = bd.id AND bp.user_id = @user_id
WHERE bd.is_active = true
ORDER BY bd.display_order ASC, bd.name ASC;

-- name: ListBadgeTiers :many
SELECT * FROM badge_tiers
ORDER BY badge_group, threshold;

-- =====================================================================
-- Admin — award events, seasons, tiers (v2 management surfaces).
-- =====================================================================

-- name: ListAdminBadgeAwardEvents :many
SELECT
 bae.id,
 bae.user_id,
 u.username,
 bae.badge_id,
 bd.name AS badge_name,
 bd.slug AS badge_slug,
 bae.source,
 bae.activity_event_id,
 bae.awarded_by,
 au.username AS awarded_by_username,
 bae.created_at
FROM badge_award_events bae
JOIN badge_definitions bd ON bd.id = bae.badge_id
LEFT JOIN users u ON u.id = bae.user_id
LEFT JOIN users au ON au.id = bae.awarded_by
WHERE (@badge_id::uuid IS NULL OR bae.badge_id = @badge_id::uuid)
 AND (@q::text = '' OR u.username ILIKE '%' || @q::text || '%'
   OR bd.name ILIKE '%' || @q::text || '%'
   OR bae.source ILIKE '%' || @q::text || '%')
ORDER BY
 CASE WHEN @sort_dir::text = 'asc' THEN
   CASE @sort_by::text
     WHEN 'username' THEN u.username
     WHEN 'badge_name' THEN bd.name
     WHEN 'source' THEN bae.source
     ELSE NULL END
 END ASC NULLS LAST,
 CASE WHEN @sort_dir::text = 'asc' AND @sort_by::text = 'created_at' THEN bae.created_at END ASC NULLS LAST,
 CASE WHEN @sort_dir::text <> 'asc' THEN
   CASE @sort_by::text
     WHEN 'username' THEN u.username
     WHEN 'badge_name' THEN bd.name
     WHEN 'source' THEN bae.source
     ELSE NULL END
 END DESC NULLS LAST,
 CASE WHEN @sort_dir::text <> 'asc' AND @sort_by::text = 'created_at' THEN bae.created_at END DESC NULLS LAST,
 bae.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAdminBadgeAwardEvents :one
SELECT COUNT(*) FROM badge_award_events bae
JOIN badge_definitions bd ON bd.id = bae.badge_id
LEFT JOIN users u ON u.id = bae.user_id
WHERE (@badge_id::uuid IS NULL OR bae.badge_id = @badge_id::uuid)
 AND (@q::text = '' OR u.username ILIKE '%' || @q::text || '%'
   OR bd.name ILIKE '%' || @q::text || '%'
   OR bae.source ILIKE '%' || @q::text || '%');

-- name: ListBadgeSeasons :many
SELECT * FROM badge_seasons ORDER BY starts_at DESC;

-- name: InsertBadgeSeason :one
INSERT INTO badge_seasons (slug, name, description, starts_at, ends_at, is_active)
VALUES (@slug, @name, @description, @starts_at, @ends_at, @is_active)
RETURNING *;

-- name: UpdateBadgeSeason :one
UPDATE badge_seasons SET
 name = COALESCE(sqlc.narg(name), name),
 description = COALESCE(sqlc.narg(description), description),
 starts_at = COALESCE(sqlc.narg(starts_at), starts_at),
 ends_at = COALESCE(sqlc.narg(ends_at), ends_at),
 is_active = COALESCE(sqlc.narg(is_active), is_active)
WHERE id = @id
RETURNING *;

-- name: DeleteBadgeSeason :exec
DELETE FROM badge_seasons WHERE id = @id;

-- name: InsertBadgeTier :one
INSERT INTO badge_tiers (badge_group, tier, name, threshold, badge_id)
VALUES (@badge_group, @tier, @name, @threshold, @badge_id)
RETURNING *;

-- name: UpdateBadgeTier :one
UPDATE badge_tiers SET
 badge_group = COALESCE(sqlc.narg(badge_group), badge_group),
 tier = COALESCE(sqlc.narg(tier), tier),
 name = COALESCE(sqlc.narg(name), name),
 threshold = COALESCE(sqlc.narg(threshold), threshold),
 badge_id = CASE WHEN @clear_badge_id::boolean THEN NULL
            ELSE COALESCE(sqlc.narg(badge_id), badge_id) END
WHERE id = @id
RETURNING *;

-- name: DeleteBadgeTier :exec
DELETE FROM badge_tiers WHERE id = @id;

-- name: ListBadgeGroups :many
SELECT DISTINCT badge_group FROM badge_definitions
WHERE badge_group IS NOT NULL AND badge_group != ''
ORDER BY badge_group;
