---
name: jc-docs-develop
description: Use when building new pages or components for the JapaConnect Docs app (apps/docs, port 3004, SSR, docs.japaconnect.com). Covers the unified app shell adapted for Docs (ADR-060) — nav-tree-as-left-nav, TOC-as-right-sidebar, audience gating, the API-reference code pane and full-width playground exception, Cmd+K search, and the API-Readiness Gate.
---

# JapaConnect Docs — Development Skill

Developer + user documentation at `docs.japaconnect.com`. SSR (adapter-node + Bun). API base `/api/v1/docs/*`. **Layout is the unified app shell (ADR-058) adapted for Docs (ADR-060) — the left nav IS the doc nav tree, the right sidebar IS the in-page TOC.** Reading comfort 16–18px / 1.7, excellent code blocks.

## ⛔ API-READINESS GATE
Same as `jc-main-develop`: list endpoints → verify (catalog or probe) → any missing: STOP + ⚠️ MISSING BACKEND API ROUTES alert. Never mock, never hardcode the nav tree or page content. Known routes: `GET /docs/nav` (audience-filtered tree), `GET /docs/:audience/:slug` (page), `GET /docs/search?q=&audience=`, OpenAPI spec — verify each before use.

## Japa University & Geo-Aware API Docs (ADR-063/064)
When documenting the new v1.11.0 API surfaces, ensure the API reference pages cover:
- **Geo-aware (ADR-064):** `GET /api/v1/geo/me` (returns `VisitorContext` with `ContentCountry`, archetype, confidence), `GET /api/v1/geo/lookup?ip=` (admin). Document the `?geo=` param behavior on feed/search/trending/discovery routes (defaults to `ContentCountry` when absent, explicit `?geo=all` for global).
- **Japa University (ADR-063):** `/api/v1/learn/university/tracks` (marketplace catalog), `/api/v1/learn/university/instructors/apply`, `/api/v1/me/instructor-profile`, `/api/v1/learn/university/tracks/:id/submit`, `/api/v1/me/payouts`. Document `price_kobo` (integer, kobo), `is_published_to_university` (distinct from `is_published`), and Pro dual-access behavior.
- **University spaces (Section 39):** `GET /api/v1/universities` (public discovery), `GET /api/v1/universities/:slug`, admin CRUD at `/api/v1/admin/universities`. Document `university_fields` custom field group.
- These are API reference pages (`/docs/api/:slug`) — use the three-column API-docs variant with method badges, Request/Response/Example tabs.

## The Shell (ADR-058 primitives, ADR-060 drift)
`AppShell` (collapsible `LeftNav` + center + `RightSidebar`), `ui.nav_collapsed` server-persistence (never localStorage), sticky right-sidebar top bar (⌘K launcher + avatar), dark-mode tokens, mobile single-column.
- **Left nav = the audience-gated doc nav tree** (NOT social items): render the `DocNavTree` from `GET /api/v1/docs/nav` — section groups → pages → sub-pages, active link highlighted (left-border accent), Stripe-docs section headers. An **audience switcher** sits in the nav header (only audiences the session can access, server-resolved). Collapsible.
- **Center:** the doc page `.prose` (SSR TipTap JSON → HTML via `@tiptap/html`, `.prose-table-wrapper` tables), max-width ~760px centered.
- **Right sidebar = in-page TOC (the docs drift):** H2/H3 anchors via IntersectionObserver, sticky, highlights the heading in view (desktop `≥1280px`); below it a "was this helpful? 👍👎" control + last-reviewed footer. Replaces social widgets.

## Audience Gating (server-side ONLY)
Never hide gated docs with client checks. The API returns 403; the page redirects to `/docs/access-denied`. `/docs/api` is fully public (no session needed).

## Two Docs-Specific Variants
- **API reference (`/docs/api/:slug`):** three-column API-docs variant — nav tree · prose · **right code-sample pane**. Method badges (GET green, POST blue, PATCH amber, DELETE red via CSS vars). `<Tabs.Root>` Request | Response | Example per endpoint.
- **API Playground (`/docs/api/playground`):** the **full-width tool exception** — breaks the shell (no tree/TOC columns) for room. Real API calls with the user's own key (Section 47). Never expose other users' keys.

## Key Patterns
- **⌘K search:** global keydown in `$effect` WITH cleanup return · Bits UI `<Command>`/`<Dialog.Root>` + `<Combobox.Root>` · debounced 200ms → `/docs/search?q=&audience=` (audience-filtered) · grouped results · full keyboard nav.
- **On-This-Page TOC:** IntersectionObserver (`rootMargin: '-10% 0% -80% 0%'`) in `$effect` WITH `observer.disconnect()` cleanup — never scroll listeners.
- **Code blocks:** language badge · copy button (`code.textContent` → clipboard, `<Tooltip.Root>` "Copied") · highlighting post-render via dynamic import, browser-guarded.

## SEO
Every page: title, description, canonical on `docs.japaconnect.com` · `TechArticle` JSON-LD (headline, dateModified, publisher) · landing: `WebSite` JSON-LD with SearchAction.

## LayerChart (only where docs surface usage stats)
If a page surfaces usage stats (endpoint volumes, popular pages), render with pinned `layerchart@2.0.1` — `BarChart` for top pages, `LineChart` for trends. Colors via `var(--chart-*)`, fixed heights, Gate-verified data, skeletons.

## Stack
Identical base to `jc-main-develop`: Svelte 5 Runes, Bits UI dot-notation, Tailwind v4, Lucide, TypeScript, `$t()` i18n, Rule 16, no AI attribution.

## Deliverables Per Task
1. Gate output · 2. surface built INSIDE `AppShell` (or the playground full-width exception, explicitly) · 3. `+page.server.ts` (audience-gated fetch) · 4. `+page.svelte` (nav tree + content + TOC, skeletons) · 5. components (LeftNav tree, TOC, CodeBlock, SearchPalette, AudienceSwitcher) · 6. JSON-LD · 7. types
