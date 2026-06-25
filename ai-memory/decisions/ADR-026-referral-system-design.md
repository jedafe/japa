# ADR-026: Referral System — Qualifying Action, Fraud Controls, and Visibility

**Status:** Accepted

**Context:** V2's Referral System needed a reward mechanism and abuse-resistant tracking model. The reward side resolved into Section 28's Wallet & Credits ledger (ADR-024) once that was elevated to V1; this ADR covers the remaining referral-specific decisions: what triggers a reward, how attribution works, fraud resistance, reward structure depth, and what a referrer can see about who they referred.

**Decision:**
- **Qualifying action:** the referee's first *paid* action, not signup. Signup alone is too easy to farm for low marginal cost.
- **Attribution:** 30-day window, first-click — the first referral link a prospective user clicks within the window gets credit, not the last one before conversion.
- **Reward:** Credits, granted via the Section 28 ledger on qualifying-action completion.
- **Expiration:** none — consistent with the platform-wide no-expiration policy on Credits (ADR-024).
- **Fraud resistance:** same-device/IP/payment-method self-referral detection, plus a per-period cap on rewarded referrals per user. Referral activity above the cap, or flagged by the self-referral check, routes to manual admin review rather than auto-crediting or hard-blocking.
- **Structure:** single-level only. A refers B; B's own subsequent referrals do not reward A. No multi-level/MLM-style chain.
- **Visibility:** the referrer sees their referral count and each referral's status (pending/qualified/credited), never the referee's PII — consistent with the NDPR/GDPR-conscious posture already established (Section 19).

**Rationale:**
- Gating on a paid action rather than signup ties the cost of the reward directly to revenue generated, which is the core defense against farming.
- First-click attribution is simpler to implement and reason about than multi-touch models, and is the industry-standard default for referral programs specifically (distinct from marketing attribution generally, where last-touch is more common).
- Single-level avoids the regulatory exposure some jurisdictions attach to multi-level referral/commission structures, and avoids meaningfully more complex payout-chain logic for a feature whose primary purpose is community growth, not a revenue-share business line.
- Capping rather than hard-blocking suspicious activity keeps false positives from locking out legitimate power-referrers; routing to manual review is consistent with how the platform already treats ambiguous-risk situations elsewhere (e.g. payment manual-transfer verification).
- Visibility limited to count/status (not referee identity) avoids creating a backdoor into another user's profile/activity via the referral relationship.

**Consequences:**
- Referral tracking needs a `referrals` table keyed on referral code, with referee attribution resolved at signup time from a 30-day cookie/URL-param window, and a status field (`pending` → `qualified` → `credited` or `flagged`).
- The self-referral and cap checks run before crediting, not after — a flagged referral never silently auto-credits and gets reversed later; it sits in `flagged` for admin review from the start.
- Reward crediting reuses Section 28's Activity Point Rules / ledger-write pattern rather than a referral-specific crediting path.
