---
name: jc-backend-refactor
description: Use when updating or refactoring existing JapaConnect Go backend code. Enforces layer extraction, sqlc consolidation, N+1 removal, transaction boundary correction, and error-wrapping hygiene — behavior-preserving only.
---

# JapaConnect Backend — Updating & Refactoring Skill

Conventions and gates: `jc-backend-develop`. Behavior-preserving; one-line comment per change. If the refactor reveals a missing route/node/catalog entry, run Gate 2 and flag it — refactoring is when contract gaps get caught.

## 1. Layer Extraction
Business logic found in handlers → extract to service methods · direct store calls in handlers → route through services · duplicated logic across services → shared internal package (not util-grab-bag; name by domain).

## 2. sqlc Consolidation
- Hand-written SQL strings → `.sql` query files + `sqlc generate`
- Per-row queries in loops (N+1) → single query with `= ANY($1)` / join, or sqlc `:copyfrom` for bulk inserts
- SELECT * in query files → explicit column lists (schema changes stop silently breaking scans)

## 3. Transaction Boundaries
Multi-store mutations without shared tx → service-level `pgx.BeginFunc` wrapping · side-effect enqueues moved AFTER commit · wallet/credit paths verified for `pg_advisory_xact_lock` (never weaken this while refactoring).

## 4. Error Hygiene
`errors.New(fmt.Sprintf(...))` → `fmt.Errorf("...: %w", err)` · sentinel comparisons → `errors.Is/As` · swallowed errors (`_ =`) → handled or explicitly justified · GlitchTip capture on unexpected paths.

## 5. Context & Config
`context.Background()` in request paths → propagated ctx with timeouts on external calls (Meilisearch, SeaweedFS, Honcho, imgproxy) · magic numbers → typed config from `JC_` env or system_settings.

## 6. Concurrency
Unbounded goroutines per request → Asynq tasks or bounded worker pools · shared maps without sync → restructure or `sync.Map` only where justified · goroutines without ctx cancellation → leak fixes.

## 7. Post-Refactor Verification
`sqlc generate` diff-clean · `go build ./... && go vet ./...` · existing tests pass untouched (behavior-preserving means tests don't change unless they encoded the defect) · catalogs still accurate if any route/task/key moved.
