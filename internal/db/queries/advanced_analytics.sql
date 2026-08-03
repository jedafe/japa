-- advanced_analytics.sql — Advanced Analytics Dashboard queries (Section 66, ADR-084).
-- Covers top content, revenue streams, ad performance, newsletter engagement,
-- and retention cohorts.
-- Ref: japaconnect-engineering/sections/66-advanced-analytics.md
--      japaconnect-engineering/adr/ADR-084-advanced-analytics.md

-- =====================================================================
-- Top content by engagement score (from mv_top_content_7d).
-- =====================================================================

-- name: AdvancedAnalyticsTopContent :many
SELECT
    tc.content_id,
    tc.tenant_id,
    tc.total_views,
    tc.total_reactions,
    tc.total_comments,
    tc.engagement_score,
    c.title,
    c.slug,
    c.content_type
FROM mv_top_content_7d tc
LEFT JOIN contents c ON c.id = tc.content_id
WHERE tc.tenant_id = @tenant_id
  AND (@content_type::text = '' OR c.content_type = @content_type)
ORDER BY tc.engagement_score DESC
LIMIT @top_n;

-- =====================================================================
-- Revenue streams (from mv_revenue_streams).
-- =====================================================================

-- name: AdvancedAnalyticsRevenueStreams :many
SELECT * FROM mv_revenue_streams
WHERE tenant_id = @tenant_id
  AND month >= @from_month
ORDER BY month DESC;

-- =====================================================================
-- Ad campaign aggregate performance.
-- =====================================================================

-- name: AdvancedAnalyticsAdPerformance :many
SELECT
    ac.id AS campaign_id,
    ac.name AS campaign_name,
    ac.status,
    ac.budget_credits,
    ac.spent,
    COUNT(ae.id) FILTER (WHERE ae.event_type = 'impression') AS impressions,
    COUNT(ae.id) FILTER (WHERE ae.event_type = 'click') AS clicks,
    CASE
        WHEN COUNT(ae.id) FILTER (WHERE ae.event_type = 'impression') > 0
        THEN ROUND(
            COUNT(ae.id) FILTER (WHERE ae.event_type = 'click')::numeric /
            COUNT(ae.id) FILTER (WHERE ae.event_type = 'impression')::numeric * 100, 2
        )
        ELSE 0
    END AS ctr
FROM ad_campaigns ac
LEFT JOIN ad_events ae ON ae.campaign_id = ac.id
WHERE ac.tenant_id = @tenant_id
  AND (@status::text = '' OR ac.status = @status)
GROUP BY ac.id, ac.name, ac.status, ac.budget_credits, ac.spent
ORDER BY ac.spent DESC
LIMIT @top_n;

-- =====================================================================
-- Newsletter engagement: open rates, click rates, subscriber growth.
-- =====================================================================

-- name: AdvancedAnalyticsNewsletterEngagement :many
SELECT
    nc.id AS campaign_id,
    nc.name AS campaign_name,
    nc.subject,
    nc.sent_at,
    COUNT(nss.campaign_id) AS total_sent,
    COUNT(nss.campaign_id) FILTER (WHERE nss.status = 'opened') AS total_opened,
    COUNT(nss.campaign_id) FILTER (WHERE nss.status = 'clicked') AS total_clicked,
    CASE
        WHEN COUNT(nss.campaign_id) > 0
        THEN ROUND(
            COUNT(nss.campaign_id) FILTER (WHERE nss.status = 'opened')::numeric /
            COUNT(nss.campaign_id)::numeric * 100, 2
        )
        ELSE 0
    END AS open_rate,
    CASE
        WHEN COUNT(nss.campaign_id) > 0
        THEN ROUND(
            COUNT(nss.campaign_id) FILTER (WHERE nss.status = 'clicked')::numeric /
            COUNT(nss.campaign_id)::numeric * 100, 2
        )
        ELSE 0
    END AS click_rate
FROM newsletter_campaigns nc
LEFT JOIN newsletter_send_state nss ON nss.campaign_id = nc.id
WHERE nc.tenant_id = @tenant_id
  AND nc.sent_at IS NOT NULL
GROUP BY nc.id, nc.name, nc.subject, nc.sent_at
ORDER BY nc.sent_at DESC
LIMIT @top_n;

-- =====================================================================
-- Newsletter subscriber growth (total subscribers per list).
-- =====================================================================

-- name: AdvancedAnalyticsSubscriberGrowth :many
SELECT
    nl.id AS list_id,
    nl.name AS list_name,
    COUNT(nls.subscriber_id) AS subscriber_count
FROM newsletter_lists nl
LEFT JOIN newsletter_list_subscribers nls ON nls.list_id = nl.id
WHERE nl.tenant_id = @tenant_id
GROUP BY nl.id, nl.name
ORDER BY subscriber_count DESC;

-- =====================================================================
-- User retention cohorts (D1/D7/D30).
-- Computed from platform_events: cohort = signup date,
-- retained = user generated an event on day N after signup.
-- =====================================================================

-- name: AdvancedAnalyticsRetentionCohorts :many
SELECT
    date_trunc('day', u.created_at)::date AS cohort_date,
    COUNT(DISTINCT u.id) AS cohort_size,
    COUNT(DISTINCT pe_d1.user_id) AS retained_d1,
    COUNT(DISTINCT pe_d7.user_id) AS retained_d7,
    COUNT(DISTINCT pe_d30.user_id) AS retained_d30,
    CASE WHEN COUNT(DISTINCT u.id) > 0
        THEN ROUND(COUNT(DISTINCT pe_d1.user_id)::numeric / COUNT(DISTINCT u.id)::numeric * 100, 2)
        ELSE 0 END AS d1_rate,
    CASE WHEN COUNT(DISTINCT u.id) > 0
        THEN ROUND(COUNT(DISTINCT pe_d7.user_id)::numeric / COUNT(DISTINCT u.id)::numeric * 100, 2)
        ELSE 0 END AS d7_rate,
    CASE WHEN COUNT(DISTINCT u.id) > 0
        THEN ROUND(COUNT(DISTINCT pe_d30.user_id)::numeric / COUNT(DISTINCT u.id)::numeric * 100, 2)
        ELSE 0 END AS d30_rate
FROM users u
LEFT JOIN platform_events pe_d1
    ON pe_d1.user_id = u.id
    AND pe_d1.created_at >= u.created_at + INTERVAL '1 day'
    AND pe_d1.created_at < u.created_at + INTERVAL '2 days'
LEFT JOIN platform_events pe_d7
    ON pe_d7.user_id = u.id
    AND pe_d7.created_at >= u.created_at + INTERVAL '7 days'
    AND pe_d7.created_at < u.created_at + INTERVAL '8 days'
LEFT JOIN platform_events pe_d30
    ON pe_d30.user_id = u.id
    AND pe_d30.created_at >= u.created_at + INTERVAL '30 days'
    AND pe_d30.created_at < u.created_at + INTERVAL '31 days'
WHERE u.tenant_id = @tenant_id
  AND u.created_at >= @from_date
  AND u.is_bot = false
GROUP BY cohort_date
ORDER BY cohort_date DESC
LIMIT @top_n;
