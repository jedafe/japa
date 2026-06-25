# Section 03 — RBAC (Dynamic Three-Tier)

**Status:** ✅ Locked

## Decided
User Groups → User Roles → Permissions. Fully admin-configurable except three system defaults: `super_admin`, `members`, `guest`. No hardcoded role checks anywhere in the codebase — every access decision resolves through a permission node at runtime. This was explicitly corrected mid-planning when an earlier instinct treated some access (e.g. Page Builder) as fixed-role rather than dynamic-permission.

## See Also
ADR-003. Pattern reused throughout: Page Builder (`content.page_builder.create`), Blog Extension (`blog.*` granular nodes), language pack management (scoped via join table, not a global permission).

## Amendment — AI Context Permission Nodes (post-audit)

New `ai.context.*` node family gates which site-activity categories feed JapaBot's Honcho-backed persistent context per User Group (e.g. `ai.context.points_balance`, `ai.context.course_progress`, `ai.context.mentorship_activity`, `ai.context.forum_activity`, `ai.context.referral_activity`, `ai.context.moderation_history`). All default unassigned/off, including for `members` **and `super_admin`** — every node is granted manually by an admin, with no default-grant exception for any Group, system-default or otherwise. No new schema; this is the existing pattern applied to a new domain. See ADR-025, Section 20 (AI Core), Section 28 (Wallet & Credits).

## Amendment — Group Membership Is Never Points-Automated (post-audit)

Section 28's Points (Karma) total is never a trigger for Group membership changes. All Group assignment, including any future "promote trusted users" workflow, is a manual admin action — there is no automated rule, job, or threshold check anywhere in the codebase that moves a user between Groups based on their Points balance. See ADR-028.
