# Recurring Pro/Business Billing — Design Spec

**Date:** 2026-09-24
**Status:** Approved for implementation
**Governing docs:** JEOS §17 (payment-core), §47 (public API scopes/tiers), ADR-012
**Related:** `docs/superpowers/specs/2026-09-24-agent-data-api-phase3-design.md` (tier gates that make billing meaningful)

## Context

The platform has the full subscription *surface* but not the *loop*:

- `subscription_plans` table is seeded (free/pro/business, NGN amounts) and `GET /pricing/plans` serves it.
- `subscriptions` table has the full lifecycle shape (`provider_sub_id`, `cancel_at_period_end`, `current_period_end`, statuses `trialing|active|past_due|canceled|cancelled|expired`).
- `POST /api/v1/subscriptions` exists but returns `not_implemented` — the frontend's subscribe button is a dead end.
- `POST /payments/intent` with `purpose_type=pro_membership` can collect a one-time payment, but `onPaymentSucceeded` never grants the tier — a paying user stays `regular` forever.
- Pro-tier gates already exist and work: `search.semantic`, `rag.search`, `GET /me/brief`. They are enforced against group-derived tier (`internal/tiers`), so billing only has to move group membership.

**Decision (user, 2026-09-24):** implement full recurring billing per §17 — Stripe and Paystack only (never NOWPayments, manual transfer, or Credit — none can auto-charge). Pro gating stays as-is; no new premium surfaces this round.

## Non-goals

- New pro-gated features (the gates already exist; this is the purchase path).
- Proration, plan upgrades/downgrades mid-cycle (V1: cancel + resubscribe).
- Provider customer-portal integration (Stripe Billing Portal) — defer.

## Design

### 1. Provider subscription creation

Both providers use a **hosted checkout** model — no card data touches us.

**Stripe** (`internal/payments/provider.go` `StripeProvider`):

- Pre-created Price objects are referenced by settings:
  `payments.stripe.price.pro_monthly`, `payments.stripe.price.pro_yearly`,
  `payments.stripe.price.business_monthly`, `payments.stripe.price.business_yearly`.
- `CreateSubscription` → `POST /v1/checkout/sessions` with
  `mode=subscription`, `line_items[0][price]={price_id}`, `customer_email`,
  and `subscription_data.metadata={user_id, plan}` (metadata lands on the
  Subscription object — it is what webhooks can resolve back to us).
- Returns `checkout_url`; the row is NOT created locally yet (§17: webhook
  is the source of truth — deferred creation avoids a pending-status
  lifecycle and unpaid rows).

**Paystack** (`PaystackProvider`):

- Pre-created Plan objects referenced by settings:
  `payments.paystack.plan.pro_monthly`, `payments.paystack.plan.pro_yearly`,
  `payments.paystack.plan.business_monthly`, `payments.paystack.plan.business_yearly`.
- `CreateSubscription` → `POST /transaction/initialize` with
  `email`, `amount`, `plan={plan_code}`, `metadata={user_id, plan}`.
  Paystack auto-creates the subscription on the first successful charge
  and fires `subscription.create`.
- Returns `authorization_url` as `checkout_url`.

**Service signature change:** `CreateSubscription` returns
`(SubscriptionCheckout{CheckoutURL string}, error)` — there is no
local row to return yet. `POST /subscriptions` responds `201 {data: {checkout_url}}`.
The frontend redirects to `checkout_url` instead of showing inline success.

One active subscription per user: if `GetActiveSubscription` returns a row,
create returns `409 already_subscribed` (V1 upgrade path is cancel+resubscribe).

### 2. Webhook event model

Subscription lifecycle events don't carry a payment `provider_ref`, so they
can't ride the existing `payment_webhook_process` task unchanged. A new Asynq
task `subscription:webhook_process` (§11 catalog addition, critical tier,
same retry policy as `payment_webhook_process`) receives:

```json
{"provider": "stripe|paystack", "event_id": "...", "event_type": "...",
 "subscription_ref": "sub_...|SUB_...", "payload": {...raw event data}}
```

The HTTP webhook handlers already verify signatures before enqueue; they gain
an event-type branch: subscription events → `subscription:webhook_process`,
payment events → `payment_webhook_process` (unchanged).

Event mapping:

