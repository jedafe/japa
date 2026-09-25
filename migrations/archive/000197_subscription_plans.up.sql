-- 000197_subscription_plans.up.sql
-- Adds the subscription_plans table so plan types, contents, and prices can be
-- managed via the admin panel instead of being hardcoded in the Go handler.
-- Existing plans (Free, Pro, Business) are seeded from the current defaults.

CREATE TABLE subscription_plans (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id),
    slug            text NOT NULL,
    name            text NOT NULL,
    description     text NOT NULL DEFAULT '',
    features        jsonb NOT NULL DEFAULT '[]'::jsonb,
    monthly_amount  bigint NOT NULL DEFAULT 0,
    yearly_amount   bigint NOT NULL DEFAULT 0,
    currency        text NOT NULL DEFAULT 'NGN',
    highlighted     boolean NOT NULL DEFAULT false,
    sort_order      integer NOT NULL DEFAULT 0,
    status          text NOT NULL DEFAULT 'active',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_subscription_plans_tenant_slug ON subscription_plans(tenant_id, slug);
CREATE INDEX idx_subscription_plans_tenant_sort ON subscription_plans(tenant_id, sort_order);
CREATE INDEX idx_subscription_plans_tenant_status ON subscription_plans(tenant_id, status);

-- Permission node for managing subscription plans.
INSERT INTO permissions (key, description) VALUES
    ('payments.plans.manage', 'Manage subscription plans')
ON CONFLICT (key) DO NOTHING;

-- Grant super_admin the new node.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key = 'payments.plans.manage'
ON CONFLICT DO NOTHING;

-- Seed the three existing plans so the public /pricing/plans endpoint and the
-- admin panel have data from the start. Feature lists match the prior hardcoded
-- values in internal/payments/handlers.go ListPricingPlans.
INSERT INTO subscription_plans (tenant_id, slug, name, description, features, monthly_amount, yearly_amount, currency, highlighted, sort_order, status) VALUES
    (
        '00000000-0000-0000-0000-000000000001',
        'free',
        'Free',
        'Get started with core community features',
        '[{"text":"Community forum access","included":true},{"text":"Basic profile","included":true},{"text":"5 posts per month","included":true},{"text":"Public spaces","included":true},{"text":"AI assistant (10 queries/day)","included":true},{"text":"Priority support","included":false},{"text":"Advanced analytics","included":false},{"text":"Custom branding","included":false}]'::jsonb,
        0,
        0,
        'NGN',
        false,
        0,
        'active'
    ),
    (
        '00000000-0000-0000-0000-000000000001',
        'pro',
        'Pro',
        'For professionals building their diaspora network',
        '[{"text":"Everything in Free","included":true},{"text":"Unlimited posts","included":true},{"text":"Professional profile","included":true},{"text":"AI assistant (100 queries/day)","included":true},{"text":"Priority support","included":true},{"text":"Advanced analytics","included":true},{"text":"Document verification","included":true},{"text":"Custom branding","included":false}]'::jsonb,
        500000,
        5000000,
        'NGN',
        true,
        1,
        'active'
    ),
    (
        '00000000-0000-0000-0000-000000000001',
        'business',
        'Business',
        'For organisations and power users',
        '[{"text":"Everything in Pro","included":true},{"text":"Unlimited AI queries","included":true},{"text":"Team management","included":true},{"text":"API access","included":true},{"text":"Dedicated support","included":true},{"text":"Custom integrations","included":true},{"text":"Advanced analytics","included":true},{"text":"Custom branding","included":true}]'::jsonb,
        1500000,
        15000000,
        'NGN',
        false,
        2,
        'active'
    );
