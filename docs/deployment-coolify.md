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

## GitHub → private registry → Coolify image deploys

Images are built on the self-hosted CI runner and pushed to a private
`registry:2` container on the Coolify host — no GitHub Packages quota,
no public registry, images never leave your infra. Each app's
**Deployment Type** is **Docker Image**:

- staging apps track `<registry-host>/japa-<app>:staging`
- production apps track `<registry-host>/japa-<app>:production`

### The registry container

Deploy `registry:2` in the Coolify project as a **Docker Image** service:

- domain: `registry.japa.to` → container port `5000` (Traefik issues TLS)
- volume: `/var/lib/registry` (blob storage — size it generously)
- auth (htpasswd — required, it's internet-facing):
  ```
  REGISTRY_AUTH=htpasswd
  REGISTRY_AUTH_HTPASSWD_REALM=japa-registry
  REGISTRY_AUTH_HTPASSWD_PATH=/auth/htpasswd
  REGISTRY_HTTP_SECRET=<random 64+ chars>
  ```
  Mount an htpasswd file at `/auth/htpasswd`:
  `docker run --rm httpd:2-alpine htpasswd -Bbn ci <password> > htpasswd`
  (the `ci` user = `REGISTRY_USERNAME`/`REGISTRY_PASSWORD` repo secrets)

Coolify app pulls authenticate once via **Settings → Registries** →
`registry.japa.to` + the same credentials. Deploy webhooks then pull
the tag and roll the container.

### Two flows

- **Staging** (`deploy.yml`, manual dispatch): builds only the changed
  apps (`apps` input: `changed`/`all`/comma list) with the `staging`
  environment's `PUBLIC_*` variables, tags `:<sha>` + `:staging`, calls
  each deployed app's webhook.
- **Production** (`deploy-production.yml`, manual dispatch): retags the
  api image `:staging`→`:production` registry-side (it is env-agnostic —
  all config is runtime `JC_*` env), builds the 6 frontends with the
  `production` environment's `PUBLIC_*` variables (vite inlines them at
  build time, so frontend images are per-environment builds from the
  same commit), then calls the prod webhooks.

The `PUBLIC_*` build args live as **environment variables** on the
`staging`/`production` GitHub environments — same names as the
Dockerfiles' `ARG`s (`PUBLIC_JC_API_URL`, `PUBLIC_JC_WS_URL`,
`PUBLIC_JC_ENV`, `PUBLIC_JC_SITE_URL`, `PUBLIC_JC_GLITCHTIP_DSN`,
`PUBLIC_MAIN_ORIGIN`, `PUBLIC_ADMIN_ORIGIN`, `PUBLIC_DOCS_ORIGIN`,
`PUBLIC_JC_ACADEMY_ORIGIN`, `PUBLIC_JC_TOOLS_ORIGIN`). Deploy webhook
URLs are **environment secrets** (`COOLIFY_WEBHOOK_*` on `production`,
`*_STAGING` on `staging`). Registry credentials are repo secrets
(`REGISTRY_USERNAME`/`REGISTRY_PASSWORD`); `REGISTRY_HOST` is a repo
variable.

Retention: `registry-retention.yml` deletes all but the newest 10 `sha`
tags weekly. Reclaim blob space with a weekly cron on the VPS:
`docker exec <registry> registry garbage-collect /etc/docker/registry/config.yml`
followed by `docker system prune -f`.

**Update flow (manual staging, gated production):** a maintainer runs
`Deploy Staging` from Actions — path-filtered, so only apps whose source
changed since the last deploy rebuild. Production ships only when a
maintainer runs the `Deploy Production` workflow (`deploy-production.yml`) by hand —
**Actions → Deploy Production → Run workflow** on a commit that has
already been exercised on staging. The manual dispatch is the promotion
gate: GitHub's required-reviewers environment rule needs a paid plan
for private repos, so environment approval isn't used. There is no
`staging` branch; staging Coolify apps track `main` so the deployed
SHA is identical in both environments.

One-time GitHub setup: **Settings → Environments** → create `staging`
and `production` (no protection rules needed — the workflow gate is
the dispatch trigger). If the plan is upgraded later, a required
reviewer on `production` can replace the manual gate.

## Staging environment

Coolify projects have built-in **environments** — create a `staging` environment
in the same project and clone each application service into it (same Dockerfiles;
same server, or a second VPS if the box is tight on memory). Both staging and
production apps track the `main` branch — the environments differ only by
Coolify env vars/build args, and the workflow split decides which fires
(see "Update flow" above).

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

`cmd/migrate` is a separate binary — it ships in the api image as
`/app/japa-migrate` alongside `/app/japa-seed`, `/app/japa-seed-geo-cities`,
and `/app/japa-requeue-universities` (migrations/ and db/seed/ are copied
in too). `japa-seed` requires `JC_SEED_ADMIN_PASSWORD` when
`JC_ENV=production` (no dev default). Never point staging at the prod
database.

**Wiring automatic migrations:** on the api application → **Advanced** →
set **Post-Deployment Command** to `/app/japa-migrate` (leave the container
field empty for a single-container app). Do the same on the staging api
app so schema follows every staging deploy.

