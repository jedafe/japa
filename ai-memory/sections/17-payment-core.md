# Section 17 — Payment Core

**Status:** ✅ Locked

## Decided
Provider abstraction: Paystack (NGN), Stripe (USD/international), NOWPayments (crypto), plus Manual Bank/Mobile Money Transfer (reference-code reconciliation, proof-of-payment upload, `pending_verification` status, admin review queue, logged to Audit Log). Future gateway additions (Apple Pay, Google Pay) need zero abstraction-layer changes — they're payment methods inside Stripe/Paystack's existing checkout, not new providers.

Payment intent flow: webhook is the source of truth, never the client-side callback. Idempotency via `payment_webhook_events` (provider, event_id, raw_payload, processed_at). Business logic processed async via `payment_webhook_process` (critical tier), never synchronously in the handler.

`payments` table: polymorphic `purpose_type` (`pro_membership`, `org_boost`, `donation`, `course_access`) + `purpose_id`, nullable `subscription_id`. Recurring billing (Pro membership) via Stripe or Paystack only — never NOWPayments or manual transfer. Course access uses a dual model: included with active Pro subscription, or purchasable à la carte. Organization boosts get their own `organization_boosts` table (duration-bound), feeding into Section 25's ad campaign infrastructure. Refund handling differs by purpose: org boost expires the boost row immediately, course access revokes the grant, Pro membership refunds affect future renewal only (not retroactive access).

All card collection via Stripe Elements / Paystack hosted checkout — raw card data never touches our servers.

## Internal Discount Codes (restored post-audit, see ADR-029)

A first-party discount code system, distinct from the existing affiliate coupon/referral mechanism — different data model, different purpose, no sharing of tables:

```sql
discount_codes (
  id, site_id, code TEXT UNIQUE NOT NULL,
  discount_type TEXT NOT NULL,   -- 'percent' | 'fixed'
  discount_value NUMERIC NOT NULL,
  applicable_to  TEXT NOT NULL,  -- 'course_access' | 'pro_membership' | 'all'
  max_uses INT NULL,             -- null = unlimited
  uses_per_user INT NOT NULL DEFAULT 1,
  group_id FK NULL,              -- null = any user; non-null = scoped to this UserGroup
  valid_from TIMESTAMPTZ NOT NULL,
  valid_until TIMESTAMPTZ NULL,  -- null = no expiry
  created_by FK, created_at
)
discount_code_uses (
  code_id FK, user_id FK, payment_id FK,
  used_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (code_id, user_id)
)
```

Applied at checkout: code is validated (enabled, within date range, user eligible, usage caps not exceeded) before the payment intent is created; the discount is reflected in the `payments.amount` that goes to the provider. `uses_per_user = 1` enforces one-code-one-user for promotional codes. `group_id` lets admin scope a code to a specific UserGroup (e.g. a code only valid for the "Beta Testers" group). Admin creates/revokes codes in the Admin Panel, gated by a `payments.discount_codes.manage` permission node.

## See Also
ADR-012, ADR-029.

## Amendment — Credit as a Payment Provider Value (post-audit)

`payments.provider` gains a fifth value, `credit`, alongside Paystack/Stripe/NOWPayments/manual_transfer — same one-time-purchase-only restriction class as NOWPayments and manual transfer (no recurring `pro_membership` billing via Credit). New `purpose_type` value, `credit_purchase`, covers *acquiring* Credit with real money (Stripe/Paystack only, webhook-driven, same idempotency pattern as every other purpose type).

Spending Credit is architecturally different from every other provider: there's no external party, so it does **not** implement the `Provider` interface — no `Send()`, no webhook to verify. It's a direct, synchronous ledger debit (checked against balance, written to the `payments` row and the Section 28 Credit ledger in the same transaction) at checkout time. See Section 28 for the full ledger design and ADR-012/ADR-024 for rationale.
