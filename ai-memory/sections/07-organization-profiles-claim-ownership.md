# Section 07 — Organization Profiles & Claim Ownership

**Status:** ✅ Locked (reconstructed; Verification Badges restored post-audit, see ADR-029)

## Decided
Organization profiles with authorship switching. Claim Ownership system for businesses/organizations to take control of an existing unclaimed listing. Organization boosts (paid featured placement, Section 17) and self-serve ad campaigns (Section 25, v2) both run through the same underlying Ad Placement campaign infrastructure rather than parallel systems.

## Verification Badges (restored post-audit)

A `verified` flag plus `verification_type` (embassy, immigration-lawyer, business, notable-member) on the entity being verified — not Organization-only, since the original intent covers credible individual community members too (e.g. a recognized immigration lawyer posting under their personal profile rather than an org page). Granting/revoking verification is admin-only, gated by an RBAC permission node rather than tied to the Claim Ownership flow — claiming a listing and being verified are separate trust signals, and a claimed-but-unverified org is a normal, expected state. The existing `organization.verified` webhook event (Section 12) covers the org case; the same event fires for verified individual profiles under a parallel `user.verified` event.

## See Also
ADR-029.
