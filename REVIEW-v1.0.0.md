# REVIEW-v1.0.0.md — Production Readiness Audit

Status: **V1.0.0 architecture fully resolved.** This is a planning/architecture audit — no code has been written yet, so "ready" here means the design is complete and internally consistent, not that the system has been built and tested. That remains a separate, future bar.

## Scope and Confidence

Sections 9–27 were planned in full detail across this session, with explicit confirmation at each step. Sections 1–8, 13–14, and 20 draw on carried-over architectural memory from earlier sessions and are reconstructed at the thematic level rather than verbatim — flagged in `AGENTS.md` and the relevant `ai-memory/sections/` files for verification.

## What's Solid

The architecture has real internal coherence rather than being 27 independently bolted-together sections. Notable reuse patterns that indicate this:

- Organization boosts (Section 17) reuse the Ad Placement campaign infrastructure (Section 25) rather than duplicating it.
- Mentions, tags, and onboarding's recommended-spaces algorithm all reuse the same Meilisearch indexes (Section 13) instead of separate lookup logic.
- The Email provider abstraction and AI Gateway share the identical multi-provider, circuit-breaker-fallback shape.
- Page Builder's Rich Text block embeds TipTap rather than reinventing rich text editing — one editor, one source of truth.
- The Content Type System's `field_values` JSONB pattern underlies both ordinary content types and Page Builder's block storage.

## Gaps Identified and Resolution

| Gap | Resolution |
|---|---|
| No production transactional email provider named | Multi-provider abstraction locked: Resend, Postmark, AWS SES, SendGrid, SendPulse, plus admin-addable custom SMTP. Category-routed (transactional vs. marketing), circuit-breaker fallback. |
| AI Tool Registry catalog never itemized for v1 | Itemized: Content agent (`content.generate_draft`, `content.editor_assist`, `content.suggest_tags`), Moderation agent (`moderation.flag_content`, `moderation.classify_severity`), Platform agent (`platform.translate_string`). Analytics agent has zero registered tools in v1 — explicitly a v2/v3 placeholder, not an oversight. |
| No canonical geo data source for the Space hierarchy | Countries States Cities open dataset, seeded into Spaces directly. Country + state/province auto-seeded at launch; city-level Spaces are admin-created on demand, never auto-seeded. |
| No rate limiting or abuse protection anywhere in the design | Layered: Nginx coarse per-IP limiting at the proxy, Valkey-backed application-level limiting for per-user/per-feature rules, login brute-force protection with exponential backoff, and a hard per-user AI Gateway quota (not just the admin-facing budget alert). |
| No file upload validation specified | Server-side MIME verification, per-context size limits, ClamAV malware scanning via a new `media_scan` Asynq task that blocks persistence to MinIO until clear. |
| Meilisearch backup strategy unclear | Confirmed: no separate backup needed. Meilisearch is fully rebuildable from Postgres (source of truth). Restore runbook (Section 27) includes "rebuild search index" as an explicit post-restore step. |
| PCI card-handling scope implicit, not stated | Made explicit: all card collection goes through Stripe Elements / Paystack's hosted checkout. Raw card data never touches our servers. |

## Non-Engineering Items, Explicitly Out of Architecture's Scope

These need legal/operational attention, not code, and aren't something an architecture review closes out:
- Actual privacy policy text
- DPO designation
- Breach notification procedure

## Sign-Off Statement

V1.0.0 is architecturally complete. The gaps identified above have been resolved at the design level and are reflected in the relevant `ai-memory/sections/` and `ai-memory/decisions/` files. Recommended before development starts: confirm the email provider credentials are actually provisioned, and run the geo dataset seed script once the repository exists. Everything else is ready to build against.

## Addendum: Version Verification Pass

A subsequent pass verified pinned tech stack versions against current actual releases rather than relying on initially-stated figures, which had drifted (notably Go 1.23 stated vs. 1.26 actual current stable). Corrected versions are now the single source of truth in `AGENTS.md`. One item flagged during this pass — Redis 8.x's license change away from BSD — has since been resolved: **Valkey** (BSD-licensed, protocol-compatible) is the locked decision, see ADR-021.

