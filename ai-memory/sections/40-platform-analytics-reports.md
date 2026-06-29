# Section 40 — Platform Analytics & Site Reports

**Status:** ✅ Locked

## Two-Layer Analytics

**Layer 1 — Umami (self-hosted, external):** privacy-first page-view analytics. Cookieless. Embedded script on main site frontend. Umami dashboard accessible at a sub-path inside Control Panel via iframe or API proxy. Tracks: page views, unique visitors, bounce rate, referrers, browser/OS, country.

**Layer 2 — Platform Native Analytics:** custom event tracking from Go handlers for platform-specific signals that Umami can't capture (content created, space joined, tag followed, course enrolled, payment completed, reaction given, etc.). Stored in a write-optimised table, read via materialized views.

## Platform Events Schema

```sql
CREATE TABLE platform_events (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenants(id),
  event_type  text NOT NULL,
  -- 'content.created'|'content.viewed'|'space.joined'|'space.followed'
  -- |'tag.followed'|'user.registered'|'payment.completed'|'course.enrolled'
  -- |'reaction.given'|'comment.created'|'ad.viewed'|'ad.clicked'|'search.performed'
  -- |'dm.sent'|'ticket.submitted'|'follow.created'|'share.clicked'
  user_id     uuid REFERENCES users(id),   -- NULL for anonymous
  entity_type text,
  entity_id   uuid,
  meta        jsonb NOT NULL DEFAULT '{}', -- context-specific data
  country_code text,                        -- from GeoIP (Section 37)
  session_id  text,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_pe_tenant_type_created ON platform_events (tenant_id, event_type, created_at DESC);
CREATE INDEX idx_pe_user ON platform_events (user_id, event_type, created_at DESC) WHERE user_id IS NOT NULL;
CREATE INDEX idx_pe_entity ON platform_events (entity_type, entity_id, event_type) WHERE entity_id IS NOT NULL;
```

## Materialized Views (refreshed periodically by `analytics:refresh_views` Asynq task, low tier, hourly)

```sql
CREATE MATERIALIZED VIEW mv_daily_content_stats AS
SELECT
  date_trunc('day', created_at)::date AS day,
  tenant_id, entity_id AS content_id,
  COUNT(*) FILTER (WHERE event_type = 'content.viewed')    AS views,
  COUNT(*) FILTER (WHERE event_type = 'reaction.given')    AS reactions,
  COUNT(*) FILTER (WHERE event_type = 'comment.created')   AS comments,
  COUNT(DISTINCT user_id)                                   AS unique_viewers
FROM platform_events
WHERE event_type IN ('content.viewed', 'reaction.given', 'comment.created')
GROUP BY 1, 2, 3;

CREATE UNIQUE INDEX ON mv_daily_content_stats (day, tenant_id, content_id);
```

## User Analytics Dashboard (`/dashboard/analytics`)

For content creators — mirrors dev.to's analytics UX:

```
Metrics shown (date-range selectable, last 7 / 30 / 90 days / custom):
  Total views, reactions, comments, followers gained
  Per-content breakdown: sorted by views, shows title + type + date
  Follower/following counts over time (line chart)
  Top content by views (bar chart)
  Traffic sources (from platform_events.meta.referrer if set)
  Ad performance (if user has run ads via Section 25): impressions, clicks, CTR
  Credit/Points earned this period (from Section 28)
```

## Security Scanner

A periodic `security:scan` task (low tier, daily at 03:00 UTC) runs checks and appends findings to a `security_scan_results` table:

```sql
CREATE TABLE security_scan_results (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id   uuid NOT NULL REFERENCES tenants(id),
  scan_type   text NOT NULL,
  -- 'dependency_audit'|'failed_login_spike'|'large_error_rate'|'sla_breach_count'
  -- |'suspicious_ip_patterns'|'orphaned_media'|'stale_tokens'
  severity    text NOT NULL DEFAULT 'info',  -- 'info'|'warning'|'critical'
  finding     text NOT NULL,
  meta        jsonb NOT NULL DEFAULT '{}',
  resolved_at timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now()
);
```

Surfaced in Control Panel → Security tab. Critical findings trigger a `moderation_alert` WebSocket event to logged-in super_admins.

## Cache Management (Admin)

Control Panel → Tools → Cache:

```go
// Flush operations — each is a distinct admin action with Audit Log entry
func FlushCache(ctx context.Context, scope string) error {
  switch scope {
  case "all":
    flushValkey(ctx)       // FLUSHDB on the jc: namespace (not full DB, just our keys)
    clearCFCache(ctx)      // Cloudflare Cache-Tag purge (if Cloudflare configured)
    clearDBQueryCache(ctx) // pg_stat_statements reset (optional)
  case "pages":
    clearCFCache(ctx, "cf-cache-tag: pages")
  case "feeds":
    // jc:feed:* is not a documented key — remove this flush or document what it covers
    // (feeds data lives in Postgres; no Valkey feed cache exists in V1)
    // flushValkeyPattern(ctx, "jc:feed:*")  -- REMOVED: erroneous key pattern
  case "i18n":
    flushValkeyPattern(ctx, "jc:i18n:*")
  case "search":
    // Meilisearch doesn't need flushing — rebuilt from Postgres on demand
  }
  auditLog(ctx, "cache.flushed", scope)
  return nil
}
```

## Site Reports (Exportable)

Admin Panel → Reports — all tables support CSV and JSON export:

| Report | Filters | Format |
|---|---|---|
| User registrations | Date range, country, user group | CSV / JSON |
| Content published | Date range, content type, space | CSV / JSON |
| Payment transactions | Date range, provider, purpose type | CSV |
| Ticket resolutions | Date range, type, status | CSV / JSON |
| Platform events | Date range, event type | JSON (large) |
| Ad performance | Date range, campaign | CSV |
| Moderation actions | Date range, action type | CSV |

Export runs as a `data_export` Asynq task (low tier) — for large datasets, user gets an email with download link rather than a synchronous response.

## See Also

Section 21 (Control Panel dashboard, DB health, Audit Log), Section 22 (Admin Panel reports UI), Section 37 (GeoIP feeds country_code into analytics). ADR-040.