| Provider | Event | Action |
|---|---|---|
| Stripe | `checkout.session.completed` (mode=subscription) | create `subscriptions` row (status from subscription object, `provider_sub_id`, period bounds from `current_period_*`); grant tier |
| Stripe | `invoice.paid` (billing_reason=subscription_cycle) | extend `current_period_end`; insert renewal `payments` row (`purpose_type=pro_membership`, `subscription_id`); ensure tier (recovers missed activation) |
| Stripe | `invoice.payment_failed` | status → `past_due` (keep tier during provider retry window) |
| Stripe | `customer.subscription.updated` | sync `cancel_at_period_end`, `current_period_end` |
| Stripe | `customer.subscription.deleted` | status → `canceled`, `cancelled_at`; downgrade tier |
| Paystack | `subscription.create` | create row (status `active`, `provider_sub_id` = subscription_code); grant tier |
| Paystack | `charge.success` carrying `subscription_code` | renewal: extend period (from `next_payment_date` on the subscription object when present, else +interval), payments row, ensure tier |
| Paystack | `invoice.payment_failed` / `invoice.create` failed | status → `past_due` |
| Paystack | `subscription.not_renew` | `cancel_at_period_end=true` |
| Paystack | `subscription.disable` | status → `canceled` at `current_period_end`; downgrade when reached |

Idempotency: `payment_webhook_events` already dedupes by (provider, event_id);
the subscription worker records the same way so retries are safe.

### 3. Tier grant / revoke seam

`payments.Service` gets a `TierManager` seam (same pattern as
`SetEnqueuer`/`SetProvider`):

```go
type TierManager interface {
    SetUserTier(ctx context.Context, userID pgtype.UUID, tier string) error
}
```

wired to `tiers.Service` in server.go. Grant on activation/renewal-ensure;
revoke to `regular` on `expired`/`canceled`-at-period-end. Revocation only
fires when the tier the subscription granted is still the user's tier —
an admin-granted `business` is never clobbered by an expired `pro` row
(check resolved tier == granted tier before downgrading).

### 4. Cancel / reactivate

- `POST /me/subscription/cancel` → provider call first, then local flag:
  - Stripe: `POST /v1/subscriptions/{id}` `cancel_at_period_end=true`
  - Paystack: `POST /subscription/disable` (Paystack has no
    cancel-at-period-end — disable stops renewals; the local row keeps
    `active` + `cancel_at_period_end=true` until `current_period_end`,
    entitlement unchanged meanwhile)
- `POST /me/subscription/reactivate` → Stripe unset `cancel_at_period_end`;
  Paystack `POST /subscription/enable`.
- Provider call failure → 502 `provider_error`, no local state change.

### 5. Expiry sweep (webhook-miss safety net)

New periodic task `subscription:expiry_sweep` (§11): every N minutes
(`subscription.expiry_sweep_interval_minutes`, default 30) —

- `active`/`past_due` rows where `current_period_end < now() - grace`
  (`subscription.past_due_grace_hours`, default 72) → `expired`, downgrade tier.
- `active` rows with `cancel_at_period_end` past `current_period_end` →
  `expired`, downgrade tier.
- `past_due` within grace → keep (provider may still recover).

### 6. Settings catalog additions (JEOS-first)

| Key | Default | Purpose |
|---|---|---|
| `payments.stripe.price.pro_monthly` | — | Stripe Price ID |
| `payments.stripe.price.pro_yearly` | — | Stripe Price ID |
| `payments.stripe.price.business_monthly` | — | Stripe Price ID |
| `payments.stripe.price.business_yearly` | — | Stripe Price ID |
| `payments.paystack.plan.pro_monthly` | — | Paystack plan_code |
| `payments.paystack.plan.pro_yearly` | — | Paystack plan_code |
| `payments.paystack.plan.business_monthly` | — | Paystack plan_code |
| `payments.paystack.plan.business_yearly` | — | Paystack plan_code |
| `subscription.past_due_grace_hours` | `72` | Grace before downgrade |
| `subscription.expiry_sweep_interval_minutes` | `30` | Sweep cadence |

Missing price/plan settings for a requested (provider, plan) pair →
`503 plan_not_configured` — fail loud, never silently grant.

### 7. Frontend

`SubscriptionSection.svelte`: on a `checkout_url` field in the create
response, `window.location.href = checkout_url` (hosted checkout). Return
lands on `/settings/subscription?subscribed=1`; the section shows a pending
state until the webhook activates (`GET /me/subscription` poll or
"check again" — webhook latency is seconds).

### 8. Route catalog additions

```
POST /api/v1/subscriptions/checkout-complete — no: hosted checkout returns
   to the frontend URL; no backend callback needed (webhook is truth).
```

No new public routes — `POST /subscriptions`, `GET /me/subscription`,
cancel/reactivate all exist. Response shape change on create:
`{data: {checkout_url}}` replaces the never-shipped subscription row shape.

## Failure modes

- Webhook lost → expiry sweep is the net; user-side "refresh status" not needed V1.
- Double webhook → deduped by `payment_webhook_events`.
- Subscription event for unknown ref → logged + 200 (never error out a provider retry).
- Provider API down during cancel → 502 to user, no state divergence.

## Testing

- Unit: plan→price/plan-code settings resolution, event→transition mapping,
  downgrade-safety (never clobber a different tier), cancel flows.
- Smoke: `JC_ALLOW_UNSIGNED_WEBHOOKS=true` in dev + crafted provider payloads
  to drive the full lifecycle without real provider keys.
