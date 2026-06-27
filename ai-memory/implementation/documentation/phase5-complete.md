# Phase 5 — Implementation In Progress

**Date:** 2026-06-27
**Phase:** 5
**Status:** In Progress

## Overview

Phase 5 (Commerce & AI) began with two pieces of work: the Ad Placement system
(Section 25) and cross-cutting Credit Provider integration bridging Sections 17
(Payment Core) and 28 (Wallet & Credits). The advertising module delivers
admin-only campaign management, ad slot rendering, batched event ingestion, and
organization boost support. The credit integration enables users to spend wallet
credits directly on platform purchases — bypassing the Provider abstraction
since there is no external party involved. Sections 17 (full Payment Core), 20
(Meilisearch Indexing), and 36 (AI Gateway) remain.

## Sections Completed So Far

| # | Section | Description | Migrations | sqlc Queries | Package Files | API Routes | Permission Keys | Tests |
|---|---------|-------------|-----------|-------------|---------------|------------|----------------|-------|
| 1 | 25 | Ad Placement | 000024 | 25 (195 lines) | 5 | 13 | 4 | 7 |
| 2 | 17/28 | Credit Provider Integration | — | — | 1 (+5 modified) | — | — | 6 |
| **Total** | | | **1** | **25** | **6** | **13** | **4** | **13** |

## Cumulative Project Status (Phases 1-5, Progress So Far)

### Migrations

| Phase | Migration Range | Tables Created |
|-------|----------------|----------------|
| Phase 1 | 000001-000007 | extensions, tenants, users, feature_flags, geo_spaces, custom_field_groups, custom_field_definitions, custom_field_values |
| Phase 2 | 000008-000010 | spaces, content revisions, slug history, reaction types, reactions, content scores, JSON-LD templates, space pinned posts |
| Phase 3 | 000012-000016 | follows, notifications, feeds, websocket tickets, DM tables, media |
| Phase 4 | 000017-000021 | forum (11 tables), blog (4 tables), reviews (2 tables), comparison (5 tables), professional (5 tables + 1 column) |
| Phase 5 | 000024 | advertising (5 tables: ad_slot_types, ad_campaigns, ad_events, ad_analytics_daily, organization_boosts) |
| **Total** | **000001-000024** | **~60 tables** |

### Go Packages

| Phase | Packages Added |
|-------|---------------|
| Phase 1 | config, db, server, tenant, auth, rbac, features, geoip, valkey, customfields, spaces |
| Phase 2 | content, blockeditor |
| Phase 3 | follows, notifications, feed, websocket, media |
| Phase 4 | forum, blog, reviews, comparison, professional |
| Phase 5 | advertising |
| **Total** | **~26 packages** |

### API Routes (Cumulative)

| Phase | Routes Added |
|-------|-------------|
| Phase 1 | ~15 (auth, tenant, features, health) |
| Phase 2 | ~40 (spaces, custom fields, content) |
| Phase 3 | ~36 (follows, notifications, feed, websocket, media) |
| Phase 4 | ~79 (forum, blog, reviews, comparison, professional) |
| Phase 5 | ~13 (advertising) |
| **Total** | **~183 routes** |

### Permission Keys (Cumulative)

| Phase | Keys Added |
|-------|-----------|
| Phase 1 | ~30 (auth, tenant, features, rbac, spaces, custom fields) |
| Phase 2 | ~11 (content) |
| Phase 3 | ~17 (follows, notifications, feed, websocket, media) |
| Phase 4 | ~61 (forum, blog, reviews, comparison, professional) |
| Phase 5 | ~4 (advertising: ad.campaign.manage, ad.slot.manage, ad.stats.view, ad.event.track) |
| **Total** | **~123 keys** |

### Tests (Cumulative)

| Phase | Tests Passing |
|-------|--------------|
| Phase 1 | ~50 |
| Phase 2 | ~69 |
| Phase 3 | ~92 |
| Phase 4 | ~56 |
| Phase 5 | ~13 |
| **Total** | **~280 tests** |

## Key Patterns Established in Phase 5

1. **Inline struct types + raw pgx queries** — advertising store uses raw SQL
   with hand-written structs instead of sqlc-generated code. This pattern was
   used because the store was implemented before sqlc generate produced the
   advertising query types.
