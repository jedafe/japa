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
Every `.up.sql` has a correct `.down.sql` twin · numbering contiguous · no edits to applied migrations (checksum/history check) · schema table count vs JEOS-stated 145 — report actual, flag drift · circular FK (users↔spaces) still resolved via stub+ALTER pattern.

## E. Asynq Reconciliation
Enqueue call sites vs registered task types vs `asynq-tasks.md` (JEOS says 68 — report actual) · every task idempotent or justified · queue tiers sensible (payments/auth=critical, search sync/emails=default, digests/cleanup=low) · retry+backoff configured · dead-letter monitoring wired.

## F. Valkey & Settings Reconciliation
Keys in code vs `valkey-keys.md` (34 patterns stated) · all `jc:` namespaced · TTLs on every cache key · invalidation paired with every cached read's write path · `system_settings` keys in code vs catalog (49 stated) vs seed.

## G. Security
sqlc-only data access (grep for `Exec(`/`Query(` with concatenated strings) · input validation on every handler · MIME sniffing on uploads · rate limiting on auth + expensive routes · secrets absent from logs/responses · tenant_id filtering on every tenant-scoped store method (sample-test cross-tenant access) · webhook signature verification + idempotency.

## H. GDPR/NDPR Erasure Completeness
User deletion cascades through every user-owned table (enumerate against schema) AND calls the Honcho deletion API (required) · erasure task idempotent and resumable · exports cover the same surface.

## I. Contract Quality
List routes return pagination meta · error responses use code+message shape · timestamps RFC 3339 UTC · IDs stringified · 4xx/5xx semantics correct (no 500 for expected conditions).

## Report Format
```
## Audit Report — Backend (github.com/jedafe/japa)
**Status:** PASS / NEEDS FIXES
### Critical (block deployment) / High / Medium / Low
### Reconciliation Counts (actual vs JEOS-stated)
| Routes: [n] vs catalog [n] | Permission nodes: seed [n] vs checked [n] vs stated 249 |
| Tables: [n] vs 145 | Asynq tasks: [n] vs 68 | Valkey patterns: [n] vs 34 | Settings keys: [n] vs 49 |
### RBAC CRUD Matrix
[entity × routes table]
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
```
