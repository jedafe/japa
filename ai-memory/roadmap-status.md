# Roadmap Status — JapaConnect

High-level view across all sections. Detail per section lives in `ai-memory/sections/NN-*.md`. Rationale behind specific decisions lives in `ai-memory/decisions/`.

## V1.0.0 — Fully Locked (45/45 sections)

| # | Section | Confidence |
|---|---|---|
| 01 | Foundation & Tech Stack | Reconstructed + Honcho DB placement added |
| 02 | Authentication & Sessions | Fully reconstructed — bcrypt/12, JWT+refresh, multi-kid rotation, TOTP mandatory for admin, OAuth social, idempotency keys, CSRF posture, password policy, profile_type |
| 03 | RBAC | Reconstructed + amended (AI context nodes, no-auto-promotion rule) |
| 04 | Multi-Tenancy | Reconstructed |
| 05 | Geo-First Space System | Reconstructed + refined |
| 06 | Custom Field Builder & Form Builder | Reconstructed |
| 07 | Organization Profiles & Claim Ownership | Reconstructed + Verification Badges restored |
| 08 | Forum Q&A / Solved | Reconstructed — full scope restored (tags, polls, anonymous posting, bookmarks, reactions, view counts, nested threads) |
| 09 | Onboarding Flow | Full + product tour (Driver.js) restored |
| 10 | Real-Time (WebSockets) | Full + Direct Messaging restored |
| 11 | Job Queue (Asynq) | Full |
| 12 | Webhooks | Full |
| 13 | Search | Full |
| 14 | Content Type System | Reconstructed — full scope restored (JSON-LD templates, geo classification, revisions, scheduling, polymorphic reactions/bookmarks/engagement counts, comparison type) |
| 15 | Block Editor (TipTap) | Full + mention edge cases restored |
| 16 | Page Builder | Full |
| 17 | Payment Core | Full + Credit provider amended + Discount Codes restored |
| 18 | i18n / Language Packs | Full + locale scope corrected (en-NG/en-US/en-GB/en-CA only, not French) |
| 19 | GDPR / NDPR Compliance | Full + Honcho erasure step added |
| 20 | AI Core | Full + Honcho DB/admin visibility amendments |
| 21 | Control Panel | Full + Honcho health + DB health tooling restored |
| 22 | Admin Panel | Full + Honcho per-user panel added |
| 23 | Main Site Frontend | Full + Dark Mode + timezone-aware display restored |
| 24 | Blog Extension | Full + Blog Series schema restored |
| 25 | Ad Placement | Full |
| 26 | SEO & Performance | Full |
| 27 | DevOps, Testing & QA | Full |
| 28 | Wallet & Credits System | Full — added post-audit + Karma folded in |
| 29 | Universal Ratings & Reviews | Full — restored post-audit (cross-session audit pass) |
| 30 | Generic Comparison System | Full — restored post-audit (cross-session audit pass, gave canonical home to feature previously referenced across 4 sections) |
| 31 | Follow System | Full — added post-audit |
| 32 | Activity Feed / Timeline | Full — added post-audit |
| 33 | Professional Profiles | Full — added post-audit |
| 34 | Media System + Library + Modal Picker | Full — added post-audit |
| 35 | Notification System | Full — added post-audit |
| 36 | Learning Platform (learn.japaconnect.com) | Full — added post-audit |
| 37 | IP Geolocation (MaxMind GeoLite2) | Full — added post-audit |
| 38 | Ticket System | Full — added post-audit |
| 39 | Space Types & University System | Full — added post-audit |
| 40 | Platform Analytics & Site Reports | Full — added post-audit |
| 41 | Spam Prevention & Content Safety | Full — added post-audit |
| 42 | Feature Flags & Pluggable Architecture | Full — added post-audit |
| 43 | Bot & Crawler Management | Full — added post-audit |
| 44 | Documentation System (docs.japaconnect.com) | Full — added post-audit |
| 45 | Dedicated Post Create Page (/create) | Full — added post-audit |

**Production-readiness audit:** complete, see `REVIEW-v1.0.0.md`. All identified gaps (email provider, AI tool registry, geo data source, rate limiting, file upload validation) resolved at design level.

## V2.0.0 — Commerce & Community (in progress)

