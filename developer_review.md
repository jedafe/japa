# developer_review.md — JapaConnect Engineering Review

This is the companion document to `descriptions.md`. Where that document explains *what* JapaConnect is, this one covers *how to think about building it well* — the kind of judgment a senior engineer would apply reviewing this project before active development starts: build order, risk areas, what's genuinely solid, what to watch for, and what a careless implementation would get wrong even while technically matching the spec.

## 1. Overall Architecture Assessment

The strongest signal in this codebase's design is reuse density, not any individual clever choice. Organization boosts, self-serve ads, and admin-curated promotions all run through one campaign system. The AI Gateway and the email provider abstraction share an identical interface-plus-circuit-breaker shape. Onboarding, mention autocomplete, and tag suggestions all hit the same search indexes rather than three parallel lookup paths. This matters practically: an agent implementing a *new* feature that smells like one of these patterns (anything needing provider fallback, anything needing ranked/matched suggestions, anything needing admin-curated-plus-algorithmic content) should default to reusing the existing pattern rather than inventing a fourth variant. If a new requirement seems to need a genuinely new pattern, that's worth pausing on — check `ai-memory/decisions/` first; there's a reasonable chance a similar problem was already solved.

The weakest point isn't a design flaw — it's a confidence gradient. Sections 1–8 (Foundation, Auth, RBAC, Multi-tenancy, Geo Spaces, Custom Fields, Organizations, Forum) were planned in earlier sessions and exist here only as reconstructed summaries, not verbatim original specs. Sections 9–27 were built section-by-section with explicit confirmation at every step and can be trusted at full fidelity. **Section 02 (Authentication & Sessions) is the single lowest-confidence file in the entire set** — it has no real detail beyond "this exists and Section 10's WebSocket auth depends on it." Before any auth-related work starts, this section needs either verification against an original source or a proper planning pass from scratch. Treat it as unplanned, not just under-documented.

## 2. Build Order & Dependency Graph

The four-release grouping in `roadmap/ROADMAP-v1.1.0.md` through `v1.4.0.md` is a thematic proposal, not a strict dependency order — but there is a real dependency structure underneath it worth respecting regardless of which release groups what:

**Must exist before almost anything else:** Foundation/stack (01), Auth (02 — pending the verification above), RBAC (03), Multi-tenancy (04). Every other system either enforces permissions through RBAC or scopes data through tenant_id.

**Must exist before content of any kind:** Content Type System (14) and the Custom Field Builder (06) — both Block Editor (15) and Page Builder (16) build directly on these, and Page Builder specifically should not be started before TipTap (15) is working, since it embeds that editor rather than running parallel to it.

**Must exist before community features feel real:** Geo-First Space System (05) — Onboarding (09) recommends spaces and people relative to this, and Search (13) indexes spaces as a first-class entity.

**Must exist before money changes hands:** Payment Core (17) before Organization Boosts (part of 07/25) or Course Access (part of 17/24) can be tested meaningfully, since boosts and course access are payment purposes, not separate payment systems.

**Genuinely parallelizable once the above exists:** Job Queue (11), Real-Time/WebSockets (10), Webhooks (12), and Search (13) don't have strong dependencies on each other and can be built concurrently by different work streams if this ever stops being a solo effort.

**Should come last, not first:** SEO (26) and the Control/Admin Panel dashboards (21/22) are observability and discoverability layers over systems that need to exist and produce real data before their dashboards or metadata strategies are meaningfully testable.

## 3. Risk Register

Items worth tracking explicitly rather than assuming are settled:

| Item | Status | Why It Matters |
|---|---|---|
| Section 02 (Auth) | Reconstructed, lowest confidence in the set | Every permission check and the WebSocket ticket auth pattern depends on this being right |
| Release grouping (v1.1.0–v1.4.0) | Proposed, not mandated | Fine to rearrange, but don't assume it reflects an agreed build sequence — see Section 2 above for the actual dependency graph |
| Geo dataset seed script | Designed, not yet run against a real repository | First real-world test of the Countries States Cities import happens at actual repo setup, not during planning |
| Email provider credentials | Abstraction designed, no live credentials provisioned | `email_send` will fail silently in any environment without real provider keys configured — this is an ops task, not a code gap |
| V2/V3 items beyond what's listed in `descriptions.md` | Concept-level or unplanned | Don't let an agent infer detailed specs for Newsletter, Referral System, Scholarship Finder, Readiness Check, Remittance Tracker, Mobile Apps, or the Foundation/NGO module — none of these have real architecture yet, and confidently inventing one would create false confidence for future planning sessions |

