# descriptions.md — JapaConnect Project Description

This document exists to give an AI coding agent (or a new human contributor) full context on what JapaConnect is, who it's for, and the complete shape of the product across its full lifecycle — before diving into any specific build task. It is a narrative overview, not a spec; for exact implementation detail, every claim below links back to the authoritative source in `AGENTS.md`, `ai-memory/sections/`, or `ai-memory/decisions/`.

## 1. What JapaConnect Is

JapaConnect is a community platform for the Nigerian diaspora and for Nigerians actively planning to relocate abroad. "Japa" is Nigerian slang for leaving the country — the platform's name is a direct nod to its core audience. It is solo-founded, solo-developed, and solo-owned by Jeph, who acts as both product decision-maker and the sole engineer building it, with AI coding agents (Claude Code primary, Cursor secondary) doing the implementation work against the architecture documented in this repository.

The platform sits at the intersection of three things people moving across borders actually need and rarely find in one place: practical, structured guidance (visas, work, study, settling in as a new migrant), a real community of people who've already done it or are doing it alongside you, and a path to either earn from or formalize the diaspora business and trade activity that already happens informally in these communities.

## 2. Who It's For

Four overlapping audiences shape every design decision in this project:

**People planning to relocate.** Researching visa pathways, comparing destination countries, looking for first-hand accounts rather than generic immigration-lawyer content.

**Already-relocated diaspora members.** Settling in, building community in a new country, staying connected to home, and increasingly looking to formalize businesses or side income that serves other diaspora members.

**Organizations and businesses.** Immigration consultancies, diaspora-focused service providers, educational institutions, and community organizations who want a presence in front of this audience — the entire Organization Profiles, Claim Ownership, Ad Placement, and Japa Ads self-serve system exists for this audience specifically.

**Admins and moderators.** Running the platform day to day — content moderation, community health, user support, financial oversight. Every admin-facing system (Control Panel, Admin Panel, the dynamic RBAC model underneath both) is designed so this can scale without requiring code changes as the team managing it grows beyond a single founder.

## 3. The Bootstrapping Reality

This matters architecturally, not just as background: JapaConnect is being built by one person, for a v1 launch with realistic infrastructure constraints (a single modest VPS, not a fleet of servers), with the explicit goal of avoiding both under-engineering (features that break the moment real users show up) and over-engineering (infrastructure complexity that costs more developer time to maintain than it returns in value at this scale). Decisions like deferring true zero-downtime deploys, choosing flat-rate over CPM/CPC ad pricing for v2, and keeping city-level Geo Spaces admin-created rather than auto-seeded all come from this same discipline: build for the scale that actually exists, in a way that doesn't block scaling later.

## 4. Complete Feature Inventory

### V1.0.0 — Foundation (fully locked, 45/45 sections)

**Identity, Access & Community Structure.** A dynamic three-tier RBAC system (User Groups → Roles → Permissions) where almost nothing is hardcoded — every access decision is admin-configurable. Multi-tenancy baked in from the first migration even though v1 launches single-tenant. A geo-first Space system that's the platform's core organizing principle: every user has one Primary Space per hierarchy level (changeable when they relocate) plus unlimited Followed Spaces, with countries and states auto-seeded from an open geographic dataset and cities created on demand as communities actually form around them.

**Onboarding.** A five-step flow — identity and location, destination and goals, interests, recommended spaces/people/organizations to follow, and a first-action nudge — with admin-configurable minimum engagement requirements that auto-fulfill via a ranked algorithm if a user skips rather than letting them land in an empty platform.

**Content & Authoring.** A unified content type system (`contents` table, JSONB field values, behavior packs per type) underlying blog posts, forum discussions, and pages alike. A TipTap-based rich text editor with custom mention (`@`), tag (`#`), and interlink (`[[`) support that resolves references at render time rather than baking in stale text. A drag-and-drop Page Builder layered on top of — not parallel to — that same editor, for composing landing-page-style content from reusable blocks. A forum with Solved-thread tracking that doubles as a structured-data SEO asset.

**Commerce.** A payment core abstracting Paystack (Naira), Stripe (international cards), NOWPayments (crypto), and a manual bank/mobile-money transfer path for users without card access — covering Pro membership subscriptions, organization "featured listing" boosts, one-time donations, and a dual-access course-paywall model. All card collection happens through hosted checkout (Stripe Elements/Paystack popup) — raw card data never touches platform servers. A two-tier Wallet & Credits system (Points, earned via site activity; Credits, spendable and purchasable) sits alongside this — added post-audit once V2's Referral System surfaced a cross-cutting need for it — and is itself a fifth payment method for one-time purchases (course access, and V2's Japa Ads), backed by an immutable ledger rather than a mutable balance.

**AI.** A multi-provider AI Gateway (Anthropic, OpenRouter, OpenAI, Gemini, self-hosted Ollama) with circuit-breaker fallback, a permission-gated Tool Registry, and a mandatory human-in-the-loop review pipeline for any AI-generated immigration or financial content before it can publish — this is a community making real-world relocation decisions; AI assistance never ships unreviewed for the content categories where being wrong has real consequences. JapaBot's persistent user context runs on Honcho (Plastic Labs, self-hosted, its own database on the same Postgres instance), with per-category visibility into that context gated by RBAC per User Group, an admin-facing per-user inspection panel (view + delete-entry, no editing), and a deletion call into Honcho built into the account-erasure flow.

**Internationalization.** Runtime-resolved (not compile-time) language packs, because admins need to be able to create a new language, have it AI-auto-translated, and have it human-reviewed — all without a code deploy. English launches at the root path; additional languages get a path prefix.

