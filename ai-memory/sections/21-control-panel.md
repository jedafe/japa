# Section 21 — Control Panel

**Status:** ✅ Locked

## Decided
Feature Flags UI, Extension management UI. Platform dashboard: throughput, latency, error rate, queue depth per tier, DB/Valkey usage, active WS connections, Meilisearch health, SeaweedFS storage, DLQ depth, Honcho connectivity/sync lag/store size (post-audit addition, same health-group pattern as Meilisearch — see ADR-027). AI cost monitoring via `ai_usage_log` (provider, model, task_type, tokens, cost_usd, user_id nullable) — Ollama tracked at zero cost since it's self-hosted, with configurable budget thresholds and alerts. System health in four groups (infrastructure, application, background processing, real-time), simple green/yellow/red status on a 30-60s poll, with an optional critical-only push riding the `moderation_alert` WS event type.

Audit log formalized here as the canonical store: `audit_logs` (actor_user_id nullable, action_type, target_type + target_id, metadata JSONB, ip_address, created_at), append-only, `super_admin`-only query access — every prior reference to "logs to Audit Log" across other sections points here.

Backup/deployment visibility lives here (status, history, manual trigger, rollback) — actual backup execution and deploy triggering are infra-level (Section 27), never triggered by the running app on itself.

## Database Health Tooling (restored post-audit, see ADR-029)

A DB health panel (separate from the application-metrics dashboard group): table bloat estimate, dead-tuple ratio, long-running query list (`pg_stat_activity`, queries beyond a configurable threshold), and a one-click **manual VACUUM** trigger via an Asynq `db_manual_vacuum` task dispatched to the maintenance tier rather than executed in the HTTP handler. Configurable slow-query threshold (default 1 second) drives the display — queries above the threshold are highlighted; well below it, the panel shows clean. Gated by the `platform.db.health_view` and `platform.db.vacuum_trigger` permission nodes rather than being `super_admin`-only, so the access can be delegated narrowly. Read-only — no schema changes, no index creation, no data modification via this surface.

## API Key Management Panel (session 6 — Section 47)

Route: `/control/api-keys`

The Control Panel hosts the admin-facing side of the Public API / Developer Portal (Section 47). Gated by permission nodes `api.keys.view.all`, `api.keys.manage`, `api.keys.analytics.view`.

**Views:**
- **All Keys** — paginated table: user, app_name, key_prefix, rate_limit_tier, status, request_count, last_used_at, created_at. Filterable by status/tier/user. CSV export.
- **Key Detail** — shows all metadata + a 30-day request volume sparkline + top 5 endpoints called + error rate.
- **Create Key** — admin can create keys on behalf of any user OR as a system key (user_id = NULL for server-to-server integrations). Scopes are multi-select from the `api.*` node family.
- **Actions** — Suspend (reversible; key returns `401` while suspended), Revoke (permanent; key deleted from lookup), Change rate limit tier (takes effect immediately — no redeploy), Edit notes.
- **Rate Tier Config** — inline editor for the four tier limits stored in `system_settings` (`api.rate_limit.basic`, `.standard`, `.premium`). Changes are hot-loaded by the rate-limit middleware; no restart required.

## Audit Log Schema (audit pass 2 — referenced 8× across docs, never had CREATE TABLE)

```sql
-- Append-only; no UPDATE or DELETE ever runs on this table (enforced at application layer)
CREATE TABLE audit_logs (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id            uuid NOT NULL REFERENCES tenants(id),
  actor_user_id        uuid REFERENCES users(id) ON DELETE SET NULL,
  impersonated_user_id uuid REFERENCES users(id) ON DELETE SET NULL,  -- Section 46
  action_type          text NOT NULL,  -- e.g. 'user.banned'|'content.removed'|'impersonate.start'|'cache.flushed'
  entity_type          text,           -- 'user'|'content'|'space'|'tag'|'api_key'|'system'
  entity_id            uuid,
  metadata             jsonb NOT NULL DEFAULT '{}',
  ip_address           inet,
  user_agent           text,
  created_at           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_actor    ON audit_logs (tenant_id, actor_user_id, created_at DESC);
CREATE INDEX idx_audit_entity   ON audit_logs (entity_type, entity_id, created_at DESC)
  WHERE entity_id IS NOT NULL;
CREATE INDEX idx_audit_created  ON audit_logs (tenant_id, created_at DESC);
CREATE INDEX idx_audit_imperso  ON audit_logs (impersonated_user_id, created_at DESC)
  WHERE impersonated_user_id IS NOT NULL;
-- Retention: rows older than system_settings audit.log_retention_days (default 365) pruned
-- weekly by maintenance:prune_audit_logs task (low tier)
```
