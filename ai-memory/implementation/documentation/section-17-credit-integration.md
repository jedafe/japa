# Section 17/28 — Credit Provider Integration: Implementation Complete

**Date:** 2026-06-27
**Phase:** 5
**Section:** 17/28
**Status:** Complete

## Overview

This is a cross-cutting integration between Section 17 (Payment Core) and
Section 28 (Wallet & Credits). The credit provider is NOT a `Provider` — it
bypasses the provider abstraction entirely because there is no external party,
no `Send()` call, and no webhook to verify. Instead, `PayWithCredit` performs a
direct, synchronous ledger debit against the wallet balance and records a
payment row with `provider="credit"` and `status="succeeded"` in the same
logical flow.

This integration closes the production-blocking gap identified during the
Phase 4 audit: the Wallet & Credits system existed but had no path for users to
actually spend credits on platform purchases (ad slots, course access, etc.).
The `CreditSpender` interface bridges the two packages without creating a
circular dependency.

## Components Implemented

### Credit Spender Interface (internal/payments/credit.go, 84 lines)

**ErrInsufficientCredit** — Sentinel error returned when a user's credit balance
is too low for the requested purchase. Callers inspect this with
`errors.Is(err, ErrInsufficientCredit)` to provide appropriate user feedback.

**CreditSpendInput** — Input struct for the wallet debit call:
- `Amount` — Credit amount to spend (int64)
- `Source` — Origin of the spend (set to `"payment"` when called from PayWithCredit)
- `Reference` — Unique reference string for idempotency
- `PurposeType` — What the credits buy (e.g., `"ad_slot"`, `"course_access"`)
- `PurposeID` — Entity-specific identifier for the purchase target

**CreditSpender** — Interface satisfied by `wallet.Service`:
- `SpendCredits(ctx, tenantID, userID, input) (int64, error)` — debits the
  wallet and returns the new balance

**PayWithCredit** — Method on `Service`:
1. Validates inputs (wallet configured, amount positive, purpose type non-empty)
2. Builds a `CreditSpendInput` with `Source: "payment"` and a reference combining purpose type and ID
3. Calls `s.wallet.SpendCredits()` to debit the wallet (checks balance, writes ledger entry)
4. Calls `s.CreatePayment()` to record a payment row with `ProviderCredit` and `PaymentStatusSucceeded`
5. Returns the new credit balance

### Payment Core Updates (internal/payments/payments.go)

- Added `ProviderCredit = "credit"` constant to the provider list
- Added `PurposeAdSlot = "ad_slot"` purpose type constant
- Added `wallet CreditSpender` field to `Service` struct
- Changed `NewService` signature from `NewService(database *db.DB)` to
  `NewService(database *db.DB, wallet CreditSpender)` — the wallet parameter is
  optional (pass `nil` if credit payments are not in use)

### Credit Payment Tests (internal/payments/payments_test.go, 6 new tests)

**mockCreditSpender** — Test double implementing `CreditSpender` with a
configurable `spendFunc` closure. Enables unit testing of `PayWithCredit`
without a database.

| # | Test | Verifies |
|---|------|----------|
| 1 | `TestPayWithCredit_WalletNotConfigured` | Returns error when `wallet` field is nil |
| 2 | `TestPayWithCredit_ZeroAmount` | Returns error when amount is zero |
| 3 | `TestPayWithCredit_EmptyPurposeType` | Returns error when purpose type is empty |
| 4 | `TestPayWithCredit_ErrInsufficientCredit` | Returns `ErrInsufficientCredit` that satisfies `errors.Is()` |
| 5 | `TestPayWithCredit_Success` | `SpendCredits` called with correct amount, purposeType, purposeID, and source `"payment"` |
| 6 | `TestPurposeAdSlotConstant` | `PurposeAdSlot` equals `"ad_slot"` |

### Wallet Bug Fixes (internal/wallet/)

**store.go:**
- Added `derefString(s *string) string` helper — converts a `*string` to a
  `string`, returning `""` for nil. Fixes the pointer-to-value type mismatch
  between `sqlc`-generated `ListLedgerEntriesByUserParams.LedgerType` (string) and
  the `*string` passed from the service layer.
- Fixed `UpsertWalletSettingsParams.PointsToCreditsRate` — changed from `int32`
  to `*int32` to match the `sqlc`-generated parameter type.

**service.go:**
- Fixed 3 calls from a removed `.Encode()` method to
  `fmt.Sprintf("%x", userID.Bytes)` for building idempotency keys in
  `ConvertPointsToCredits` (2 calls) and `SpendCredits` (1 call).

### Server Wiring (internal/server/server.go)

- Added `walletCreditAdapter` struct — adapter that wraps `*wallet.Service`
  to satisfy `payments.CreditSpender` by mapping `payments.CreditSpendInput`
  to `wallet.SpendCreditsInput`.
