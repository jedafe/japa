---
name: jc-docs-develop
description: Use when building new pages or components for the JapaConnect Docs app (apps/docs, port 3004, SSR, docs.japaconnect.com). Covers the three-column docs layout, Cmd+K search, code blocks, API reference with LayerChart usage stats, and the API-Readiness Gate.
---

# JapaConnect Docs — Development Skill

Developer + user documentation at `docs.japaconnect.com`. SSR (adapter-node + Bun). API base `/api/v1/docs/*`. Design: Stripe docs / SvelteKit.dev — three-column, 18px/1.7 reading comfort, excellent code blocks.

## ⛔ API-READINESS GATE
Same as `jc-main-develop`: list endpoints → verify (catalog or probe) → any missing: STOP + ⚠️ MISSING BACKEND API ROUTES alert. Never mock, never hardcode the sidebar tree. Known routes: `GET /docs/sections`, `GET /docs/pages/:slug`, `GET /docs/search?q=`, `GET /docs/api-reference` — verify each before use.

## Three-Column Layout
Sidebar 260px sticky (API-driven nav tree, `<Accordion.Root type="multiple">`, active page highlighted) · content max-width 680px centered · On-This-Page 180px sticky right (H2/H3 anchors via IntersectionObserver). Mobile: single column, sidebar in `<Dialog.Root>` drawer, right panel hidden.

## Key Patterns
**Cmd+K search:** global keydown in `$effect` WITH cleanup return · `<Dialog.Root>` + `<Combobox.Root>` · debounced 200ms → `/docs/search` · results grouped by section · full keyboard nav.
**On-This-Page:** IntersectionObserver (`rootMargin: '-10% 0% -80% 0%'`) in `$effect` WITH `observer.disconnect()` cleanup — never scroll listeners.
**Code blocks:** language badge · copy button (`code.textContent` → clipboard, `<Tooltip.Root>` "Copied!") · highlighting post-render via dynamic import, browser-guarded.
**API reference:** OpenAPI spec render · method badges (GET green, POST blue, PATCH orange, DELETE red) · `<Tabs.Root>` Request | Response | Example per endpoint · interactive playground with user's API key.

## LayerChart — Docs Analytics (Where Applicable)
If a docs page or the API reference surfaces usage stats (endpoint request volumes, popular pages, search-term trends), render with LayerChart: `<Bars>` for top pages/endpoints, `<Spline>` for trends. Same rules: `var(--color-*)`, fixed heights, API data (Gate-verified), skeletons.

## SEO
Every page: title, description, canonical on `docs.japaconnect.com` · `TechArticle` JSON-LD (headline, dateModified, publisher) · landing page: `WebSite` JSON-LD with SearchAction.

## Stack
Identical base to `jc-main-develop`: Svelte 5 Runes, Bits UI dot-notation, Tailwind v4, Lucide, TypeScript, `$t()` i18n, Rule 16, no AI attribution.

## Deliverables Per Task
1. Gate output · 2. `+page.server.ts` · 3. `+page.svelte` (three-column, skeletons) · 4. components (Sidebar, OnThisPage, CodeBlock, SearchModal) · 5. JSON-LD · 6. types
