-- Section 40: Platform Analytics & Site Reports

-- Platform Events (write-optimised)
CREATE TABLE platform_events (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    event_type  text NOT NULL,
    user_id     uuid REFERENCES users(id),
    entity_type text,
    entity_id   uuid,
    meta        jsonb NOT NULL DEFAULT '{}',
    country_code text,
    session_id  text,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_pe_tenant_type_created ON platform_events (tenant_id, event_type, created_at DESC);
CREATE INDEX idx_pe_user ON platform_events (user_id, event_type, created_at DESC) WHERE user_id IS NOT NULL;
CREATE INDEX idx_pe_entity ON platform_events (entity_type, entity_id, event_type) WHERE entity_id IS NOT NULL;

-- Security Scan Results
CREATE TABLE security_scan_results (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    scan_type   text NOT NULL,
    severity    text NOT NULL DEFAULT 'info',
    finding     text NOT NULL,
    meta        jsonb NOT NULL DEFAULT '{}',
    resolved_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_security_scan_tenant ON security_scan_results(tenant_id, severity, created_at DESC);

-- Materialized Views (refreshed by analytics:refresh_views Asynq task)
CREATE MATERIALIZED VIEW mv_daily_content_stats AS
SELECT
    date_trunc('day', created_at)::date AS day,
    tenant_id,
    entity_id AS content_id,
    COUNT(*) FILTER (WHERE event_type = 'content.viewed') AS views,
    COUNT(*) FILTER (WHERE event_type = 'reaction.given') AS reactions,
    COUNT(*) FILTER (WHERE event_type = 'comment.created') AS comments,
    COUNT(DISTINCT user_id) AS unique_viewers
FROM platform_events
WHERE event_type IN ('content.viewed', 'reaction.given', 'comment.created')
GROUP BY 1, 2, 3;

CREATE UNIQUE INDEX ON mv_daily_content_stats (day, tenant_id, content_id);

CREATE MATERIALIZED VIEW mv_daily_space_stats AS
SELECT
    date_trunc('day', created_at)::date AS day,
    tenant_id,
    entity_id AS space_id,
    COUNT(*) FILTER (WHERE event_type = 'space.joined') AS joins,
    COUNT(*) FILTER (WHERE event_type = 'content.created') AS posts,
    COUNT(DISTINCT user_id) AS active_users
FROM platform_events
WHERE event_type IN ('space.joined', 'content.created')
GROUP BY 1, 2, 3;

CREATE UNIQUE INDEX ON mv_daily_space_stats (day, tenant_id, space_id);

CREATE MATERIALIZED VIEW mv_user_engagement_stats AS
SELECT
    date_trunc('day', created_at)::date AS day,
    tenant_id,
    user_id,
    COUNT(*) FILTER (WHERE event_type = 'content.created') AS posts_created,
    COUNT(*) FILTER (WHERE event_type = 'reaction.given') AS reactions_given,
    COUNT(*) FILTER (WHERE event_type = 'comment.created') AS comments_made,
    COUNT(*) FILTER (WHERE event_type = 'content.viewed') AS views
FROM platform_events
WHERE user_id IS NOT NULL
GROUP BY 1, 2, 3;

CREATE UNIQUE INDEX ON mv_user_engagement_stats (day, tenant_id, user_id);
