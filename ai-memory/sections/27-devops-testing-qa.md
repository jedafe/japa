# Section 27 — DevOps, Testing & QA

**Status:** ✅ Locked (GlitchTip replaces Sentry; Cloudflare added)

## Infrastructure

VPS deployment via Coolify or Dokploy. Provider-agnostic — Hetzner CX31 was the original cost reference, not a hard requirement. Docker Compose orchestration with these services:

| Service | Port | Notes |
|---|---|---|
| go-api | 8080 | Go monolith |
| sveltekit-main | 3000 | Main site (SSR) |
| sveltekit-admin | 3001 | Admin SPA |
| sveltekit-control | 3002 | Control Panel SPA |
| sveltekit-learn | 3003 | Learning Platform (SSR) |
| postgres | 5432 | PostgreSQL 18.x + pgvector |
| valkey | 6379 | Valkey (Redis-compatible) |
| meilisearch | 7700 | Search |
| minio | 9000/9001 | Object storage |
| imgproxy | 8080 | Image transformations (internal) |
| honcho | 8000 | AI memory (own `honcho` DB on Postgres) |
| glitchtip | 8000 | Self-hosted error tracking |
| mailpit | 1025/8025 | Local email dev only |

Nginx as reverse proxy with SSL termination (Let's Encrypt via Coolify/Dokploy).

## Error Tracking — GlitchTip (self-hosted)

GlitchTip replaces Sentry. It is Sentry SDK-compatible — the Go backend and SvelteKit frontends use the standard Sentry SDK pointing to the GlitchTip DSN:

```go
// Go backend
sentry.Init(sentry.ClientOptions{
  Dsn:         os.Getenv("JC_GLITCHTIP_DSN"),
  Release:     os.Getenv("JC_RELEASE_SHA"),
  Environment: os.Getenv("JC_ENV"),
  TracesSampleRate: 1.0, // 100% acceptable at v1 traffic — revisit at scale
})
defer sentry.Flush(2 * time.Second)
```

```ts
// SvelteKit (+layout.ts or hooks.client.ts)
import * as Sentry from '@sentry/sveltekit';
Sentry.init({
  dsn: import.meta.env.JC_GLITCHTIP_DSN,
  release: import.meta.env.JC_RELEASE_SHA,
  tracesSampleRate: 1.0,
});
```

GlitchTip PostgreSQL connects to a separate `glitchtip` database on the same Postgres 18.x instance (same pattern as Honcho).

## CI/CD (GitHub Actions)

```yaml
# .github/workflows/ci.yml
jobs:
  check:
    - lint (golangci-lint, eslint)
    - test (go test -race ./..., vitest)
    - build (go build, bun run build)
    - version-pin-check: grep stack versions in AGENTS.md, fail if mismatch

  deploy-staging:
    needs: check
    on: push to staging branch
    - coolify/dokploy deploy hook

  deploy-production:
    needs: check
    on: push to main branch
    - run DB migrations (distinct pre-traffic step)
    - coolify/dokploy deploy hook
    - post-deploy: purge Cloudflare cache (if configured)
```

## Database Migrations

Migrations run as a distinct pre-traffic deploy step — never inside the running server. Breaking schema changes use the two-phase pattern:
1. Phase 1: add new column (nullable), deploy app that writes both old and new
2. Phase 2: backfill data, add NOT NULL constraint, remove old column in next deploy

`pg_stat_statements` extension **must be enabled in the first migration** — it's required for production query optimization and cannot be retroactively added without clearing the stats:

```sql
-- 00001_initial_setup.sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgvector";
CREATE EXTENSION IF NOT EXISTS "pg_stat_statements";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";
```

## Connection Pool (pgx)

```go
pool, _ := pgxpool.New(ctx, os.Getenv("JC_DATABASE_URL"))
pool.Config().MaxConns = 25
pool.Config().MinConns = 5
pool.Config().MaxConnLifetime = 30 * time.Minute
pool.Config().MaxConnIdleTime = 5 * time.Minute

// Reader/Writer split from day one (both point to same DSN until replica exists)
type DB struct {
  Writer *pgxpool.Pool
  Reader *pgxpool.Pool  // = Writer until read replica is added
}
```

## Backups

Nightly automated to MinIO (or S3-compatible provider), 30 daily + 12 monthly retention. Restore runbook is a documented procedure — never a one-click UI action:

```bash
# restore-runbook.sh
pg_restore --clean --if-exists -d "$JC_DATABASE_URL" "$1"
go run ./cmd/reindex-search   # Meilisearch always rebuilt from Postgres on restore
go run ./cmd/verify-health
```

Quarterly restore drills recommended — untested backups are a false sense of security.

## Test Coverage

Targeted by risk, not a blanket percentage:
- 60–70% on core Go packages (RBAC checks, payment logic, content state machine, wallet ledger)
- Integration tests against real test Postgres + Valkey for auth / payments / RBAC
- Small Playwright E2E smoke set for critical journeys (signup, post create, payment)
- Lighthouse CI gate for Core Web Vitals regressions

## Staging

Single lightweight environment on cheaper infra, auto-deployed from `staging` branch through the same CI/CD pipeline.

## See Also

ADR-023 (file upload validation), ADR-021 (Valkey over Redis 8), ADR-022 (rate limiting).
