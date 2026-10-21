-- internal/db/queries/next_actions.sql — §122 Next-Action Suggestion Engine.

-- name: ListEnabledNextActionRules :many
SELECT id, slug, name, description, enabled, priority, audience, conditions,
       payload, channels, completion_signal, frequency, valid_from, valid_until,
       created_by, created_at, updated_at, upsell
FROM next_action_rules
WHERE enabled = true
  AND (valid_from IS NULL OR valid_from <= now())
  AND (valid_until IS NULL OR valid_until > now())
ORDER BY priority ASC;

-- name: ListNextActionRules :many
SELECT id, slug, name, description, enabled, priority, audience, conditions,
       payload, channels, completion_signal, frequency, valid_from, valid_until,
       created_by, created_at, updated_at, upsell
FROM next_action_rules
ORDER BY priority ASC, created_at DESC;

-- name: GetNextActionRule :one
SELECT id, slug, name, description, enabled, priority, audience, conditions,
       payload, channels, completion_signal, frequency, valid_from, valid_until,
       created_by, created_at, updated_at, upsell
FROM next_action_rules
WHERE id = @id;

-- name: GetNextActionRuleBySlug :one
SELECT id, slug, name, description, enabled, priority, audience, conditions,
       payload, channels, completion_signal, frequency, valid_from, valid_until,
       created_by, created_at, updated_at, upsell
FROM next_action_rules
WHERE slug = @slug;

-- name: CreateNextActionRule :one
INSERT INTO next_action_rules
    (slug, name, description, enabled, priority, upsell, audience, conditions, payload,
     channels, completion_signal, frequency, valid_from, valid_until, created_by)
VALUES
    (@slug, @name, @description, @enabled, @priority, @upsell, @audience, @conditions,
     @payload, @channels, @completion_signal, @frequency, @valid_from,
     @valid_until, @created_by)
RETURNING id, slug, name, description, enabled, priority, audience, conditions,
       payload, channels, completion_signal, frequency, valid_from, valid_until,
       created_by, created_at, updated_at, upsell;

-- name: UpdateNextActionRule :one
UPDATE next_action_rules SET
    name = @name, description = @description, enabled = @enabled,
    priority = @priority, upsell = @upsell, audience = @audience, conditions = @conditions,
    payload = @payload, channels = @channels,
    completion_signal = @completion_signal, frequency = @frequency,
    valid_from = @valid_from, valid_until = @valid_until, updated_at = now()
WHERE id = @id
RETURNING id, slug, name, description, enabled, priority, audience, conditions,
       payload, channels, completion_signal, frequency, valid_from, valid_until,
       created_by, created_at, updated_at, upsell;

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
SET status = CASE WHEN status IN ('pending','snoozed') THEN 'shown' ELSE status END,
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
-- Keyset pagination on user_id — stable cursor, no skip/dup when events
-- arrive mid-sweep (unlike OFFSET).
SELECT DISTINCT user_id FROM platform_events
WHERE user_id IS NOT NULL
  AND user_id > @after_id
  AND created_at > now() - make_interval(days => @days)
ORDER BY user_id ASC
LIMIT @batch_size;

-- name: MergeGuestNextAction :exec
-- Guest→user state merge (§122): impressions sum; terminal statuses win over
-- non-terminal on either side; snooze keeps the latest snoozed_until.
INSERT INTO user_next_actions
    (user_id, rule_id, status, impressions, first_shown_at, last_shown_at, snoozed_until)
VALUES
    (@user_id, @rule_id, @status, @impressions, @last_shown_at, @last_shown_at, @snoozed_until)
ON CONFLICT (user_id, rule_id) DO UPDATE SET
    status = CASE
        WHEN user_next_actions.status IN ('dismissed','completed','expired') THEN user_next_actions.status
        WHEN EXCLUDED.status IN ('dismissed','completed','expired') THEN EXCLUDED.status
        WHEN user_next_actions.status = 'snoozed' AND user_next_actions.snoozed_until > now() THEN user_next_actions.status
        ELSE EXCLUDED.status END,
    impressions = user_next_actions.impressions + EXCLUDED.impressions,
    snoozed_until = GREATEST(
        COALESCE(user_next_actions.snoozed_until, '-infinity'::timestamptz),
        COALESCE(EXCLUDED.snoozed_until, '-infinity'::timestamptz)),
    last_shown_at = GREATEST(
        COALESCE(user_next_actions.last_shown_at, '-infinity'::timestamptz),
        COALESCE(EXCLUDED.last_shown_at, '-infinity'::timestamptz)),
    updated_at = now();

-- name: HasRecentNextActionShown :one
-- Dedupes client track('shown') against the server-side impression emission
-- (§122): true when a shown event for this identity+rule exists in @seconds.
SELECT EXISTS(
    SELECT 1 FROM platform_events
    WHERE event_type = 'next_action.shown'
      AND meta->>'rule_slug' = @rule_slug::text
      AND ((@user_id::uuid IS NOT NULL AND user_id = @user_id)
        OR (@session_id::text <> '' AND session_id = @session_id))
      AND created_at > now() - make_interval(secs => @seconds)
) AS has_recent;

-- name: CountUpsellShownToday :one
-- Upsell rules carry the explicit next_action_rules.upsell flag (migration
-- 000921 — replaces the completion_signal-predicate heuristic). Counts
-- actual impressions from user_next_actions — server-authoritative, no
-- dependency on client track calls.
SELECT count(*) FROM user_next_actions una
JOIN next_action_rules nar ON nar.id = una.rule_id
WHERE una.user_id = @user_id
  AND una.last_shown_at > now() - interval '1 day'
  AND nar.upsell;

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

-- name: ExpireStaleUserNextActions :execrows
-- Writes the 'expired' lifecycle state (§122): active rows whose rule passed
-- its valid_until or was disabled/deleted are terminal — they stop counting
-- toward stats and can no longer resurface. Runs at the end of each sweep.
UPDATE user_next_actions
SET status = 'expired', updated_at = now()
WHERE status IN ('pending','shown','snoozed')
  AND NOT EXISTS (
      SELECT 1 FROM next_action_rules nar
      WHERE nar.id = user_next_actions.rule_id
        AND nar.enabled
        AND (nar.valid_until IS NULL OR nar.valid_until > now())
  );

-- name: BackfillGuestSessionEvents :execrows
-- Guest-platform_events rows carry the jc_guest session key but no user_id.
-- At register/login we attribute them to the new user so user-scoped
-- predicates (tool_used, event_count, feature_gate_hit) see pre-auth history.
UPDATE platform_events
SET user_id = @user_id
WHERE session_id = @session_id AND user_id IS NULL;

-- name: HasUserNextAction :one
SELECT EXISTS(
    SELECT 1 FROM user_next_actions
    WHERE user_id = @user_id AND rule_id = @rule_id
);
