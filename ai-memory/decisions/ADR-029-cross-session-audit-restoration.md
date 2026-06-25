# ADR-029: Cross-Session Audit — Restored Features (Reconstruction Gap Resolution)

**Status:** Accepted

**Context:** Four earlier planning sessions established v1.0.0's architecture, but a subsequent reconstruction pass (session 4) — which itself documented memory degradation mid-session — produced documents that skimmed or omitted a material number of previously-locked features. This ADR records what was found missing and restored in the post-audit pass triggered when the gap was discovered. It is not a set of new decisions; these are restorations of things that were locked earlier and should have been in the docs all along.

**Restored features (by section):**

**Section 07 — Organization Profiles & Claim Ownership**
- Verification Badges: `verified` flag + `verification_type` (embassy, immigration-lawyer, business, notable-member) on both orgs and individual profiles; admin-only grant/revoke via RBAC permission node; `user.verified` webhook event added alongside the existing `organization.verified`.

**Section 08 — Forum Q&A / Solved** (was a one-sentence stub)
- Nested thread structure (not flat), configurable edit window with visible `edited` flag.
- Voting: upvote/downvote on threads/replies (not emoji reactions — that's for other content types). One vote per user per item, changing vote updates the existing row.
- Tag system with moderators: tags are first-class entities with description, rules, cover image, follower count, and assignable moderators (including AI agents per Section 20).
- Polls: attachable to a thread, with live vote-count updates via the existing WS `space_activity`/`feed_update` channels (no new mechanism).
- Anonymous posting: per-thread, identity withheld from other users but always attributable to admins/moderators; excluded from author's public profile activity.
- Bookmarks: `bookmark_count` on threads, with user-side folder organization.
- Reply view counts: admin-switchable, with maintenance archival on toggle rather than deletion.

**Section 09 — Onboarding Flow**
- Product tour (Driver.js or equivalent): contextual walkthrough triggered after the five-step wizard, highlighting primary nav / Space switcher / post-creation. Dismissible; "Replay tour" link in profile settings.

**Section 10 — Real-Time (WebSockets)**
- Direct Messaging: 1:1 conversations using the `dm_message` event type. `dm_conversations` and `dm_messages` tables. Message requests for non-mutual-follow senders. Per-user DM privacy setting (`everyone` / `followers only` / `nobody`); mentions bypass privacy since they're not new conversations. Typing indicators reuse existing `typing` event type.

**Section 14 — Content Type System** (was a two-sentence stub)
- JSON-LD template per content type: admin-defined templates in `content_type_jsonld_templates`, field-interpolated at render time — no code change needed when adding a new content type.
- Geo classification: `geo_space_id` on all content types for geo-filtered discovery.
- Revisions: `content_revisions` table, every save creates a revision, live row = latest revision.
- Scheduling: shared `publish_at` + `scheduled_publish` task across all content types.
- Polymorphic engagement counts: `view_count`, `comment_count`, `share_count`, `reshare_count` on `contents`.
- Polymorphic reactions: `reaction_types` (admin-configurable emoji set) + `reactions` table for all non-forum content; forum threads/replies use upvote/downvote instead.
- Polymorphic bookmarks: `bookmark_count` + user folder organization following the same entity_type/entity_id pattern.
- `comparison` content type added to built-in v1 type list (backing Section 30).

**Section 15 — Block Editor (TipTap)**
- Mention edge cases: DM privacy bypass, block relationship suppression, erased-account render fallback `@[deleted user]`, Space membership gate on private-Space mentions.

**Section 17 — Payment Core**
- Internal Discount Codes: `discount_codes` + `discount_code_uses` tables. `discount_type` (percent/fixed), `applicable_to`, `max_uses`, `uses_per_user`, `group_id` scoping, date range, `payments.discount_codes.manage` permission node. Distinct from affiliate coupons/referral credits — different tables, different purpose.

**Section 18 — i18n / Language Packs**
- Locale scope corrected: V1 is regional English variants only (`en-NG` default, `en-US`, `en-GB`, `en-CA`). True additional-language translation deferred. The earlier erroneous `/fr/...` reference has been replaced with the correct decision.

**Section 21 — Control Panel**
- Database health tooling: table bloat, dead-tuple ratio, long-running queries (`pg_stat_activity`), manual VACUUM trigger (Asynq maintenance-tier task), configurable slow-query threshold. Gated by `platform.db.health_view` / `platform.db.vacuum_trigger` permission nodes.

**Section 23 — Main Site Frontend**
- Dark Mode: system-default + Light/Dark/System toggle in user dropdown; server-side preference (follows user across devices); CSS custom properties / Tailwind v4 dark class, no reload needed.
- Timezone-aware display: UTC storage, user-timezone display, browser-detected default, event/scheduled-post dual-timezone tooltip.

**Section 24 — Blog Extension**
- Blog Series schema: `blog_series` + `blog_series_posts` tables, `/blog/series/[slug]` route, next/prev navigation per post in series, multi-series support, active/archived series status.

**Section 28 — Wallet & Credits (Karma fold-in)**
- Karma/Reputation system folded into Points ledger — no second ledger. Points balance surfaces as reputation/trust signal on profile and feeds onboarding's reputation-based fallback tier. No automated Group promotion (see ADR-028).

**New Section 29 — Universal Ratings & Reviews**
- `reviews` + `review_aggregates` tables. Polymorphic (Organizations, Universities, Services, etc.). One review per user per entity. `review_aggregates` maintained via `review_aggregate_recalc` Asynq task, not a live `AVG()`. Moderation via existing Reporting & Appeals queue. Data source for Section 26's structured data.

**New Section 30 — Generic Comparison System**
- Two layers: TipTap comparison block (inline, Layer 1) and full comparison content type (Layer 2). `comparison_items`, `comparison_dimensions`, `comparison_values` tables. `/compare/[entity-type]/[slug-or-id-list]` route. AI-generatable via HITL pipeline. Comparison Matrix as a shared frontend component. Mentorship Matching (V2) reuses this page pattern. Excluded from JSON-LD (no clean schema.org fit).

**Consequences:** Section count moves from 28 to 30. ADR count moves from 028 to 029. All files that reference section count or list sections (AGENTS.md, REVIEW-v1.0.0.md, roadmap-status.md, descriptions.md) are updated accordingly in this same pass. The `comparison` content type is added to Section 14's built-in v1 list, closing the dangling reference that existed in Sections 15/16/25/26. Section 26's `Review`/`AggregateRating` JSON-LD reference is now backed by a real schema (Section 29).