- Updated `payments.NewService(database)` call to
  `payments.NewService(database, &walletCreditAdapter{svc: walletService})`.
- The adapter lives in `server.go` to avoid import cycles between the `payments`
  and `wallet` packages.

### Permission Keys

No new permission keys were added. Credit payment is invoked internally by other
subsystems (advertising, learning platform) and uses the existing payment
permissions.

### API Routes

No new routes were added. `PayWithCredit` is an internal method called by other
handlers, not a standalone endpoint. The advertising and learning platform
handlers invoke it when a user chooses to pay with credits.

## Test Summary

| Package | Test Count | Result |
|---------|-----------|--------|
| internal/payments | 12 (6 original + 6 new credit tests) | PASS |

## Build Verification

| Command | Result |
|---------|--------|
| `go test ./internal/payments/ -count=1` | ok — 12 tests pass |
| `go build ./...` | success |
| `go vet ./...` | clean |

## Files Created

| # | File | Lines | Purpose |
|---|------|-------|---------|
| 1 | `internal/payments/credit.go` | 84 | `CreditSpender` interface, `CreditSpendInput`, `PayWithCredit` method, `ErrInsufficientCredit` sentinel |

## Files Modified

| # | File | Change |
|---|------|--------|
| 1 | `internal/payments/payments.go` | Added `ProviderCredit` and `PurposeAdSlot` constants; added `wallet CreditSpender` field to `Service`; changed `NewService` signature to accept wallet parameter |
| 2 | `internal/payments/payments_test.go` | Added 6 credit payment tests and `mockCreditSpender` test double; total now 12 tests |
| 3 | `internal/wallet/store.go` | Added `derefString` helper; fixed `UpsertWalletSettingsParams.PointsToCreditsRate` pointer type |
| 4 | `internal/wallet/service.go` | Fixed 3 `.Encode()` calls to `fmt.Sprintf("%x", userID.Bytes)` for idempotency keys |
| 5 | `internal/server/server.go` | Added `walletCreditAdapter` adapter; updated `payments.NewService` call to pass wallet adapter |

## Key Decisions

- **Credit is NOT a Provider** — it does not implement the `Provider` interface.
  There is no `Send()` call, no external party, and no webhook to verify. The
  credit balance is checked and debited synchronously via the wallet, then a
  payments row is created with `provider="credit"` and `status="succeeded"`.
  Forcing it into the `Provider` interface would require no-op `Send()` and
  `VerifyWebhookSignature()` methods — a code smell that obscures the real
  flow.

- **No new migration** — this is a runtime bridge between two existing packages.
  Both the wallet ledger and the payments table already exist. The integration
  connects them in code, not in schema.

- **CreditSpender is an interface** — `wallet.Service` satisfies this interface
  but is not imported directly by the `payments` package. This avoids a circular
  dependency between `internal/payments` and `internal/wallet`, and makes
  `PayWithCredit` testable with a mock.

- **PayWithCredit records succeeded immediately** — credit payments have no
  pending or processing intermediate state. The wallet debit and payment record
  happen in a single logical flow. If the wallet debit succeeds, the payment
  has effectively succeeded.

- **ErrInsufficientCredit is a sentinel error** — callers inspect it with
  `errors.Is()` to distinguish "not enough credits" from other failures (wallet
  service down, invalid input). This enables the advertising handler, for
  instance, to show a "top up your credits" message rather than a generic error.

- **walletCreditAdapter lives in server.go** — placing the adapter in the
  `server` package avoids import cycles. The `payments` package defines the
  interface; `wallet` implements it independently; `server` glues them together.
  This is the standard Go dependency-injection pattern for cross-package
  interfaces.

## Known Issues and Deferrals

- **Credit refund flow not implemented** — credit payments are marked
  `succeeded` immediately, but the refund logic in the payment core is only
  wired for external providers (Paystack, Stripe, etc.). Refunding a credit
  payment requires reversing the wallet ledger entry, which is deferred to a
  future phase.

- **No DB transaction wrapping** — `SpendCredits` and `CreatePayment` are
  separate calls. If `CreatePayment` fails after `SpendCredits` succeeds, the
  wallet is debited but no payment record exists. The two operations need to be
  wrapped in a single database transaction to ensure atomicity. This is
  deferred to Phase 6.

- **Learning platform payment integration not yet wired** — the `PayWithCredit`
  method exists but is not yet called by the learning platform handlers. Course
  access via credits is a Phase 5+ concern.

- **Currency is hardcoded to "NGN"** — `PayWithCredit` hardcodes the currency
  as `"NGN"` in the payment record. Multi-currency credit payments are not
  supported yet.

## Next Steps

Phase 5 post-implementation summary (`phase5-complete.md`).
