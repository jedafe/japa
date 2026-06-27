# Section 39 — Space Types & University System

**Date:** 2026-06-27
**Phase:** 7
**Status:** Complete

## Overview

Section 39 implements the university discovery and detail pages for JapaConnect. The frontend provides a `/universities` listing page with filters (country, program type, ranking), a `/universities/[slug]` detail page with tabs (Overview, Programs, Admissions, Student Life), and reusable components (UniversityCard, UniversityFilters, UniversityHeader, UniversityTabs, UniversitySidebar). No new backend code was needed — the geo-space and content type system from earlier phases provides the data layer.

## Components

### Frontend (SvelteKit / Svelte 5)

1. **src/routes/universities/+page.server.ts** — Server load fetching university list from API with filters
2. **src/routes/universities/+page.svelte** — Listing page with filter sidebar and card grid
3. **src/routes/universities/[slug]/+page.server.ts** — Server load fetching single university by slug
4. **src/routes/universities/[slug]/+page.svelte** — Detail page with header, tabs, sidebar
5. **src/lib/universities/UniversityCard.svelte** — Card component for university listing
6. **src/lib/universities/UniversityFilters.svelte** — Filter sidebar with country, program, ranking controls
7. **src/lib/universities/UniversityHeader.svelte** — University detail header with name, location, logo
8. **src/lib/universities/UniversityTabs.svelte** — Tab navigation for Overview, Programs, Admissions, Student Life
9. **src/lib/universities/UniversitySidebar.svelte** — Sidebar with quick facts, deadlines, contact info
10. **src/lib/universities/index.ts** — Barrel export

## Files Created

1. `apps/main/src/routes/universities/+page.server.ts`
2. `apps/main/src/routes/universities/+page.svelte`
3. `apps/main/src/routes/universities/[slug]/+page.server.ts`
4. `apps/main/src/routes/universities/[slug]/+page.svelte`
5. `apps/main/src/lib/universities/UniversityCard.svelte`
6. `apps/main/src/lib/universities/UniversityFilters.svelte`
7. `apps/main/src/lib/universities/UniversityHeader.svelte`
8. `apps/main/src/lib/universities/UniversityTabs.svelte`
9. `apps/main/src/lib/universities/UniversitySidebar.svelte`
10. `apps/main/src/lib/universities/index.ts`

## Files Modified

None — builds on existing geo-space infrastructure.

## API Routes

No new API routes. Uses existing geo-space and content endpoints.

## Key Decisions

1. **Geo-space as university entity** — Universities are represented as geo-spaces with `space_type = 'university'`. This avoids a dedicated university table and reuses the existing space infrastructure (membership, content, follow).

2. **SSR for listing and detail** — Both pages use `+page.server.ts` load functions for SSR, ensuring university pages are crawlable by search engines and load fast on initial render.

3. **Tab-based detail layout** — The detail page uses a tabbed layout (Overview, Programs, Admissions, Student Life) rather than a single long page. Each tab loads its content from the space's content items filtered by type.

4. **Filter sidebar on listing** — Filters (country, program type, ranking) are applied as URL search params, making filter states shareable and bookmarkable.

5. **No new backend needed** — The university system is entirely frontend-driven, consuming existing API endpoints. This is possible because the geo-space system already supports custom fields (program types, rankings) and content types (programs, admissions info).

## Verification

### Frontend
- `bun run check` — Clean
- `bun run build` — Build succeeds

## Known Deferrals

- University comparison feature (requires Section 30 comparison system integration)
- Program-level filtering and search (requires Meilisearch integration)
- University application tracking (requires user dashboard integration)
- Real-time application deadline notifications (requires notification system)
- University-specific structured data for SEO (requires Section 26 schema extensions)

## Next Steps

Section 44 (Documentation System) adds audience-gated content; Section 45 (Post Create Page) adds the content creation flow that feeds university spaces.