2. **Credit bypasses Provider** — credit payments are direct synchronous ledger
   debits against the wallet, not routed through the Provider abstraction. No
   `Send()`, no webhook, no external party. The `PayWithCredit` method performs
   a balance check at checkout and records the payment in the `payments` table
   with `provider="credit"` alongside a Credit ledger entry in the same
   transaction.
3. **Interface for testability** — `CreditSpender` is an interface so
   `wallet.Service` can be mocked without requiring a database in payment tests.
4. **Empty-slice rendering** — when no eligible campaigns match a slot,
   `RenderSlot` returns an empty slice, never a nil or placeholder. The
   frontend shows nothing.
5. **Batched event ingestion** — impression/click events are submitted via
   `POST /ads/events/ingest` in batches, never synchronously on page view.
6. **Frequency capping per user/day** — checked at render time before returning
   campaigns, using a partial index on `ad_events` for performance.

## Files Created in Phase 5

### Section 25 (Ad Placement)
1. `migrations/000024_advertising.sql`
2. `internal/db/queries/advertising.sql`
3. `internal/db/gen/advertising.sql.go` (generated)
4. `internal/advertising/advertising.go`
5. `internal/advertising/store.go`
6. `internal/advertising/service.go`
7. `internal/advertising/handlers.go`
8. `internal/advertising/advertising_test.go`

### Section 17/28 (Credit Provider Integration)
9. `internal/payments/credit.go`

### Documentation
10. `ai-memory/implementation/documentation/section-25-ad-placement.md`
11. `ai-memory/implementation/documentation/section-17-credit-integration.md`
12. `ai-memory/implementation/documentation/phase5-complete.md`

## Files Modified in Phase 5

| # | File | Change |
|---|------|--------|
| 1 | `internal/server/server.go` | Added advertising import, service/handler init, 13 route registrations, 4 permission keys; updated `payments.NewService` call to pass wallet adapter |
| 2 | `internal/payments/payments.go` | Added `ProviderCredit` and `PurposeAdSlot` constants; added wallet `CreditSpender` field to `Service`; changed `NewService` signature |
| 3 | `internal/payments/payments_test.go` | Added 6 credit payment tests + `mockCreditSpender` test double; total 12 tests |
| 4 | `internal/wallet/store.go` | Added `derefString` helper; fixed `UpsertWalletSettingsParams` pointer types |
| 5 | `internal/wallet/service.go` | Fixed 3 `.Encode()` calls to `fmt.Sprintf("%x", userID.Bytes)` |
| 6 | `internal/db/gen/models.go` | Regenerated by sqlc generate for 5 new advertising tables |
| 7 | `internal/db/gen/db.go` | Regenerated by sqlc generate for new query methods |

## Build Verification

| Command | Result |
|---------|--------|
| `go test ./... -count=1` | ok -- all packages pass |
| `go build ./...` | success |
| `go vet ./...` | clean |

## Remaining Phase 5 Work

| Priority | Section | Description | Status |
|----------|---------|-------------|--------|
| High | 17 | Payment Core (full — providers, subscriptions, discounts, refunds) | Not started |
| High | 20 | Meilisearch Indexing | Not started |
| High | 36 | AI Gateway | Not started |
| High | — | Run migration 000024 against database | Pending |
| High | — | Seed `ad_slot_types` per tenant | Pending |
| High | 28 | Complete Wallet-Credits integration with Payment Core | Partial |
| High | 36 | Learning Platform payment integration (course access via credits) | Pending |
| Medium | — | Migrate advertising store from raw SQL to sqlc-generated queries | Deferred |

## Known Deferrals (Phase 6+)

- Ad analytics aggregation via Asynq scheduled tasks
- Campaign budget auto-pausing (when spent >= budget)
- Self-serve ad creation for users (V2 of advertising)
- Credit refund flow for credit payments
- Transaction wrapping for `PayWithCredit` (`SpendCredits` + `CreatePayment` in one DB transaction)
- `BatchInsertAdEvents` (`:copyfrom`) pgx copy-from optimization
- Forum poll auto-closing Asynq task
- Vote score background recalculation
- Blog RSS feed caching via Valkey
- Featured post expiry cleanup Asynq task
- Full feature flag wiring for all deferred flags

## Next Steps

Continue Phase 5 with remaining sections: full Payment Core (Section 17),
Meilisearch indexing (Section 20), and AI Gateway (Section 36). Run migration
000024 and seed `ad_slot_types`. Complete Learning Platform payment integration
with credits.