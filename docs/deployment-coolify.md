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
