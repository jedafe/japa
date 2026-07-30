-- name: ListSubscriptionPlans :many
SELECT * FROM subscription_plans
WHERE tenant_id = $1
ORDER BY sort_order ASC, created_at ASC;

-- name: ListActiveSubscriptionPlans :many
SELECT * FROM subscription_plans
WHERE tenant_id = $1 AND status = 'active'
ORDER BY sort_order ASC, created_at ASC;

-- name: GetSubscriptionPlanByID :one
SELECT * FROM subscription_plans
WHERE id = $1 AND tenant_id = $2;

-- name: GetSubscriptionPlanBySlug :one
SELECT * FROM subscription_plans
WHERE tenant_id = $1 AND slug = $2;

-- name: CreateSubscriptionPlan :one
INSERT INTO subscription_plans (
    tenant_id, slug, name, description, features,
    monthly_amount, yearly_amount, currency,
    highlighted, sort_order, status
) VALUES (
    @tenant_id, @slug, @name, @description, @features,
    @monthly_amount, @yearly_amount, @currency,
    @highlighted, @sort_order, @status
)
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
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: DeleteSubscriptionPlan :exec
DELETE FROM subscription_plans
WHERE id = $1 AND tenant_id = $2;
