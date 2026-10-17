# Coolify Deployment

JapaConnect deploys to Coolify as **N separate containers** — nothing is bundled.
Each app builds its own Dockerfile; infra services run as their own images on the
shared project network (reachable by service name).

## Application services

| Coolify app | Dockerfile | Notes |
|---|---|---|
| `api` | root `Dockerfile` | **HTTP + Asynq worker + scheduler run in one process** (`cmd/api/main.go` starts all three). One container covers web + background jobs. |
| `worker` *(optional)* | root `Dockerfile` | Worker-only node: same binary with `JC_HTTP_ENABLED=0`. Deploy when job load grows — see "Scaling the api" below. |
| `main` | `apps/main/Dockerfile` | |
| `admin` | `apps/admin/Dockerfile` | |
| `control` | `apps/control/Dockerfile` | |
| `academy` | `apps/academy/Dockerfile` | |
| `docs` | `apps/docs/Dockerfile` | |
| `tools` | `apps/tools/Dockerfile` | |

There is no `cmd/worker` binary — the worker shares `cmd/api`, gated by env flags.

### Scaling the api

All-in-one (default) is fine at launch. When job load grows, split out workers:

| Replica | `JC_HTTP_ENABLED` | `JC_SCHEDULER_ENABLED` |
|---|---|---|
| api ×N | *(set)* | `0` on all but one |
| worker ×N | `0` | `0` |

Only ONE process may run the scheduler (duplicate schedulers enqueue duplicate
periodic tasks). Workers/HTTP scale freely — Asynq distributes tasks.

## Infra services (Coolify "Docker Image" resources)

| Service | Image | Volume | Env |
|---|---|---|---|
| postgres | `pgvector/pgvector:pg18` | yes | `POSTGRES_PASSWORD` etc. |
| valkey | `valkey/valkey:9-alpine` | yes | `valkey-server --appendonly yes` |
| meilisearch | `getmeili/meilisearch:v1.45` | yes | `MEILI_MASTER_KEY` |
| seaweedfs | `chrislusf/seaweedfs:latest` | yes | `server -s3 -dir=/data` |
| imgproxy | `darthsim/imgproxy` | no | `IMGPROXY_KEY`/`IMGPROXY_SALT` |
| glitchtip | `glitchtip/glitchtip:latest` | yes | `DATABASE_URL`, `REDIS_URL`, `SECRET_KEY` |
| umami | `ghcr.io/umami-software/umami:postgresql-v2.20.2` | yes | `DATABASE_URL`, `APP_SECRET` |

