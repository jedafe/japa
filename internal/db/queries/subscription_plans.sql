-- name: ListSubscriptionPlans :many
SELECT * FROM subscription_plans

ORDER BY sort_order ASC, created_at ASC;

-- name: ListActiveSubscriptionPlans :many
SELECT * FROM subscription_plans
WHERE status = 'active'
ORDER BY sort_order ASC, created_at ASC;

-- name: GetSubscriptionPlanByID :one
SELECT * FROM subscription_plans
WHERE id = $1;

-- name: GetSubscriptionPlanBySlug :one
SELECT * FROM subscription_plans
WHERE slug = $1;

-- name: CreateSubscriptionPlan :one
INSERT INTO subscription_plans (slug, name, description, features, monthly_amount, yearly_amount, currency, highlighted, sort_order, status, tier) VALUES (@slug, @name, @description, @features, @monthly_amount, @yearly_amount, @currency, @highlighted, @sort_order, @status, NULLIF(@tier, ''))
RETURNING *;

-- name: UpdateSubscriptionPlan :one
UPDATE subscription_plans
SET
 slug = COALESCE(NULLIF(@slug, ''), slug),
 name = COALESCE(NULLIF(@name, ''), name),
 description = @description,
 features = @features,
 monthly_amount = @monthly_amount,
 yearly_amount = @yearly_amount,
 currency = COALESCE(NULLIF(@currency, ''), currency),
 highlighted = @highlighted,
 sort_order = @sort_order,
 status = COALESCE(NULLIF(@status, ''), status),
 tier = COALESCE(NULLIF(@tier, ''), tier),
 updated_at = now()
WHERE id = @id
RETURNING *;

-- name: DeleteSubscriptionPlan :exec
DELETE FROM subscription_plans
WHERE id = $1;

-- ─── Subscription Plan ↔ Track junction (migration 000273) ───────────────

-- name: AddTrackToPlan :exec
INSERT INTO subscription_plan_tracks (plan_id, track_id) VALUES (@plan_id, @track_id)

ON CONFLICT (plan_id, track_id) DO NOTHING;

-- name: RemoveTrackFromPlan :exec
DELETE FROM subscription_plan_tracks
WHERE plan_id = @plan_id AND track_id = @track_id;

-- name: RemoveAllTracksFromPlan :exec
DELETE FROM subscription_plan_tracks
WHERE plan_id = @plan_id;

-- name: ListTrackIDsForPlan :many
SELECT track_id FROM subscription_plan_tracks
WHERE plan_id = $1
ORDER BY created_at ASC;

-- name: CountTracksForPlan :one
SELECT count(*) FROM subscription_plan_tracks
WHERE plan_id = $1;

-- name: IsTrackInPlan :one
SELECT 1 FROM subscription_plan_tracks
WHERE plan_id = $1 AND track_id = $2
LIMIT 1;
