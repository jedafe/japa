-- internal/db/queries/next_actions.sql — §122 Next-Action Suggestion Engine.

-- name: ListEnabledNextActionRules :many
SELECT id, slug, name, description, enabled, priority, audience, conditions,
       payload, channels, completion_signal, frequency, valid_from, valid_until,
       created_by, created_at, updated_at
FROM next_action_rules
WHERE enabled = true
  AND (valid_from IS NULL OR valid_from <= now())
  AND (valid_until IS NULL OR valid_until > now())
ORDER BY priority ASC;

-- name: ListNextActionRules :many
SELECT id, slug, name, description, enabled, priority, audience, conditions,
       payload, channels, completion_signal, frequency, valid_from, valid_until,
       created_by, created_at, updated_at
FROM next_action_rules
ORDER BY priority ASC, created_at DESC;

-- name: GetNextActionRule :one
SELECT id, slug, name, description, enabled, priority, audience, conditions,
       payload, channels, completion_signal, frequency, valid_from, valid_until,
       created_by, created_at, updated_at
FROM next_action_rules
WHERE id = @id;

-- name: GetNextActionRuleBySlug :one
SELECT id, slug, name, description, enabled, priority, audience, conditions,
       payload, channels, completion_signal, frequency, valid_from, valid_until,
       created_by, created_at, updated_at
FROM next_action_rules
WHERE slug = @slug;

-- name: CreateNextActionRule :one
INSERT INTO next_action_rules
    (slug, name, description, enabled, priority, audience, conditions, payload,
     channels, completion_signal, frequency, valid_from, valid_until, created_by)
VALUES
    (@slug, @name, @description, @enabled, @priority, @audience, @conditions,
     @payload, @channels, @completion_signal, @frequency, @valid_from,
     @valid_until, @created_by)
RETURNING id, slug, name, description, enabled, priority, audience, conditions,
       payload, channels, completion_signal, frequency, valid_from, valid_until,
       created_by, created_at, updated_at;

-- name: UpdateNextActionRule :one
UPDATE next_action_rules SET
    name = @name, description = @description, enabled = @enabled,
    priority = @priority, audience = @audience, conditions = @conditions,
    payload = @payload, channels = @channels,
    completion_signal = @completion_signal, frequency = @frequency,
    valid_from = @valid_from, valid_until = @valid_until, updated_at = now()
WHERE id = @id
RETURNING id, slug, name, description, enabled, priority, audience, conditions,
       payload, channels, completion_signal, frequency, valid_from, valid_until,
       created_by, created_at, updated_at;

-- name: DeleteNextActionRule :exec
DELETE FROM next_action_rules WHERE id = @id;

-- name: GetUserNextActions :many
SELECT id, user_id, rule_id, status, impressions,
       first_shown_at, last_shown_at, snoozed_until,
       dismissed_at, completed_at
FROM user_next_actions
WHERE user_id = @user_id;

-- name: UpsertUserNextAction :exec
INSERT INTO user_next_actions (user_id, rule_id, status)
VALUES (@user_id, @rule_id, 'pending')
ON CONFLICT (user_id, rule_id) DO NOTHING;

-- name: MarkNextActionShown :exec
UPDATE user_next_actions
SET status = CASE WHEN status = 'pending' THEN 'shown' ELSE status END,
    impressions = impressions + 1,
    first_shown_at = COALESCE(first_shown_at, now()),
    last_shown_at = now(),
    updated_at = now()
WHERE user_id = @user_id AND rule_id = @rule_id;

-- name: DismissNextAction :exec
UPDATE user_next_actions
SET status = 'dismissed', dismissed_at = now(), updated_at = now()
WHERE user_id = @user_id AND rule_id = @rule_id;

-- name: SnoozeNextAction :exec
UPDATE user_next_actions
SET status = 'snoozed', snoozed_until = now() + make_interval(hours => @hours),
    updated_at = now()
WHERE user_id = @user_id AND rule_id = @rule_id;

-- name: CompleteNextAction :exec
UPDATE user_next_actions
SET status = 'completed', completed_at = now(), updated_at = now()
WHERE user_id = @user_id AND rule_id = @rule_id;

