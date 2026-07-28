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
- **Middleware order**: RequestID → RealIP → Logger → Recoverer → Tenant → GeoIP → VisitorContext → Auth → RBAC → RateLimit → Handler.
  - **GeoIP** (Section 37): MaxMind GeoLite2 in-process, attaches `GeoContext{CountryCode, City, ...}` to ctx.
  - **VisitorContext** (ADR-064): runs immediately after GeoIP; synthesizes `GeoContext` + `user_profiles.field_values` (`japa_status`, `destination_country`) + resolved language into `VisitorContext{OriginCountry, CurrentCountry, DestinationCountry, JapaStage, Archetype, ContentCountry, Confidence}`. Cached at `jc:visitor:ctx:{session_id}` (30-min TTL). Gated by `system_settings.geo_personalization.enabled`.

## Stack (Non-Negotiable)
Go (latest stable) · Chi router · sqlc + pgx/v5 (pool via `pgxpool`) · Asynq (Valkey-backed, three tiers: critical/default/low) · PostgreSQL 18 · Valkey (keys namespaced `jc:`) · Meilisearch 1.45 · SeaweedFS (S3 API) · imgproxy · Honcho (AI memory, separate logical DB, same PG instance) · GlitchTip via sentry-go · golang-migrate · `slog` structured logging · env vars `JC_` prefixed, typed config, missing-required exits at boot.

## Response Contract (What the Frontends Consume)
- Success: `RenderJSON(w, status, data)` · Lists: `RenderPaginated(w, data, meta{total,page,limit})` or cursor meta for feeds
- Errors: `RenderError(w, status, code, message)` — machine code + human message; the frontend surfaces `message`
- Validation errors: 422 with per-field map · AuthN 401 · AuthZ 403 · not found 404 — never 500 for expected conditions
- All timestamps RFC 3339 UTC · IDs as strings in JSON

## Mandatory Patterns
- **Wallet/credits**: append-only immutable ledger, balance = SUM; every spend inside a tx holding `pg_advisory_xact_lock(user_id)`. Never a mutable balance column. *(Regression anchor: double-spend defect.)*
- **Revenue splits (ADR-063)**: `revenue_splits` is an **immutable audit record** — append-only, never UPDATE. Each paid enrollment or Pro-stream accrual writes one row with the exact percentages/amounts applied at the time. `university:process_enrollment` is **idempotent on payment_id** (guard against double-credit, cf. AUDIT-BE-005). Payout execution (`university:send_instructor_payout`) mirrors the wallet double-spend guard: advisory lock on instructor_id inside the tx, stamp splits with `payout_id` only after transfer succeeds.
- **Render-time resolution (ADR-052)**: coupon embeds/affiliate links store entity IDs only; resolve codes/URLs/expiry at render time; fail closed on inactive.
- **Idempotency**: webhook handlers and payment callbacks guard with Valkey `SET NX` on an idempotency key.
- **Geo-aware personalization (ADR-064)**: `VisitorContext.ContentCountry` is the default for all `?geo=` surfaces (trending §53, search §13, feed §32, discovery). Explicit `?geo=` always wins; absent param → `ContentCountry`. Feed score gets a **bounded** geo-relevance multiplier (cap so it re-weights, never dominates). Search passes `ContentCountry` as a boost/optional-filter on the `relevant_countries` Meilisearch attribute. **Never hard-filter by geo** — re-weight only. User override: `user_profiles.content_country_preference` (nullable ISO code) wins over resolved. Killswitch: `system_settings.geo_personalization.enabled`.
- **University space type (Section 39)**: universities are `space_type = 'university'` spaces with the `university_fields` custom field group. Auto-seeded on first deploy from `db/seed/universities.json`; logo fetch via `university:fetch_logo` (low tier). Admin CRUD + CSV bulk import. Discovery routes at `/api/v1/universities` (public) and `/api/v1/admin/universities` (admin). The `logo` field doubles as the university page's profile image (Section 34 media + imgproxy).
- **Uploads**: MIME sniffed from content (not extension), size-capped, stored to SeaweedFS, served via imgproxy URLs.
- **Tenancy**: tenant middleware resolves host → tenant_id into context; stores filter by it.
- **GDPR/NDPR erasure**: user deletion cascades through owned data AND calls the Honcho deletion API — required, not best-effort. **Japa University**: `instructor_profiles`, `revenue_splits`, and `instructor_payouts` are user-owned — erasure must cascade through them. `revenue_splits` are financial audit records: anonymize the instructor reference, never delete the split row (regulatory retention).
- **Search**: writes that affect searchable entities enqueue a Meilisearch sync task (default queue) — never sync inline. Content documents gain a `relevant_countries` filterable attribute (array of ISO codes from geo spaces/tags) for ADR-064 search weighting.
- **Errors**: wrap with `fmt.Errorf("...: %w", err)`; unexpected errors captured to GlitchTip with request context. *(Regression anchor BE-002: GlitchTip must actually be wired.)*
- **TipTap content**: stored as JSON, rendered to HTML server-side, cached in Valkey.