## Addendum: Section 28, Wallet & Credits System (Added Post-Audit)

After this audit's original sign-off, a cross-cutting need for a Points/Credits ledger surfaced while planning V2's Referral System. Rather than scope it narrowly inside that one V2 feature, it was elevated into V1.0.0 as Section 28, since Courses (Section 17) and V2's Japa Ads both need to consume the same infrastructure. This is a genuine architectural addition, not a gap in the original audit — the original 27 sections didn't need a wallet system; V2 planning is what created the need. See Section 28, ADR-024, and ADR-025. Two existing sections were amended as a result: Section 03 (RBAC, new `ai.context.*` permission node family) and Section 17 (Payment Core, Credit as a fifth, non-`Provider` payment method). `super_admin` is not pre-granted `ai.context.*` nodes by default — confirmed manual-grant-only, no exception for system-default Groups.

## Addendum: Honcho Admin Visibility and Erasure Integration (Post-Audit)

Two further gaps surfaced once `ai.context.*` gating existed: no admin-facing visibility into Honcho's health or per-user stored content, and no path for account erasure to reach Honcho's data, which lives in its own database rather than the main application's tables. Resolved per ADR-027: Control Panel gains Honcho health monitoring (same pattern as Meilisearch), Admin Panel gains a per-user read-only-plus-delete-entry AI Context panel, and the erasure job gains a required (not best-effort) Honcho deletion step. Honcho's database placement is now stated explicitly rather than left implicit: same PostgreSQL 18.x instance as the main app, separate logical database, own internally-managed migrations — see Section 01 and the ADR-002 addendum.

## Addendum: Cross-Session Audit — Feature Restoration (ADR-029)

A review of four earlier planning sessions against the final doc set revealed a material number of previously-locked features that were lost during session 4's reconstruction pass. All restored in a single pass per ADR-029. Key restorations: Forum full scope (nested threads, tags/moderators, polls, anonymous posting, reactions, bookmarks, view counts — Section 08 was a one-sentence stub); Direct Messaging (Section 10, using the `dm_message` event type that had always been in the catalog but had no feature behind it); Content Type full scope (JSON-LD templates, geo classification, revisions, scheduling, polymorphic reactions/bookmarks/engagement counts — Section 14 was a two-sentence stub); Verification Badges (Section 07); product tour/Driver.js (Section 09); mention edge cases (Section 15); Internal Discount Codes (Section 17); locale scope corrected from French to regional English only, en-NG/en-US/en-GB/en-CA (Section 18); DB health tooling (Section 21); Dark Mode and timezone-aware display (Section 23); Blog Series schema (Section 24); Karma/Reputation folded into the Points ledger with no automated Group promotion (Section 28, ADR-028). Two new sections: 29 (Universal Ratings & Reviews) and 30 (Generic Comparison System, given its first canonical home). Section count is now 30. ADR count is now 029.

## Addendum: Comprehensive Audit Pass — 45 Sections (Post-Audit)

A thorough cross-session audit against all four original planning sessions, plus a feature requirements document, revealed 15 additional systems and sections that needed to be designed, added, or restored. Section count moves from 30 to 45. Key additions: Follow System (31), Activity Feed/Timeline (32), Professional Profiles (33), Media System + Library + Modal Picker (34), Notification System (35), Learning Platform (36), IP Geolocation (37), Ticket System (38), Space Types & University System (39), Platform Analytics & Reports (40), Spam & Content Safety (41), Feature Flags & Pluggable Architecture (42), Bot & Crawler Management (43), Documentation System (44), Post Create Page /create (45). Additionally: Section 18 (i18n) corrected — French is the second language per session-4 explicit decision (not English-only as incorrectly set earlier). Section 27 corrected — GlitchTip replaces Sentry throughout. Section 06 fully redesigned as Universal Custom Field Architecture. Sections 05, 14, 22, 23, 26 significantly expanded. ROADMAP regenerated comprehensively with Go + SvelteKit code examples for all 45 sections.
