-- Fix mv_revenue_streams (Section 66, ADR-084): the original definition
-- filtered payments.purpose_type/status values that do not exist in the
-- payments CHECK constraints ('pro_subscription', 'ad_campaign',
-- 'credit_pack', 'completed'), so the view always returned zero rows.
-- Recreate with the enum values the constraint actually allows
-- (pro_membership / course_access / ad_slot / credit_purchase / succeeded).
DROP MATERIALIZED VIEW IF EXISTS mv_revenue_streams;

CREATE MATERIALIZED VIEW mv_revenue_streams AS
SELECT date_trunc('month', created_at)::date AS month,
    sum(amount) FILTER (WHERE purpose_type = 'pro_membership') AS subscription_revenue,
    sum(amount) FILTER (WHERE purpose_type = 'course_access') AS course_revenue,
    sum(amount) FILTER (WHERE purpose_type = 'ad_slot') AS ad_revenue,
    sum(amount) FILTER (WHERE purpose_type = 'credit_purchase') AS credit_revenue
FROM payments
WHERE status = 'succeeded'
GROUP BY date_trunc('month', created_at)::date;

CREATE UNIQUE INDEX idx_mv_revenue_streams ON mv_revenue_streams (month);

-- Populate immediately so the endpoint does not 500 until the next
-- analytics:refresh_views run.
REFRESH MATERIALIZED VIEW mv_revenue_streams;
