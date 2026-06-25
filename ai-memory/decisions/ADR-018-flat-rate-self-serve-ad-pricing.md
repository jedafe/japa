# ADR-018: Flat-Rate Pricing for Self-Serve Ads Over CPM/CPC (V2)

**Status:** Accepted

**Context:** V2's self-serve Japa Ads feature needed a pricing model. CPM/CPC requires a real-time spend ledger, wallet/credit balance system, and auto-pausing on budget exhaustion — substantially more infrastructure than flat-rate.

**Decision:** Flat-rate pricing (pay a fixed amount for a slot over a duration), reusing the exact payment pattern already built for organization boosts (Section 17) rather than building new billing infrastructure.

**Rationale:** The platform's scale doesn't yet justify the complexity of real-time performance-based billing. Flat-rate is close to free to implement given the org-boost pattern already exists.

**Consequences:** Advertisers don't get spend control tied to actual performance, only to duration. CPM/CPC is a plausible v3+ evolution once real ad demand justifies the added complexity.

**Addendum (post-audit):** Section 28's Wallet & Credits ledger was subsequently added to V1 for unrelated reasons (Referral System rewards, course purchases). This does **not** reopen this decision — the "substantially more infrastructure than flat-rate" rationale above was about CPM/CPC's real-time spend-tracking and auto-pause requirements, not the mere existence of a credit balance. Japa Ads gains Credit as a *payment method* for the same flat fee (debiting the Section 28 ledger instead of charging a card), not a switch to spend-based billing. Revisiting flat-rate vs. CPM/CPC on its own merits remains a v3+ question.
