# Section 26 — SEO & Performance (Phase 7 Additions)

**Date:** 2026-06-27
**Phase:** 7
**Status:** Complete

## Overview

Section 26's backend (OG image generation, XML sitemap, seo.sql queries) was completed in Phase 5. Phase 7 adds the frontend components that consume the SEO data: `MetadataHead.svelte` for meta tags, `JsonLd.svelte` for structured data injection, and `structuredData.ts` helpers for common schema types. These components are integrated into the home page and create page.

## Components

### Frontend (Svelte 5)

1. **src/lib/seo/MetadataHead.svelte** — Renders OG, Twitter Card, canonical, and robots meta tags via `<svelte:head>`; accepts `title`, `description`, `image`, `url`, `type`, `noindex` props
2. **src/lib/seo/JsonLd.svelte** — Renders JSON-LD structured data in a `<script type="application/ld+json">` tag; accepts any valid JSON-LD schema object
3. **src/lib/seo/structuredData.ts** — Helper functions returning schema objects:
   - `articleSchema()` — Article structured data for blog posts
   - `organizationSchema()` — Organization schema for the platform
   - `breadcrumbSchema()` — Breadcrumb navigation schema
   - `websiteSchema()` — Website schema for homepage
   - `faqSchema()` — FAQ schema for FAQ pages
4. **src/lib/seo/index.ts** — Barrel export

### Route Integration

- **Home page** (`+page.svelte`) — Uses `MetadataHead` with `createWebsiteSchema()` and `JsonLd`
- **Create page** (`create/+page.svelte`) — Uses `MetadataHead` with `noindex={true}` to prevent indexing of content creation pages

## Files Created

1. `apps/main/src/lib/seo/MetadataHead.svelte`
2. `apps/main/src/lib/seo/JsonLd.svelte`
3. `apps/main/src/lib/seo/structuredData.ts`
4. `apps/main/src/lib/seo/index.ts`

## Files Modified

1. `apps/main/src/routes/+page.svelte` — Added MetadataHead and JsonLd components
2. `apps/main/src/routes/create/+page.svelte` — Added MetadataHead with noindex

## API Routes

No new API routes. Backend endpoints (`/sitemap.xml`, `/og/{id}.png`) were implemented in Phase 5.

## Key Decisions

1. **Component-level SEO** — Each page composes its own `MetadataHead` and `JsonLd` rather than using a global layout-level SEO component. This gives pages full control over their metadata without fighting layout defaults.

2. **noindex on create pages** — The create/edit page uses `noindex` to prevent search engines from indexing content creation forms, which have no SEO value and could confuse crawlers.

3. **Schema helpers as functions** — `structuredData.ts` exports functions rather than pre-built objects, allowing pages to pass dynamic data (e.g., article title, publish date) at call time.

4. **Svelte 5 runes for reactive metadata** — `MetadataHead` uses `$derived` for computed meta tag values, ensuring they update reactively when page data changes.

## Verification

### Frontend
- `bun run check` — Clean (main app)
- `bun run build` — Build succeeds

## Known Deferrals

- Dynamic OG image generation with real text rendering (V1 uses placeholder background)
- Canonical URL auto-generation from slug
- Page-specific `og_title`, `og_description`, `og_image` database columns
- Sitemap Valkey caching for performance
- Structured data for Learning platform courses (Section 36 dependency)

## Next Steps

Section 39 (Space Types & University) adds university-specific structured data; Section 44 (Documentation System) adds audience-gated SEO metadata.
