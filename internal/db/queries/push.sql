-- Push notification subscriptions and tokens (Section 29).

-- name: ListPushSubscriptionsByUser :many
SELECT id, endpoint, p256dh, auth_key
FROM push_subscriptions
WHERE user_id = @user_id;

-- name: GetPushSubscriptionByID :one
SELECT id, user_id, endpoint, p256dh, auth_key
FROM push_subscriptions
WHERE id = @id;

-- name: SavePushSubscription :exec
INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth_key)
VALUES (@user_id, @endpoint, @p256dh, @auth_key)
ON CONFLICT (user_id, endpoint) DO UPDATE
SET p256dh = EXCLUDED.p256dh, auth_key = EXCLUDED.auth_key;

-- name: DeletePushSubscription :exec
DELETE FROM push_subscriptions WHERE id = @id;

-- name: DeletePushSubscriptionByEndpoint :exec
DELETE FROM push_subscriptions WHERE user_id = @user_id AND endpoint = @endpoint;

-- name: PruneStalePushTokens :exec
DELETE FROM push_tokens
WHERE last_used_at < now() - interval '90 days'
   OR (last_used_at IS NULL AND created_at < now() - interval '30 days');
