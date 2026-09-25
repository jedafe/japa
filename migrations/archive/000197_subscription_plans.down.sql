-- Remove the subscription_plans table and its permission node.

DELETE FROM role_permissions
WHERE permission_id = (SELECT id FROM permissions WHERE key = 'payments.plans.manage');

DELETE FROM permissions WHERE key = 'payments.plans.manage';

DROP TABLE IF EXISTS subscription_plans;