**Compliance.** GDPR/NDPR-aligned data export and right-to-erasure, implemented as soft-delete-and-anonymize on the same database row (never a hard delete) so community content survives intact while personal data is genuinely wiped.

**Operations.** A three-tier background job queue (Asynq), real-time presence and notifications over WebSockets with a Redis/Valkey pub/sub bridge for multi-instance scaling, Meilisearch-powered search across every major entity type, layered rate limiting and abuse protection, server-side file upload validation with malware scanning, and a Control Panel / Admin Panel split between platform-operations visibility and community-operations management.

**Discoverability.** SEO-aware metadata and structured data (including schema.org QAPage markup on solved forum threads — a close-to-free SEO win given the platform already tracks that data for other reasons), chunked sitemaps, and performance targets aligned to Google's Core Web Vitals, mostly satisfied by infrastructure already in place rather than requiring new systems.

See `AGENTS.md` for the full section map and `ai-memory/sections/01` through `45` for exact implementation detail on every item above.

### V2.0.0 — Commerce & Community (in progress)

The next layer, building directly on V1's foundations rather than introducing parallel systems:

- **Content Extensions** (Events, Jobs, Services) — new content types on the existing Content Type System, architecture decided.
- **Japa Ads (self-serve)** — extends V1's admin-only Ad Placement into advertiser self-service, with flat-rate pricing (not CPM/CPC) to avoid building real-time billing infrastructure the platform's current scale doesn't justify, and an admin review gate before any self-submitted campaign goes live. Amended post-audit to accept Credit (V1 Section 28) as a payment method for the flat fee.
- **JapaBot AI** — a chat UI surfaced directly on the existing AI Core; no new backend extension required.
- **Mentorship Matching** — connecting established diaspora members with newcomers; schema, API, and matching logic fully planned.
- **Newsletter** — locked: multi-provider abstraction (Section 15) gains a `newsletter` routing category that can be pointed at SendPulse, named lists tied to the Geo Space hierarchy, hybrid admin-authored + auto-pulled dynamic content blocks, one-off and recurring digests via Asynq, one-click List-Unsubscribe plus a granular preference center, click-only tracking (no open pixel, consistent with the platform's privacy posture), platform-level bounce suppression, English (default) and French (/fr/...), with admin-managed Language Pack system for additional languages.
- **Referral System** — locked: Credits via the V1 Section 28 ledger, granted on the referee's first qualifying paid action (not signup); 30-day first-click attribution; no expiration; same-device/IP/payment-method self-referral detection plus a per-period reward cap, with above-cap activity routed to manual admin review; single-level only; referrer sees referral count and status only, never referee PII.

### V3.0.0 — Intelligence & Scale (not yet started)

Directional only at this stage: a Scholarship Finder, a "Japa Readiness Check" assessment tool, and a Remittance Tracker all have an agreed extension pattern but undetermined content/provider specifics; native mobile apps and a Foundation/NGO module (housing support, scholarships) exist as stated intent without architecture yet.

## 5. Technical Architecture, In Brief

Go 1.26+ backend (Chi, sqlc, pgx/v5, Asynq) as a monolith — deliberately not microservices, given the team size. SvelteKit on Bun across three apps (main site, admin SPA, control panel SPA). PostgreSQL 18 with pgvector for embeddings — no separate vector database. Valkey (the BSD-licensed, cloud-provider-default Redis-protocol fork) for caching, pub/sub, and queue backing. Meilisearch for search. The full version table and every pinned dependency live in `AGENTS.md`.

The architecture's defining trait isn't any single technology choice — it's how much the systems reuse each other rather than existing as 45 independent silos: organization boosts run through the same campaign infrastructure as advertising; onboarding's recommendation engine, content mentions, and tag autocomplete all query the same search indexes; the email provider abstraction and the AI Gateway share an identical circuit-breaker-fallback shape; Page Builder embeds the same rich-text editor used everywhere else rather than reinventing it; the Wallet & Credits ledger reuses the payment provider abstraction's idempotency pattern; the Generic Comparison System's Comparison Matrix component is directly reused by Mentorship Matching's UI rather than rebuilt; Ratings & Reviews moderation flows through the existing reporting queue rather than a separate review-moderation system.

## 6. Business Model

Multiple monetization paths running in parallel rather than a single bet: Pro membership subscriptions (recurring, Stripe/Paystack only), organization featured-listing boosts (one-time, reusing the ad campaign system), course access (subscription-inclusive, à la carte, or paid with purchased Credit), donations, and — starting in V2 — self-serve advertising (which also accepts Credit for its flat-rate fee). Credit purchases (real money → spendable Wallet balance) are themselves a revenue path, processed through the same Stripe/Paystack rails as everything else. The longer-term Foundation/NGO module (V3, housing support and scholarships) signals an intent for the platform to eventually fund community-benefit work, not just operate as a pure commercial product.

## 7. Where to Go for Implementation Detail

This document is the map, not the territory. For actual build work:
- `AGENTS.md` — architecture context, pinned versions, canonical code patterns.
- `ai-memory/sections/` — exact decisions per section, V1.0.0.
- `ai-memory/decisions/` — the *why* behind every major architectural choice.
- `ai-memory/skills/` — working code patterns for recurring implementation problems.
- `RULES.md` — hard rules that override convenience.
- `developer_review.md` — the engineering-judgment companion to this document: build order, risk areas, and what a senior reviewer would flag before this goes into active development.