Resolved items, included here only so they're not re-litigated: Valkey over Redis 8 (ADR-021), rate limiting and abuse protection (ADR-022), file upload validation (ADR-023), production email provider (ADR-015), AI Tool Registry catalog for v1 (Section 20), canonical geo data source (ADR-020), Meilisearch backup strategy (confirmed unnecessary — rebuildable from Postgres), PCI scope (ADR-012).

## 4. Code Quality Standards — What Actually Gets Enforced

`RULES.md` is the canonical list; the ones worth a reviewer's particular attention because they're easy to violate by accident under deadline pressure:

**The "never hardcode a role" rule is the one most likely to get quietly broken.** It's faster to write `if user.Role == "admin"` than to wire up a permission node, and it'll work fine in testing. The cost shows up later when an admin tries to delegate that one specific capability to a non-admin group and discovers it's impossible without a code change. Every new access-controlled feature needs its permission node(s) decided at design time, not retrofitted.

**The render-time-not-save-time rule for mentions/tags/interlinks is easy to violate subtly.** A naive implementation might resolve a mention's display name once and cache the resolved string — this looks correct until someone renames their account and every historical mention of them is now wrong with no way to fix it short of a data migration. The pattern is: store the ID, resolve the label at render time, always.

**The webhook synchronous-processing trap.** It's tempting to just process a webhook's business logic directly in the HTTP handler "since it's simple." This breaks the moment a provider's webhook delivery has any retry behavior under load, because duplicate processing isn't idempotent unless you built it to be. Validate, record, enqueue, return — every time, even for the webhook that feels too simple to need it.

## 5. Testing & QA Philosophy

Section 27 deliberately rejects a blanket coverage-percentage target in favor of risk-weighted coverage: real test investment goes into RBAC permission resolution, payment state transitions, and the content publish state machine — not into achieving a number. A reviewer should be more concerned about an agent that reports "95% coverage" on trivial getters than one that reports "70% coverage, but every payment webhook path and every permission check has a test." If test output ever drifts toward the former, that's a signal worth flagging even if the percentage looks better.

## 6. Production Readiness — Current State

As of this document, V1.0.0's architecture is complete and every gap identified in the original production-readiness audit has a resolved, documented home (see `REVIEW-v1.0.0.md` for the full audit and `ai-memory/decisions/` for ADR-015, ADR-020, ADR-021, ADR-022, ADR-023 specifically). What remains before this is actually production-ready in the literal sense: zero code has been written yet. Architecture completeness and implementation readiness are different bars — this document and everything it points to describes the former. The latter starts at first commit.

## 7. Anti-Patterns to Avoid (Beyond What RULES.md States Directly)

**Don't let "it's just for v1, we'll fix it later" justify skipping `tenant_id` on a new table, skipping a permission node for a new feature, or skipping the two-phase migration pattern for a breaking schema change.** These three specifically get exponentially more expensive to retrofit the longer they're deferred, unlike most "we'll fix it later" technical debt which stays roughly the same cost over time.

**Don't add a new external service dependency without checking whether the existing provider-abstraction pattern (payments/email/AI) already fits the need.** A fourth bespoke integration pattern in a solo-maintained codebase is a maintenance burden with no offsetting benefit if the existing pattern would have worked.

**Don't treat the V2/V3 status table in `ai-memory/roadmap-status.md` as a backlog to start pulling from opportunistically.** Several V2/V3 items are explicitly concept-level — building against an unplanned feature produces an implementation that constrains the eventual real design rather than informing it.

## 8. Recommended Reading Order for a New Agent or Contributor

1. `descriptions.md` — what this is and why.
2. This document.
3. `AGENTS.md` — architecture context and canonical patterns.
4. `RULES.md` — the hard constraints.
5. The specific `ai-memory/sections/` file(s) relevant to the task at hand.
6. Any `ai-memory/decisions/` ADR referenced from that section file, for the reasoning behind a decision that seems unintuitive in isolation.
7. `ai-memory/skills/` for the actual implementation pattern, last — by this point the *why* should already be clear, and the skill file is just the *how*.
