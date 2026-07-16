---
name: jc-backend-develop
description: Use when building new features, endpoints, services, or entities in the JapaConnect Go backend (github.com/jedafe/japa). Covers the layered architecture (Handlers→Services→Stores), the Spec-Readiness and Contract-Delivery Gates, full RBAC CRUD delivery per entity, sqlc/pgx patterns, and catalog maintenance.
---

# JapaConnect Backend — Development Skill

Go monolith serving all five frontend apps at `/api/v1`. Module: `github.com/jedafe/japa`.

## ⛔ GATE 1 — SPEC-READINESS (Run Before Any Implementation)
Before writing code for any feature:
1. Locate the governing JEOS section doc and/or ADR for the feature.
2. If no spec exists, or the spec conflicts with what's being asked: STOP and output:
```
⚠️ MISSING/CONFLICTING SPEC — implementation paused
Feature: [name]
Issue: [no JEOS section covers this | conflicts with ADR-0XX | conflicts with section XX]
Action needed: create/update the JEOS spec (and ADR if architectural) before backend work continues.
```
Architectural decisions made mid-implementation without an ADR are spec drift — the exact failure mode JEOS audits exist to catch.

## ⛔ GATE 2 — CONTRACT-DELIVERY (The Mirror of the Frontend Gates)
The frontend agents STOP when routes are missing. Your job is to make sure they never stop. **Every new entity ships with the complete contract or it doesn't ship:**

1. **Full RBAC CRUD route set** (non-negotiable — super-admin must be able to manage every entity):
   - `GET /api/v1/admin/[entity]` — list with `?page=&limit=&sort=&q=&filter=` (offset pagination + meta for admin tables)
   - `GET /api/v1/admin/[entity]/:id` · `POST` create · `PATCH /:id` update · `DELETE /:id`
   - **Both directions of every state transition**: `POST /:id/suspend` AND `/:id/unsuspend`, `/ban` AND `/unban`, `/approve` AND `/reject`, etc.
   - Bulk endpoints where the DataTable spec needs them: `POST /api/v1/admin/[entity]/bulk` with `{action, ids}`
   - Readonly/log entities still get `DELETE /:id` and `POST /clear` (all / older-than)
2. **RBAC permission nodes seeded** — one node per action (`[entity].list`, `.view`, `.create`, `.update`, `.delete`, `.suspend`, `.ban`, …) added to the seed migration AND registered in the RBAC middleware. *Regression anchor BE-005: 174 of 249 nodes were once missing from seed. Never partially seed.*
3. **Catalogs updated in the same PR** — `api-routes-catalog.md` (new routes), `valkey-keys.md` (new `jc:` keys), `system-settings.md` (new keys), `asynq-tasks.md` (new tasks). An uncatalogued route is invisible to the frontend gates and therefore doesn't exist.

If any part of the contract can't be delivered (e.g. spec ambiguity on a state transition): STOP + output:
```
⛔ INCOMPLETE CONTRACT — implementation paused
Entity: [entity]
Undeliverable: [e.g. unclear whether orgs support suspend; bulk action semantics undefined]
Action needed: [decision required]
```

## Architecture (Layered — Never Skip Layers)
```
Handler (Chi)  →  Service (business logic)  →  Store (sqlc-generated)  →  PostgreSQL 18
     ↓                    ↓
Middleware chain     Asynq enqueue (side effects)
```
- **Handlers**: parse/validate input, call ONE service method, render response. No business logic, no direct store calls.
- **Services**: business logic, transaction boundaries, orchestration across stores, enqueue Asynq tasks.
- **Stores**: sqlc-generated only. Hand-written SQL goes in `.sql` query files → `sqlc generate`. Never string-concatenate SQL.
- **Middleware order**: RequestID → RealIP → Logger → Recoverer → Tenant → Auth → RBAC → RateLimit → Handler.

## Stack (Non-Negotiable)
Go (latest stable) · Chi router · sqlc + pgx/v5 (pool via `pgxpool`) · Asynq (Valkey-backed, three tiers: critical/default/low) · PostgreSQL 18 · Valkey (keys namespaced `jc:`) · Meilisearch 1.45 · SeaweedFS (S3 API) · imgproxy · Honcho (AI memory, separate logical DB, same PG instance) · GlitchTip via sentry-go · golang-migrate · `slog` structured logging · env vars `JC_` prefixed, typed config, missing-required exits at boot.

## Response Contract (What the Frontends Consume)
- Success: `RenderJSON(w, status, data)` · Lists: `RenderPaginated(w, data, meta{total,page,limit})` or cursor meta for feeds
- Errors: `RenderError(w, status, code, message)` — machine code + human message; the frontend surfaces `message`
- Validation errors: 422 with per-field map · AuthN 401 · AuthZ 403 · not found 404 — never 500 for expected conditions
- All timestamps RFC 3339 UTC · IDs as strings in JSON

## Mandatory Patterns
- **Wallet/credits**: append-only immutable ledger, balance = SUM; every spend inside a tx holding `pg_advisory_xact_lock(user_id)`. Never a mutable balance column. *(Regression anchor: double-spend defect.)*
- **Render-time resolution (ADR-052)**: coupon embeds/affiliate links store entity IDs only; resolve codes/URLs/expiry at render time; fail closed on inactive.
- **Idempotency**: webhook handlers and payment callbacks guard with Valkey `SET NX` on an idempotency key.
- **Uploads**: MIME sniffed from content (not extension), size-capped, stored to SeaweedFS, served via imgproxy URLs.
- **Tenancy**: tenant middleware resolves host → tenant_id into context; stores filter by it.
- **GDPR/NDPR erasure**: user deletion cascades through owned data AND calls the Honcho deletion API — required, not best-effort.
- **Search**: writes that affect searchable entities enqueue a Meilisearch sync task (default queue) — never sync inline.
- **Errors**: wrap with `fmt.Errorf("...: %w", err)`; unexpected errors captured to GlitchTip with request context. *(Regression anchor BE-002: GlitchTip must actually be wired.)*
- **TipTap content**: stored as JSON, rendered to HTML server-side, cached in Valkey.

## Testing Expectations
Table-driven tests for services · `httptest` for handlers (auth + RBAC denial cases included) · a failing-permission test per new permission node · store tests against a real PG (dockertest or test DB) for tx-sensitive logic (wallet, claims).

## Deliverables Per Task
1. Gate 1 + Gate 2 output · 2. migration pair if schema changes (invoke `jc-backend-migrate`) · 3. sqlc queries + `sqlc generate` · 4. store/service/handler code · 5. routes registered + RBAC nodes seeded · 6. Asynq tasks (invoke `jc-backend-worker`) · 7. all catalogs updated · 8. tests · 9. `go build ./... && go vet ./...` clean (run locally via Claude Code — no Go toolchain in chat sessions).
