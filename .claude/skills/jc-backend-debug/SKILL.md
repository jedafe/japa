---
name: jc-backend-debug
description: Use when reviewing or debugging JapaConnect Go backend code (github.com/jedafe/japa). Diagnoses the BE-series defect classes, pgx/sqlc pitfalls, transaction misuse, Asynq wiring, Valkey key issues, and layered-architecture violations.
---

# JapaConnect Backend — Review & Debugging Skill

Conventions and gates: `jc-backend-develop`. Run checks in order.

## 0. Known Defect Classes First (BE-Series Regression Anchors)
These shipped once — check them before anything else when symptoms match:
| Anchor | Symptom | Check |
|---|---|---|
| BE-001 noopEnqueuer | Tasks silently never run (emails, search sync) | Prod wiring injects the REAL Asynq client, not the noop test double |
| BE-002 GlitchTip unwired | Errors vanish, no reports | sentry-go initialized at boot AND error paths actually capture |
| BE-003 migration runner | Schema corrupted after deploy | Runner globs `.up.sql` only; `.down.sql` never applied forward |
| BE-00x wallet race | Negative balances / double-spend | Spend path holds `pg_advisory_xact_lock(user_id)` inside the tx |
| BE-005 RBAC seed gap | 403 on routes that should work | Every permission node the middleware checks exists in seed |
| BE-00x config JSON | JSON-typed settings come back zero-valued | `config.NewSettings` decode path for JSON-encoded values |

## 1. Layer Violations
Handler containing business logic or direct store calls · service importing `net/http` · store with hand-concatenated SQL (must be sqlc) · middleware order broken (Tenant before Auth before RBAC).

## 2. pgx / sqlc Pitfalls
- Missing `rows.Close()` / not using sqlc iterators → pool exhaustion under load
- `pgx.ErrNoRows` not distinguished from real errors → 500 where 404 belongs
- Nullable columns scanned into non-pointer/non-`pgtype` fields → silent zero values
- Queries edited in generated `.go` files instead of `.sql` sources (lost on regen)
- `sqlc generate` not re-run after query/schema change → drift between SQL and structs
- Context not propagated (`context.Background()` inside request path kills cancellation)

## 3. Transaction Misuse
- Tx opened in handler instead of service · multiple stores mutated without a shared tx
- `defer tx.Rollback(ctx)` missing after Begin · commit-then-enqueue vs enqueue-then-commit: side-effect tasks enqueue AFTER commit (or use outbox), else tasks fire for rolled-back data
- Advisory locks taken outside the tx (`pg_advisory_lock` vs correct `pg_advisory_xact_lock`)

## 4. HTTP Contract
- Errors leaking internals (`err.Error()` straight to client) instead of `RenderError` code+message
- 500 for expected conditions (missing row, validation) · pagination meta absent on list routes
- Mutating routes missing RBAC middleware — check EVERY POST/PATCH/DELETE has a node check
- Input structs without validation · IDs from URL params unvalidated

## 5. Asynq
Task enqueued but type not registered in the mux (silent dead-letter) · wrong queue tier (payment task on `low`) · non-idempotent handler that retries with side effects · payload struct changed without version tolerance for in-flight tasks.

## 6. Valkey
Keys outside `jc:` namespace · cache set without TTL (unbounded growth) · cache invalidation missing on the write path (stale render cache) · `SET NX` idempotency key without expiry (permanently locked).

## 7. Tenancy & Security
Store queries missing tenant_id filter (cross-tenant leak) · uploads trusting file extension over sniffed MIME · secrets in logs · rate limiting absent on auth/expensive routes.

## Output Protocol
1. **Root cause** — one sentence · 2. **Broken code** · 3. **Fixed code** (surgical, minimal) · 4. **Secondary findings** · If the root cause matches a BE-series class, say so explicitly — it feeds the regression list.
