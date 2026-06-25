# Section 28 — Wallet & Credits System

**Status:** ✅ Locked

## Decided

Two-tier internal currency: **Points** (soft, activity-earned, non-monetary) and **Credits** (spendable, monetary-equivalent). Conversion is one-way only (Points → Credits, never reversed), at a fixed but admin-configurable global rate, and is user-initiated rather than automatic at the moment a point is earned.

Added post-audit, after V1.0.0's initial sign-off, once a cross-cutting need for V2's Referral System surfaced — elevated into V1 rather than scoped narrowly to one V2 feature, since Courses (Section 17) and the V2 Referral System and Japa Ads both need to consume the same ledger.

### Acquisition sources

- **Purchase.** Real money → Credits directly, via the existing payment provider abstraction (Section 17): a `payments` row with a new `purpose_type` value, `credit_purchase`, provider Stripe or Paystack, fixed exchange rate. Processed exactly like any other webhook-driven payment — webhook is the source of truth, idempotent via `payment_webhook_events`, business effect (crediting the ledger) applied async via Asynq, never synchronously in the handler.
- **Referral rewards.** Credits, granted on the referee's first qualifying *paid* action — not on signup alone, to limit fraud incentive. (V2 Referral System detail.)
- **Activity Points.** Awarded automatically via the Activity Point Rules engine below.

### Activity Point Rules engine

A site-wide, admin-configurable registry: each entry is `activity_key → signed point_value`, plus an enabled flag and an optional per-activity frequency cap (e.g. "max 5 points/day from this activity"). Every feature module (Forum, Content Extensions, Mentorship Matching, Referral, Courses, moderation tooling, etc.) emits a typed activity event referencing an `activity_key` whenever a user performs a trackable action. An Asynq task (default tier) consumes each event, resolves the matching rule, checks the cap if one is set, and writes a signed entry to the Points ledger.

- Point values can be negative (e.g. `content_flagged_removed`, `spam_detected`) as well as positive — same mechanism as positive awards, just a negative `point_value` on the rule.
- Each event carries an idempotency key (e.g. `forum_post:1234`) so a retried event can't double-award.
- Changing a rule's `point_value` applies **forward-only** — historical ledger entries are never retroactively rescored.
- Extensible by construction: adding a new point-worthy activity anywhere on the platform is a new registry entry plus one event emission at the call site, not new engine code.

### Ledger structure

Both Points and Credits use an **immutable transaction log**, never a mutable balance column: `{type, amount, resulting_balance, source, actor, reference, timestamp}`. Current balance is the latest `resulting_balance`, not a separately-maintained counter that can drift from the log.

- Supports reversal/clawback entries (fraudulent referral, refunded course). A clawback may take a user's Credit balance negative rather than being blocked outright — collection/restriction policy stays a manual admin process for now, not an automated block.
- All purchase-triggered grants carry idempotency keys, protecting against Stripe/Paystack webhook retries causing double-crediting — same idempotency discipline as every other webhook-driven effect in the system.

### Expiration

No expiration on either Points or Credits, applied uniformly. Avoids regulatory friction some jurisdictions impose around expiring prepaid value, and keeps the policy consistent regardless of source.

### Spending Credit — bypasses the Provider abstraction

Unlike Paystack/Stripe/NOWPayments, **Credit is not implemented as a `Provider`.** There's no external party to call `Send()` against and no webhook to verify — spending Credit is a direct, synchronous ledger debit checked against balance at checkout time, written to the `payments` row (`provider = "credit"`) and the Credit ledger in the same transaction. `payments` stays the single source of truth for "what was purchased and how," while the Section 28 ledger is the source of truth for "what's my balance and history." See Section 17 for the provider-list amendment.

### Redemption surfaces (V1 scope)

- **Courses** (Section 17): Credit becomes a payment option for à la carte course access, alongside Paystack/Stripe/NOWPayments/manual transfer.
- **Japa Ads** (V2): Credit designed in as a payment option for the flat-rate ad slot fee from the start — see ADR-018 addendum. Does not reopen the flat-rate-vs-CPM/CPC decision; it's an additional payment method for the same flat fee, not a switch to spend-based billing.
- Foundation scholarships and any other future surface can plug into the same generalized ledger without architectural rework — not built in V1, the ledger shape doesn't need to change to support it later.

### Admin tooling

RBAC-gated permission nodes (Section 03) govern manual point/credit adjustment, Activity Point Rule configuration, and conversion rate configuration. Every manual admin adjustment requires a reason field — no silent balance changes.

### Karma/Reputation (folded in, post-audit)

The originally-separate Reputation/Karma concept (points from posts, replies, helpful marks, reactions, affecting "trust level") is the same mechanism as Activity Points above, not a parallel system — there's no second points ledger. Points double as the platform's reputation signal: a user's Points total can be surfaced as a displayed trust/reputation indicator on their profile, and feeds the existing reputation-based fallback tier in onboarding's recommendation algorithm (Section 09).

**Explicitly not automated:** crossing a Points threshold never triggers an automatic RBAC Group change. Group membership (Section 03) is always a manual admin action — admin may look at a user's Points total as one input to that decision, but there is no rule, job, or trigger that promotes a user into a Group based on Points. See ADR-028.

## See Also

ADR-024 (wallet/ledger design), ADR-025 (AI context permission nodes), ADR-028 (Karma fold-in, no auto-promotion), ADR-012 amendment (Credit as a payment provider value), ADR-018 amendment (Japa Ads Credit payment, flat-rate decision unaffected), Section 03 (RBAC) amendment, Section 17 (Payment Core) amendment, Section 09 (reputation-based fallback fill).
