-- Push notification subscriptions and tokens (Section 29).

-- name: ListPushSubscriptionsByUser :many
SELECT id, endpoint, p256dh, auth_key
FROM push_subscriptions
WHERE user_id = @user_id;

-- name: DeletePushSubscription :exec
DELETE FROM push_subscriptions WHERE id = @id;

-- name: PruneStalePushTokens :exec
DELETE FROM push_tokens
WHERE last_used_at < now() - interval '90 days'
   OR (last_used_at IS NULL AND created_at < now() - interval '30 days');