## Testing Expectations
Table-driven tests for services · `httptest` for handlers (auth + RBAC denial cases included) · a failing-permission test per new permission node · store tests against a real PG (dockertest or test DB) for tx-sensitive logic (wallet, claims).

## Japa University Entities (ADR-063 — Section 36 extension)
When building Japa University creator-economy features, the entity set is:
- **`instructor_profiles`** — one per user per tenant. `status ∈ pending|approved|rejected|suspended` (gated application). `tier ∈ standard|verified|partner`. Optional `commission_pct_override`. `payout_method` (jsonb, encrypted at app layer). `kyc_status`. Full RBAC CRUD + both-direction state transitions (approve/reject, suspend/unsuspend).
- **`revenue_splits`** — immutable per-event record (`creator_pct`/`platform_pct`, amounts, `source ∈ purchase|pro_stream`). Links to `enrollment`/`payment` in and `payout` out. **Append-only — never UPDATE.**
- **`instructor_payouts`** — one per instructor per period; gross/commission/net; `status ∈ pending|processing|paid|failed`. State transitions: `pending→processing→paid` or `→failed`.
- **`learning_tracks` columns**: `instructor_id` (null = platform course), `price_kobo` (null = free/Pro-only), `currency`, `is_published_to_university` (marketplace visibility, distinct from `is_published`), `category`, `language`, `total_enrolled` (cached counter).
- **`enrollments` column**: `payment_id` (nullable FK → `payments`; null for free/Pro/pro_stream).
- **Course reviews**: reuse polymorphic `reviews` table (Section 29) with `entity_type = 'learning_track'`. No `course_reviews` table.
- **Migration**: `000166` (Japa University tables + columns), `000167` (`user_profiles.content_country_preference` for geo).
- **5 new Asynq tasks**: `university:process_enrollment` (critical), `university:calculate_period_payouts` (scheduled, low), `university:send_instructor_payout` (critical), `university:notify_instructor_approved` (default), `university:notify_course_approved` (default). Plus `university:fetch_logo` (low) for university space seeding.
- **12 new RBAC nodes**: `university.instructor.*`, `university.course.*`, `university.payout.*` — seed completely (BE-005).
- **7 new settings**: `university.default_commission_pct`, `university.verified_tier_commission_pct`, `university.partner_tier_commission_pct`, `university.pro_stream_rate_kobo`, `university.payout_min_threshold_kobo`, `university.payout_period_days`, `university.surface_domain`.
- **4 new Valkey keys**: `jc:visitor:ctx:{session_id}` (30-min TTL, shared with geo), `jc:uni:instructor:{user_id}` (instructor profile cache), `jc:uni:payout:period:{instructor_id}:{period}` (payout lock), `jc:uni:enrollment:processed:{payment_id}` (idempotency guard).

## Deliverables Per Task
1. Gate 1 + Gate 2 output · 2. migration pair if schema changes (invoke `jc-backend-migrate`) · 3. sqlc queries + `sqlc generate` · 4. store/service/handler code · 5. routes registered + RBAC nodes seeded · 6. Asynq tasks (invoke `jc-backend-worker`) · 7. all catalogs updated · 8. tests · 9. `go build ./... && go vet ./...` clean (run locally via Claude Code — no Go toolchain in chat sessions).
