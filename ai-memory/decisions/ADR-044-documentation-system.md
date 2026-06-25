# ADR-044: Documentation System — AI-Generated, RBAC-Gated

**Status:** Accepted

**Context:** A community platform of this scope needs documentation for seven distinct audiences (users, premium members, organizations, moderators, developers, admins, super admins), each with different content and access levels. Writing all documentation manually is not feasible at solo-founder scale.

**Decision:** Documentation at `docs.japaconnect.com` (fifth SvelteKit SSR app). Content stored in `doc_pages` table with audience scoping and `review_status`. AI generation via the `platform_agent` tool (`docs.generate_page`), producing TipTap JSON drafts that go through the mandatory HITL review pipeline before publication. API reference at `/docs/api` is fully public. Interactive API Playground at `/docs/api/playground`. RBAC gates match audience keys to permission nodes.

**Rationale:** AI-generated documentation with human review is the only sustainable approach at solo-founder scale. The same HITL pipeline already required for immigration/financial content applies here — documentation for admin or moderation workflows is similarly high-stakes. Separate subdomain (`docs.japaconnect.com`) provides clean URL separation and allows independent deployment cadence.

**Consequences:** Documentation requires the AI Core (Section 20) to be operational. Initial documentation seed is a significant content generation task — recommend batching by audience, starting with developer/API docs (most objectively verifiable) and admin docs.