Why post- and not pre-: Coolify runs *pre*-deployment commands inside the
**old** container (which doesn't carry the new migration files — and it's
skipped on first deploy). *Post*-deployment runs inside the new container,
which has the new `migrations/` and the binary. A post-deploy failure is
logged but doesn't roll back the deploy — additive zero-downtime
migrations keep that window safe (new code must already tolerate the old
schema), but check the deployment log / `schema_migrations` count when a
release ships a migration.

Manual equivalent any time: `docker exec <api-container> /app/japa-migrate`.

### Staging deploys

Automatic — `deploy-staging` in `deploy.yml` POSTs the staging apps' deploy
webhooks after every green CI on `main`. Production is separate:
`deploy-production.yml` only runs on manual `workflow_dispatch`.
Staging repo secrets are the prod names with a `_STAGING` suffix: `COOLIFY_WEBHOOK_API_STAGING`, `COOLIFY_WEBHOOK_MAIN_STAGING`,
`COOLIFY_WEBHOOK_ADMIN_STAGING`, `COOLIFY_WEBHOOK_CONTROL_STAGING`,
`COOLIFY_WEBHOOK_ACADEMY_STAGING`, `COOLIFY_WEBHOOK_DOCS_STAGING`,
`COOLIFY_WEBHOOK_TOOLS_STAGING`.

## Migrating a dev database to production

You can seed prod from a `pg_dump` of the dev database instead of starting
from the baseline — but decide what to do about media first, because the
files and the DB rows are separate problems.

### Media files live in SeaweedFS, not the app container

Dev stores uploads under `data/media/<storage_bucket>/<storage_path>`
(`JC_MEDIA_ROOT`, ~8 GB for a mature dev DB — almost all images). Prod uses
the `JC_S3_*` backend against the SeaweedFS service, so there is nothing to
copy into the image or a volume — if you want the dev files in prod, sync
them into the SeaweedFS buckets directly:

```bash
aws s3 sync data/media/japaconnect s3://japaconnect \
  --endpoint-url http://<seaweedfs-host>:8333 --exclude "temp/*"
aws s3 sync data/media/media s3://media \
  --endpoint-url http://<seaweedfs-host>:8333 --exclude "temp/*"
```

`temp/` subtrees are scratch — always exclude them. This is a one-time
transfer of the full media set over your connection; `aws s3 sync` / `rsync`
are resumable if the link drops.

### Space imagery re-downloads server-side — you can skip the sync

Country flags, state/city/university logos, space headers, and attraction
images are all fetched from remote sources (flagcdn, Wikidata, Wikipedia)
by the `space:auto_populate` Asynq task — they are **not** user uploads and
do not need to be transferred. To go live with a dev dump but no media files:

```sql
-- detach spaces from media rows whose files you didn't ship
update spaces set logo_media_id = null, header_image_id = null;
```

```bash
# then re-enqueue enrichment — workers download the images on the server
go run ./cmd/requeue-universities -types=country,state,city,university,attraction
```

Note the ordering: `requeue-universities` selects spaces **missing** a logo
or header, so `logo_media_id`/`header_image_id` must be NULLed first —
a media row pointing at a nonexistent object counts as "has a logo" and
would be skipped while serving broken images.

(Country-flag chips in the frontend need nothing at all — they're emoji
rendered client-side via `countryCodeToEmoji`.)

### User-uploaded media is not recoverable

Post attachments, avatars, blog/content images, etc. have no recorded
source URL (~all `media` rows lack `source_platform`). If you restore the
dev dump without syncing the files, those rows serve broken images
permanently — either sync the `media`/`japaconnect` buckets as above, or
delete/mark the dangling rows (`deleted_at`) so they don't render.

### Fresh prod (no dev dump) — chosen launch path

Nothing uploads from your connection. The baseline seed migration
(`migrations/000735_baseline_seed.up.sql`) ships all platform-owned
reference/config data — space types, ~15.7k geo/university spaces with
their `wiki_content`, wiki type configs/categories (`000739`), i18n, RBAC,
system settings — while `media` and all user-generated tables are excluded
by `scripts/baseline_seed_exclusions.txt`. Everything else regenerates
on the server:

```bash
# All four ops binaries ship inside the api image — run each as a
# one-off exec on the api container (env is already wired there):
#   docker exec -e JC_SEED_ADMIN_PASSWORD=<pw> <api-container> /app/japa-seed

# 1. schema + baseline seed + all later migrations
/app/japa-migrate

# 2. admin user + default community spaces
#    (JC_SEED_ADMIN_PASSWORD required — no dev default in production)
/app/japa-seed

# 3. ~69k city/state geo spaces from local TSVs (db/seed/ ships in-image)
/app/japa-seed-geo-cities

# 4. re-enqueue enrichment — workers pull Wikidata/Wikipedia content,
#    flagcdn flags, logos/headers, and seed stub wiki pages server-side
/app/japa-requeue-universities -types=country,state,city,university,attraction
```

Step 4 runs as Asynq tasks on the worker — images land in SeaweedFS and
`wiki_content`/stub `wiki_pages` fill in progressively. Re-run it any time;
it's idempotent (selects spaces still missing a logo/header).

What this deliberately does **not** carry over: dev user accounts, posts,
and user-uploaded media. If the baseline ever feels stale relative to dev
(it's a snapshot — regenerated via `python3 scripts/gen_baseline.py`),
regenerate it rather than dumping the dev DB.
