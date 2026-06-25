# ADR-025: AI Context Visibility Gated by RBAC Permission Nodes, Not a New System

**Status:** Accepted

**Context:** JapaBot's Honcho-backed persistent context (Section 20) is being extended to draw on a user's site activity — Points/Credits history, course progress, mentorship activity, forum activity, referral activity, moderation history — but admin needs control over which categories feed the AI per User Group. The initial framing treated this as a new "UserGroup-scoped visibility" system to be designed from scratch; it is in fact already covered by Section 03's existing model (User Groups → User Roles → Permissions, no hardcoded checks, every access decision resolves through a permission node at runtime).

**Decision:** A new permission node family, `ai.context.*` — one node per activity category (`ai.context.points_balance`, `ai.context.course_progress`, `ai.context.mentorship_activity`, `ai.context.forum_activity`, `ai.context.referral_activity`, `ai.context.moderation_history`). JapaBot's Honcho context builder checks the relevant node at runtime before including each category in a given user's AI context. All `ai.context.*` nodes default to **unassigned/off** for every Group, including the `members` and `super_admin` system defaults — there is no default-grant exception for any Group; every node is granted manually.

**Rationale:** No new schema or access-control system is needed — this is the exact pattern already established for Page Builder access (`content.page_builder.create`) and language pack management. Building a separate "UserGroup visibility" system would duplicate an existing, working mechanism for no benefit, and would violate the project's own corrected-mid-planning lesson from ADR-003 (don't hardcode or special-case access; resolve through the dynamic permission system). Defaulting every node to off rather than on follows the platform's existing NDPR/GDPR-conscious posture (Section 19) — a category should require an explicit admin decision to expose, not be silently on by default.

**Consequences:**
- No new tables for "AI visibility scoping" — six (or more, as categories grow) new permission node entries in the existing RBAC registry.
- Every future site-activity category that should be AI-context-eligible needs its own `ai.context.*` node defined at design time, following the existing "new feature checklist" in `ai-memory/skills/rbac-permission-node-pattern.md`.
- A freshly created custom User Group starts with zero AI visibility into any category until admin opts categories in — consistent with how a new Group has no permissions at all until explicitly granted.