-- name: CompleteNextActionByRuleSlug :exec
UPDATE user_next_actions
SET status = 'completed', completed_at = now(), updated_at = now()
WHERE user_id = @user_id
  AND rule_id = (SELECT id FROM next_action_rules WHERE slug = @slug);

-- name: CountUserEvents :one
SELECT count(*) FROM platform_events
WHERE user_id = @user_id AND event_type = @event_type
  AND created_at > now() - make_interval(days => @days);

-- name: CountSessionEvents :one
SELECT count(*) FROM platform_events
WHERE session_id = @session_id AND event_type = @event_type
  AND created_at > now() - make_interval(days => @days);

-- name: CountUserToolEvents :one
SELECT count(*) FROM platform_events
WHERE user_id = @user_id AND event_type = 'tool.used'
  AND (@tool_slug::text = '*' OR meta->>'tool' = @tool_slug)
  AND created_at > now() - make_interval(days => @days);

-- name: CountSessionToolEvents :one
SELECT count(*) FROM platform_events
WHERE session_id = @session_id AND event_type = 'tool.used'
  AND (@tool_slug::text = '*' OR meta->>'tool' = @tool_slug)
  AND created_at > now() - make_interval(days => @days);

-- name: CountUserGateHitEvents :one
SELECT count(*) FROM platform_events
WHERE user_id = @user_id AND event_type = 'feature.gate_hit'
  AND (@feature::text = '' OR meta->>'feature' = @feature)
  AND created_at > now() - make_interval(days => @days);

-- name: CountSessionGateHitEvents :one
SELECT count(*) FROM platform_events
WHERE session_id = @session_id AND event_type = 'feature.gate_hit'
  AND (@feature::text = '' OR meta->>'feature' = @feature)
  AND created_at > now() - make_interval(days => @days);

-- name: UserHasAssessmentResult :one
SELECT EXISTS(
    SELECT 1 FROM assessment_results WHERE user_id = @user_id
) AS has_result;

-- name: GetLatestAssessmentScore :one
SELECT total_score FROM assessment_results
WHERE user_id = @user_id
ORDER BY created_at DESC LIMIT 1;

-- name: GetUserForNextActionState :one
SELECT u.id, u.created_at, u.email_verified_at,
       p.display_name, p.avatar_url, p.bio
FROM users u
LEFT JOIN user_profiles p ON p.user_id = u.id
WHERE u.id = @user_id;

-- name: UserHasPushSubscription :one
SELECT EXISTS(
    SELECT 1 FROM push_subscriptions WHERE user_id = @user_id
) AS has_sub;

-- name: ListActiveUsersForSweep :many
SELECT DISTINCT user_id FROM platform_events
WHERE user_id IS NOT NULL
  AND created_at > now() - make_interval(days => @days)
LIMIT @batch_size OFFSET @batch_offset;

-- name: CountUpsellShownToday :one
-- Upsell rules are identified by a completion_signal containing an
-- account_tier or user_class predicate (the §122 upsell convention). Counts
-- actual impressions from user_next_actions — server-authoritative, no
-- dependency on client track calls.
SELECT count(*) FROM user_next_actions una
JOIN next_action_rules nar ON nar.id = una.rule_id
WHERE una.user_id = @user_id
  AND una.last_shown_at > now() - interval '1 day'
  AND nar.completion_signal::text ~ '"predicate"\s*:\s*"(account_tier|user_class)"';

-- name: NextActionStats :many
SELECT nar.slug,
       count(*) FILTER (WHERE pe.event_type = 'next_action.shown')     AS shown,
       count(*) FILTER (WHERE pe.event_type = 'next_action.clicked')   AS clicked,
       count(*) FILTER (WHERE pe.event_type = 'next_action.dismissed') AS dismissed,
       count(*) FILTER (WHERE pe.event_type = 'next_action.completed') AS completed
FROM next_action_rules nar
LEFT JOIN platform_events pe
       ON pe.meta->>'rule_slug' = nar.slug
      AND pe.event_type LIKE 'next_action.%'
GROUP BY nar.slug
ORDER BY nar.slug;
