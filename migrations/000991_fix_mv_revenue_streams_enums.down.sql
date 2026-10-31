-- Restore the pre-fix mv_revenue_streams definition.
DROP MATERIALIZED VIEW IF EXISTS mv_revenue_streams;

CREATE MATERIALIZED VIEW mv_revenue_streams AS
SELECT date_trunc('month', created_at)::date AS month,
    sum(amount) FILTER (WHERE purpose_type = 'pro_subscription') AS subscription_revenue,
    sum(amount) FILTER (WHERE purpose_type = 'course_access') AS course_revenue,
    sum(amount) FILTER (WHERE purpose_type = 'ad_campaign') AS ad_revenue,
    sum(amount) FILTER (WHERE purpose_type = 'credit_pack') AS credit_revenue
FROM payments
WHERE status = 'completed'
GROUP BY date_trunc('month', created_at)::date;

CREATE UNIQUE INDEX idx_mv_revenue_streams ON mv_revenue_streams (month);