| Section | Status |
|---|---|
| Content Extensions (Events, Jobs, Services) | ✅ Architecture decided |
| Japa Ads (self-serve) | ✅ Designed — flat-rate pricing, admin-review gate, builds on V1 Section 25; amended post-audit to accept Credit (Section 28) as a payment method for the flat fee |
| Newsletter | ✅ Locked — multi-provider abstraction with new `newsletter` routing category, named lists on the Geo Space hierarchy, hybrid admin-authored + dynamic-block content, one-off + recurring digests via Asynq, one-click List-Unsubscribe + preference center, click-only tracking, platform-level bounce suppression, English only (no localization pipeline) |
| Referral System | ✅ Locked — Credits via Section 28 on the referee's first qualifying paid action (not signup); 30-day first-click attribution; no expiration; same-device/IP/payment-method self-referral detection plus a per-period cap on rewarded referrals, with above-cap activity routed to manual admin review; single-level only (no multi-level/MLM structure); referrer sees referral count and status only, never referee PII — see ADR-026 |
| JapaBot AI (chat UI) | ✅ Architecture decided |
| Mentorship Matching | ✅ Fully planned |

## V3.0.0 — Intelligence & Scale (not yet started)

| Section | Status |
|---|---|
| Scholarship Finder | 🟡 Extension pattern decided, content TBD |
| Japa Readiness Check | 🟡 Extension pattern decided, assessment model TBD |
| Remittance Tracker | 🟡 Extension pattern decided, provider integrations TBD |
| Native Mobile Apps | ⭕ Concept only |
| Foundation / NGO Module | ⭕ Concept only |

## Document Generation Status

Set 1 (AGENTS.md, CLAUDE.md stub, RULES.md, REVIEW-v1.0.0.md, roadmap/v1.1.0–v1.4.0.md) and Set 2 (`ai-memory/` folder: 23 ADRs, 10 skills, 6 conventions, 27 section files, this status file) generated for V1.0.0. `ai-memory/bugs/` intentionally left empty — it's a living log to be filled in as real bugs are hit during implementation, not something to pre-populate during planning. A subsequent pass corrected stale tech stack version references (Go 1.23→1.26, PostgreSQL 16→18, etc.) and added code examples throughout — see ADR-021 for the one new open item that surfaced (Redis licensing). A further pass brought the audit's two thinnest gap resolutions — rate limiting (ADR-022) and file upload validation (ADR-023) — up to the same ADR-plus-section-plus-code-pattern standard as every other locked decision; all seven audit gap rows now have a dedicated home rather than just a bullet-point mention.

**Post-audit pass (added during V2 planning):** Section 28 (Wallet & Credits System) added to V1.0.0 — originated as a cross-cutting need surfaced while planning V2's Referral System, elevated to V1 foundational infrastructure since Courses, Referral, and Japa Ads all need to consume it. Two new ADRs (ADR-024, ADR-025), one new skill file (`wallet-ledger-pattern.md`), addenda to ADR-012 and ADR-018, and amendments to Section 03 (RBAC) and Section 17 (Payment Core). AGENTS.md, RULES.md, REVIEW-v1.0.0.md, descriptions.md, and roadmap/ROADMAP-v1.0.0.md updated to reflect the 28th section. `super_admin` is not pre-granted `ai.context.*` nodes by default — every Group, including system defaults, requires manual grant. Referral System (V2) subsequently locked in full — see ADR-026. Honcho admin visibility (Control Panel health, Admin Panel per-user content panel) and erasure-flow integration locked — see ADR-027, with amendments to Section 19 (GDPR/NDPR), Section 20 (AI Core), Section 21 (Control Panel), Section 22 (Admin Panel), Section 01 (Honcho's own database confirmed: same Postgres 18.x instance, separate logical database), and ADR-002/ADR-014 addenda.

**Cross-session audit pass (four earlier sessions reviewed):** A material number of features were found missing — lost when session 4's reconstruction pass skimmed earlier-locked decisions. All restored per ADR-029: Forum full scope (Section 08), product tour (Section 09), Direct Messaging (Section 10), Content Type full scope (Section 14), mention edge cases (Section 15), Discount Codes (Section 17), locale scope correction to en-NG/en-US/en-GB/en-CA only (Section 18), DB health tooling (Section 21), Dark Mode + timezone display (Section 23), Blog Series schema (Section 24), Karma folded into Points with no auto-promotion (Section 28, ADR-028), Verification Badges (Section 07). Two new sections: 29 (Universal Ratings & Reviews) and 30 (Generic Comparison System, given canonical home after being referenced across four sections without one). Section count now 30. ADR count now 029.

**ADR integrity pass:** All 46 ADRs (ADR-001 through ADR-046) now have corresponding files in ai-memory/decisions/. ADR numbering errors in Sections 41–45 corrected (were off by 2). Section 18 ADR reference corrected (ADR-046 for the French language session-4 decision). Zero dangling ADR references remain across all 45 section files.
