# Section 21 — Control Panel

**Status:** ✅ Locked

## Decided
Feature Flags UI, Extension management UI. Platform dashboard: throughput, latency, error rate, queue depth per tier, DB/Redis usage, active WS connections, Meilisearch health, MinIO storage, DLQ depth, Honcho connectivity/sync lag/store size (post-audit addition, same health-group pattern as Meilisearch — see ADR-027). AI cost monitoring via `ai_usage_log` (provider, model, task_type, tokens, cost_usd, user_id nullable) — Ollama tracked at zero cost since it's self-hosted, with configurable budget thresholds and alerts. System health in four groups (infrastructure, application, background processing, real-time), simple green/yellow/red status on a 30-60s poll, with an optional critical-only push riding the `moderation_alert` WS event type.

Audit log formalized here as the canonical store: `audit_logs` (actor_user_id nullable, action_type, target_type + target_id, metadata JSONB, ip_address, created_at), append-only, `super_admin`-only query access — every prior reference to "logs to Audit Log" across other sections points here.

Backup/deployment visibility lives here (status, history, manual trigger, rollback) — actual backup execution and deploy triggering are infra-level (Section 27), never triggered by the running app on itself.

## Database Health Tooling (restored post-audit, see ADR-029)

A DB health panel (separate from the application-metrics dashboard group): table bloat estimate, dead-tuple ratio, long-running query list (`pg_stat_activity`, queries beyond a configurable threshold), and a one-click **manual VACUUM** trigger via an Asynq `db_manual_vacuum` task dispatched to the maintenance tier rather than executed in the HTTP handler. Configurable slow-query threshold (default 1 second) drives the display — queries above the threshold are highlighted; well below it, the panel shows clean. Gated by the `platform.db.health_view` and `platform.db.vacuum_trigger` permission nodes rather than being `super_admin`-only, so the access can be delegated narrowly. Read-only — no schema changes, no index creation, no data modification via this surface.
