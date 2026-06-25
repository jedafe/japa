# ADR-024: Two-Tier Points/Credits Wallet With Immutable Ledger

**Status:** Accepted

**Context:** V2 planning for the Referral System surfaced a need for a reward currency. Rather than build a referral-specific credit balance, the same need was recognized to apply to Courses (à la carte access) and V2's Japa Ads (flat-rate ad slot payment), plus admin's stated requirement for a general activity-reward system ("points can be awarded automatically through site activities... robust enough to cover all activities site-wide"). Since V1.0.0 implementation hadn't started, this was elevated out of V2 and into V1 as foundational infrastructure rather than built once, narrowly, inside Referral.

**Decision:** A two-tier currency — Points (soft, activity-earned) and Credits (spendable, monetary-equivalent) — with one-way Points→Credits conversion, backed by an immutable transaction-log ledger rather than a mutable balance column. Points are awarded automatically via an admin-configurable Activity Point Rules registry (`activity_key → signed point_value`), driven by typed events emitted from any feature module, rather than requiring manual admin assignment per occurrence.

**Rationale:**
- **Two tiers, not one.** Purchased Credit is a prepaid monetary liability needing precise, auditable tracking. Points are a softer engagement currency that only becomes a liability once converted. Collapsing them into a single currency would force purchase-grade audit rigor onto every gamification point award, or conversely treat real money with the casualness appropriate to a points system.
- **Immutable ledger over balance column.** Real money is involved (via Credit purchase). A transaction log supports audits, disputes, and reversals; a mutable balance column does not, and can silently drift from reality under concurrent writes.
- **Event-driven Activity Points, not manual-only.** An admin-assign-every-point model doesn't scale to "robust enough to cover all activities site-wide" — it would require an admin action for every single qualifying event, which is infeasible at any real volume. An event/rule registry makes adding a new point-worthy activity a configuration change (new rule + one event emission at the call site), not new engine code.
- **One-way conversion, user-initiated.** Reversible conversion adds complexity (what's the buy-back rate, can it be gamed) with no clear benefit. User-initiated (rather than automatic-on-earn) gives users visibility and control over when their soft currency becomes spendable.
- **No expiration, either tier.** Avoids regulatory friction some jurisdictions impose around expiring prepaid value, and keeps policy uniform rather than needing different rules per source.

**Consequences:**
- Every feature module that wants to award/deduct points must emit a typed activity event with an idempotency key, rather than writing directly to a balance — this is a new integration point for Forum, Content Extensions, Mentorship Matching, Referral, Courses, and moderation tooling.
- Admin gets a new configuration surface (Activity Point Rules) and a new RBAC permission family for managing it — see Section 28 and the Section 03 (RBAC) amendment.
- Credit purchase reuses the existing Section 17 payment provider abstraction and webhook-idempotency pattern as a new `purpose_type` (`credit_purchase`) rather than introducing a parallel payment path.
- Credit *spend* deliberately does **not** go through the `Provider` interface (no external party, no webhook) — see Section 28 for the explicit carve-out, so a future contributor doesn't try to force-fit a no-op `CreditProvider`.
- A clawback/reversal can take a Credit balance negative; this is accepted rather than blocked, with collection policy left to manual admin process for now.