**Optional:** `postgres-replica` (read replica — dev stack has it; deploy if you
want read scaling), `honcho` (no published image — build from the Plastic Labs
repo; leave `JC_HONCHO_URL` unset until it's up).

**Do NOT deploy from `docker-compose.yml`** — it's dev-only (`trust` auth,
`mailpit`, `nginx` — Coolify's Traefik already fronts the apps).

## Env wiring (api service)

Same-project services resolve by name — `.env.example` defaults already use
these hostnames:

```
JC_DATABASE_URL=postgres://japa:<pw>@postgres:5432/japaconnect
JC_VALKEY_ADDR=valkey:6379
JC_MEILI_HOST=http://meilisearch:7700
JC_MEILI_API_KEY=<master key>
JC_SEAWEEDFS_S3_ENDPOINT=http://seaweedfs:8333
JC_IMGPROXY_URL=http://imgproxy:8080
JC_GLITCHTIP_DSN=https://<key>@glitchtip.<domain>/<id>
JC_SMTP_HOST=<real SMTP — mailpit is dev-only>
JC_HONCHO_URL=                    # leave unset unless you deploy Honcho
JC_VAPID_PUBLIC_KEY / JC_VAPID_PRIVATE_KEY / JC_VAPID_SUBJECT
JC_STRIPE_SECRET_KEY / webhook / price ids…
```

## GitHub → Coolify webhook deploys

`.github/workflows/deploy.yml` POSTs each app's **Deploy Webhook**
(App → Webhooks → Deploy Webhook URL) after green CI. Repo secrets:
`COOLIFY_WEBHOOK_API`, `_MAIN`, `_ADMIN`, `_CONTROL`, `_ACADEMY`, `_DOCS`, `_TOOLS`.

## Staging environment

Coolify projects have built-in **environments** — create a `staging` environment
in the same project and clone each application service into it (same Dockerfiles;
same server, or a second VPS if the box is tight on memory). Point the staging
copies at the `staging` git branch; production keeps tracking `main`.

### Domains

Use a `staging.japa.to` subdomain tree — Traefik issues TLS
automatically. One hostname per app:

`staging.japa.to`, `api.staging.japa.to`,
`admin.staging.`, `control.staging.`, `academy.staging.`, `docs.staging.`,
`tools.staging.japa.to`

### Infra in staging

| Service | Staging? |
|---|---|
| postgres | **Yes** — own instance, own password |
| valkey | **Yes** — queues/cache must not mix with prod |
| meilisearch | **Yes** — separate instance, **or** share prod's instance via `JC_MEILI_INDEX_PREFIX=staging_` on the staging api |
| seaweedfs / imgproxy | Optional — share or skip |
| glitchtip / umami | Optional — separate site IDs if you want staging telemetry |
| postgres-replica | Skip — read scaling isn't what staging validates |
| mailpit | Add — captures all transactional email so staging can't mail real users |

### Env overrides

Keep `JC_ENV=production` on the staging API — any other value makes
`internal/config` skip production validation and read `.env`, so staging would
exercise different code paths than prod.

```
JC_ENV=production
JC_SITE_URL=https://staging.japa.to
JC_COOKIE_DOMAIN=.staging.japa.to   # never reuse .japa.to
JC_CORS_ORIGINS=https://staging.japa.to,https://admin.staging.japa.to,…
JC_MCP_PUBLIC_URL=https://api.staging.japa.to
JC_DATABASE_URL=postgres://japa:<pw>@postgres:5432/japa
JC_VALKEY_ADDR=valkey:6379                  # the staging instance
JC_SMTP_HOST=mailpit                        # captures, never sends
```

Also: Stripe/Paystack/NOWPayments in test mode, and **fresh** secrets —
`JC_JWT_SECRET`, `JC_TOTP_ENCRYPTION_KEY`, `JC_INTERNAL_TOKEN`, Meilisearch
master key, DB password. Never reuse prod secrets.

`PUBLIC_*` values are **build-time** args for the frontends — set them as
Coolify build variables on each staging app:

```
PUBLIC_JC_API_URL=https://api.staging.japa.to/api/v1
PUBLIC_JC_WS_URL=wss://api.staging.japa.to/api/v1/ws
PUBLIC_*_ORIGIN=https://<app>.staging.japa.to   # cross-app links
PRIVATE_API_BASE_URL=http://<staging-api-service>:8080  # runtime env, internal name
```

Host detection (runtime env on the SvelteKit apps):

```
# apps/main + apps/tools — one var moves every canonical surface host:
# tools.<domain>, academy.<domain>, roadmap.<domain>, and per-tool
# {sub}.<domain> subdomains all resolve under the staging domain.
JC_BASE_DOMAIN=staging.japa.to
# api — only needed if sharing one Meilisearch instance with prod:
JC_MEILI_INDEX_PREFIX=staging_
```

External consoles (not in the repo): OAuth providers need staging redirect
URIs registered, Stripe/Paystack need staging webhook endpoints, Umami needs
staging site IDs (or leave `PUBLIC_JC_UMAMI_*` empty to disable).

### Migrations & seeding

`cmd/migrate` is a separate binary — the root Dockerfile only builds `cmd/api`.
Run migrations on staging as a pre-deployment command or a one-off exec before
each deploy, same as prod. Seed fresh data with `cmd/seed` +
`cmd/seed-geo-cities` — note `cmd/seed` requires `JC_SEED_ADMIN_PASSWORD` when
`JC_ENV=production` (no dev default). Never point staging at the prod database.

### Staging deploys

`deploy.yml` POSTs the staging apps' deploy webhooks after green CI on the
`staging` branch. Additional repo secrets — same names as prod with a
`_STAGING` suffix: `COOLIFY_WEBHOOK_API_STAGING`, `COOLIFY_WEBHOOK_MAIN_STAGING`,
`COOLIFY_WEBHOOK_ADMIN_STAGING`, `COOLIFY_WEBHOOK_CONTROL_STAGING`,
`COOLIFY_WEBHOOK_ACADEMY_STAGING`, `COOLIFY_WEBHOOK_DOCS_STAGING`,
`COOLIFY_WEBHOOK_TOOLS_STAGING`.

Lighter-weight alternative: enable **push-to-deploy** on the staging Coolify
apps and drop the CI gate — staging then rebuilds on every push to the branch.
