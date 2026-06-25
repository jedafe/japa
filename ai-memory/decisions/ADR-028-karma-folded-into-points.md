# ADR-028: Karma/Reputation Folded Into Points — No Automated Group Promotion

**Status:** Accepted

**Context:** An original v1 feature, the Reputation/Karma System (points from posts, replies, helpful marks, reactions, driving a "trust level" and *automatic* User Group upgrades — e.g. auto-promotion to "Trusted Member" at a point threshold), was locked early in the project's planning history but lost during a later reconstruction pass and never made it into the final v1.0.0 docs. It resurfaced during V2 planning when Section 28's Activity Points engine was designed — the two systems are conceptually the same mechanism (signed points awarded for site activity), raising the question of whether they should be unified, and if so, whether Section 28 should also inherit the original automatic-Group-upgrade behavior.

**Decision:** Unify them — there is one Points ledger (Section 28), not two. It serves double duty as both the wallet-conversion currency (Points → Credits) and the platform's reputation/trust signal (displayed on profile, feeding the reputation-based fallback tier in onboarding's recommendation algorithm, Section 09). Explicitly **not** carried over: automatic Group promotion based on a Points threshold. Group membership remains a fully manual admin action in all cases, with no rule, job, or threshold check that moves a user between Groups automatically.

**Rationale:** A single ledger avoids the confusion and duplicated bookkeeping of two parallel points systems answering "how much has this user contributed," especially once Section 28 already has the richer infrastructure (immutable ledger, Activity Point Rules registry, idempotent event-driven awarding) that the original Karma concept never had. Rejecting automatic Group promotion was an explicit admin preference: Group creation and membership is something admin wants to control directly rather than delegate to a formula, consistent with the project's broader pattern of admin retaining direct control over consequential platform state changes (e.g. manual transfer payment verification, manual erasure review) rather than fully automating them.

**Consequences:**
- No second `karma` or `reputation` table — Section 09's reputation-based fallback and any future profile-facing trust display both read from the Section 28 Points ledger.
- If automatic trust-tier promotion is wanted later, it would need a separate, explicit ADR reopening this decision — it is not a natural extension to silently add, given this ADR exists specifically to rule it out.
- Profile UI work (displaying a Points/reputation indicator) is a Section 23 (Main Site Frontend) concern, reading Section 28 data, not a new backend system.
