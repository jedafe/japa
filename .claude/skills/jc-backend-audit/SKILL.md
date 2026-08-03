---
name: jc-backend-audit
description: Use when performing a comprehensive quality audit of the JapaConnect Go backend before merge or release. Reconciles routes/nodes/catalogs against code, runs the BE-series regression checklist, and verifies security, tenancy, and GDPR erasure completeness.
---

# JapaConnect Backend — Audit Skill

Work systematically. The backend audit is primarily a **reconciliation exercise**: code vs catalogs vs seeds vs spec. Stat drift between JEOS docs and actual code is a known recurring issue — always report actual counts.

## A. Route ↔ Catalog ↔ RBAC Reconciliation (Run First)
Build three sets and diff them:
1. **Routes in code** — walk every Chi route registration
2. **Routes in `api-routes-catalog.md`**
3. **Permission nodes in seed** vs **nodes checked by middleware per route**

Findings:
- Route in code, missing from catalog → High (frontend gates can't see it)
- Route in catalog, missing from code → Critical (frontend will 404 — output the ⚠️ MISSING BACKEND API ROUTES block for the frontend team's awareness)
- Mutating route without RBAC middleware → **Critical security finding**
- Node checked by middleware but absent from seed → Critical (BE-005 class)
- Node in seed never checked anywhere → Low (dead node, flag for cleanup)

## B. RBAC CRUD Completeness Matrix (Mirror of jc-admin-audit §A)
Per entity: List / Get / Create / Update / Delete / state-transitions-both-directions / bulk / (readonly: delete+clear). Any gap = Critical — this is what makes the frontend's non-negotiable CRUD possible.

## C. BE-Series Regression Checklist (Every Audit, No Exceptions)
- [ ] BE-001: prod DI wires real Asynq enqueuer; grep for noopEnqueuer outside tests
- [ ] BE-002: GlitchTip initialized at boot; error paths capture; test event fires
- [ ] BE-003: migration runner ignores `.down.sql` in forward runs
- [ ] Wallet: every spend path holds `pg_advisory_xact_lock` inside its tx; ledger append-only (no UPDATE on transactions table)
- [ ] BE-005: node count in seed == node count referenced by middleware (report both numbers)
- [ ] Config: JSON-encoded settings decode correctly (round-trip test exists)

## D. Migration Integrity
Every `.up.sql` has a correct `.down.sql` twin · numbering contiguous · no edits to applied migrations (checksum/history check) · schema table count vs JEOS-stated ~158 v1 + ~20 v2 planned = ~178 — report actual, flag drift · circular FK (users↔spaces) still resolved via stub+ALTER pattern. **Japa University**: migration `000166` adds 3 tables (`instructor_profiles`, `revenue_splits`, `instructor_payouts`) + columns on `learning_tracks`/`enrollments`; migration `000167` adds `user_profiles.content_country_preference`. Verify `revenue_splits` has no UPDATE-enabling constructs (append-only, wallet-ledger philosophy).

## E. Asynq Reconciliation
Enqueue call sites vs registered task types vs `asynq-tasks.md` (JEOS catalog: 95 total = 75 v1 shipped + 20 v2 planned; 4 v1 tasks extended for v2 — report actual vs catalog) · every task idempotent or justified · queue tiers sensible (payments/auth/university-payouts=critical, search sync/emails/university-notifications=default, digests/cleanup/payout-calc/university-logo=low) · retry+backoff configured · dead-letter monitoring wired. **Japa University tasks**: `university:process_enrollment` (idempotent on payment_id), `university:calculate_period_payouts` (scheduled), `university:send_instructor_payout` (idempotent, advisory-locked), `university:notify_instructor_approved`, `university:notify_course_approved`, `university:fetch_logo`. **Geo**: `maintenance:update_geoip_db` (weekly, atomic reader swap).

## F. Valkey & Settings Reconciliation
Keys in code vs `valkey-keys.md` (35 patterns stated) · all `jc:` namespaced · TTLs on every cache key · invalidation paired with every cached read's write path · `system_settings` keys in code vs catalog (187 total = 125 v1 + 62 v2 planned — report actual shipped vs catalog). **Geo**: `jc:visitor:ctx:{session_id}` (30-min TTL) — verify cache invalidation on `content_country_preference` change. **Japa University**: `jc:uni:instructor:{user_id}`, `jc:uni:payout:period:{instructor_id}:{period}`, `jc:uni:enrollment:processed:{payment_id}` — verify idempotency guard TTLs. **Settings**: `geo_personalization.enabled` (killswitch), `university.*` (7 keys), `university.surface_domain`.

## G. Security
sqlc-only data access (grep for `Exec(`/`Query(` with concatenated strings) · input validation on every handler · MIME sniffing on uploads · rate limiting on auth + expensive routes · secrets absent from logs/responses · tenant_id filtering on every tenant-scoped store method (sample-test cross-tenant access) · webhook signature verification + idempotency.

## H. GDPR/NDPR Erasure Completeness
User deletion cascades through every user-owned table (enumerate against schema) AND calls the Honcho deletion API (required) · erasure task idempotent and resumable · exports cover the same surface. **Japa University**: `instructor_profiles`, `revenue_splits`, `instructor_payouts` are user-owned — erasure must cascade. `revenue_splits` are financial audit records: **anonymize the instructor reference, never delete the split row** (regulatory retention). `payout_method` (jsonb) is sensitive — encrypted at app layer, follow Section 19 handling rules.

## I. Contract Quality
List routes return pagination meta · error responses use code+message shape · timestamps RFC 3339 UTC · IDs stringified · 4xx/5xx semantics correct (no 500 for expected conditions).

## J. Geo-Aware Personalization (ADR-064)
- `VisitorContext` resolver wired after `GeoIPMiddleware` in the middleware chain · `ContentCountry` resolves correctly (override > profile > IP) · cached at `jc:visitor:ctx:{session_id}` (30-min TTL) with invalidation on preference change.
- Feed geo boost (§32) is **bounded** — verify cap prevents local content from dominating engagement/velocity signals. Search `relevant_countries` attribute populated on content sync. Trending/discovery routes default to `ContentCountry` when `?geo=` absent.
- `geo_personalization.enabled` killswitch works — when false, all surfaces fall back to global ranking.
- Archetype market lists (origin vs destination countries) are in config, not hardcoded in Go source.
- Guest homepage geo rendering is SSR (no client flicker) and degrades gracefully when `GeoContext` is absent.

## Report Format
```
## Audit Report — Backend (github.com/jedafe/japa)
**Status:** PASS / NEEDS FIXES
### Critical (block deployment) / High / Medium / Low
### Reconciliation Counts (actual vs JEOS-stated)
| Routes: [n] vs catalog ~454 | Permission nodes: seed [n] vs checked [n] vs stated 249+ |
| Tables: [n] vs 148 | Asynq tasks: [n] vs 72 | Valkey patterns: [n] vs 35 | Settings keys: [n] vs 85 |
### RBAC CRUD Matrix
[entity × routes table — include Japa University: instructor_profiles, revenue_splits, instructor_payouts, universities (space type)]
### BE-Series Regression Results
[checklist with pass/fail]
### Compliance Summary
| Route/RBAC/catalog reconciliation | ✅/❌ |
| CRUD completeness | ✅/❌ |
| BE regressions | ✅/❌ |
| Migrations | ✅/❌ |
| Asynq | ✅/❌ |
| Valkey/settings | ✅/❌ |
| Security | ✅/❌ |
| GDPR erasure | ✅/❌ |
| Geo-aware personalization (ADR-064) | ✅/❌ |
| Japa University revenue/payouts (ADR-063) | ✅/❌ |
```
## K. V2 Audit Checklist (Run When Backend Phases 41–47 Are In Progress)

### K1. Catalog Completeness (Gate pre-condition)
Before any v2 phase is marked complete, verify the four catalogs reflect the new code:
- [ ] `api-routes-catalog.md` — all new routes present
- [ ] `asynq-tasks.md` — all new tasks registered (not just planned)
- [ ] `valkey-keys-catalog.md` — all new patterns present with TTLs
- [ ] `system-settings-catalog.md` — all new keys with defaults
- [ ] `permission-key-naming.md` — all new RBAC nodes present; seeded to super_admin

### K2. V2 Schema Integrity
- [ ] `ai_personas` table: `user_id` is UNIQUE + FK users; `is_bot=true` on backing account; `created_by` set to admin who provisioned
- [ ] `platform_events.is_synthetic` column present; materialized views in §40 filter `WHERE NOT is_synthetic`
- [ ] `content_embeddings` table: HNSW index `idx_content_embeddings_hnsw` created (not plain btree); `chunk_index` + UNIQUE constraint per entity chunk
- [ ] `checklist_progress`, `assessment_results`, `saved_scholarships`, `studio_jobs`, `studio_batches`, `remittance_providers`, `remittance_rates`, `remittance_rate_alerts`, `job_applications`, `badge_definitions`, `user_badges`, `challenge_definitions`, `user_challenge_progress`, `content_imports`, `travel_providers`, `device_tokens`, `referral_codes`, `referrals`, `newsletter_subscribers`, `newsletter_campaigns`, `newsletter_unsubscribe_tokens` — all present in migrations
- [ ] `forms.form_type` discriminator + `forms.scoring_config` column added (backward-compatible: existing rows default to `form_type='standard'`)
- [ ] `comparison_items.is_baseline` column added; Lagos seeded with `is_baseline=true` under `cost_of_living` type
- [ ] `cost_of_living` comparison type seeded with 9 cost attributes

### K3. V2 Task Idempotency
- [ ] `content:generate_embedding` extended — idempotent on (entity_type, entity_id, chunk_index); upsert not insert
- [ ] `content.generate_draft` extended — idempotent on `studio_job_id`; guard against re-generation of an already-generated job
- [ ] `assessment:score_submission` — idempotent on `submission_id`
- [ ] `referral:check_conversion` — idempotent on `payment_id`; fraud checks before transition
- [ ] `notification:send_push` — handles invalid-token feedback (sets device_tokens.is_active=false)
- [ ] `remittance:fetch_rates` — upsert pattern (new row per fetch, no UPDATE of existing); cache invalidated after insert

### K4. High-Stakes Gate (ADR-068 §5, ADR-076 §3, ADR-077 §3)
- [ ] Guide categories `visa`, `banking`, `grants`, `health_insurance`, `admission` — studio jobs for these ALWAYS enter content_reviews regardless of `studio.auto_publish_types` setting
- [ ] Persona participation actions targeting content in high-stakes categories ALWAYS route to `content_reviews` regardless of `autonomy_mode = 'autonomous'`
- [ ] These checks are in task code (not config) — verify they cannot be bypassed via settings

### K5. Synthetic Activity Exclusion
- [ ] All persona-dispatched actions (autonomous or approved-queued) write `platform_events.is_synthetic = true`
- [ ] `mv_daily_content_stats` and other §40 materialized views filter `WHERE NOT is_synthetic`
- [ ] Admin has a raw/combined toggle — verify this does NOT affect the default filtered view
