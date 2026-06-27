# Section 26: SEO & Performance — Implementation Summary

**Date:** 2026-06-27  
**Status:** Complete

## Overview
Implemented SEO and performance optimization features for JapaConnect, including Open Graph image generation, XML sitemap generation, and frontend structured data components.

## Components Implemented

### Backend (Go)

1. **OG Image Generator** (`internal/seo/ogimage.go`)
   - Generates 1200×630 PNG images for social sharing
   - V1: Placeholder colored background with TODO for real text rendering
   - 24-hour cache headers
   - Route: `GET /og/{id}.png`

2. **Sitemap Generator** (`internal/seo/sitemap.go`)
   - Generates XML sitemaps for search engines
   - Includes homepage + all published content
   - 1-hour cache headers
   - Route: `GET /sitemap.xml`

3. **SEO SQL Queries** (`internal/db/queries/seo.sql`)
   - `ListContentForSitemap`: Returns published content for sitemap generation
   - `GetPageMetadataBySlug`: Returns page metadata with author info via JOIN

4. **Types** (`internal/seo/types.go`)
   - `PageMetadata`: SEO metadata for content pages
   - `OGImageRequest`: Request for OG image generation
   - `OGImageResult`: Response from OG image generation

5. **Tests** (`internal/seo/seo_test.go`)
   - 4 tests covering XML encoding, metadata structures, and handler existence

### Frontend (Svelte 5)

1. **MetadataHead Component** (`apps/main/src/lib/seo/MetadataHead.svelte`)
   - Renders OG, Twitter Card, canonical, and robots meta tags
   - Supports `noindex` prop for pages that shouldn't be indexed
   - Uses Svelte 5 runes for reactive metadata

2. **JsonLd Component** (`apps/main/src/lib/seo/JsonLd.svelte`)
   - Renders JSON-LD structured data in `<svelte:head>`
   - Accepts any valid JSON-LD schema

3. **Structured Data Helpers** (`apps/main/src/lib/seo/structuredData.ts`)
   - `createArticleSchema`: Article structured data for blog posts
   - `createOrganizationSchema`: Organization schema for the platform
   - `createBreadcrumbSchema`: Breadcrumb navigation schema
   - `createWebsiteSchema`: Website schema for homepage
   - `createFaqSchema`: FAQ schema for FAQ pages

4. **Barrel Export** (`apps/main/src/lib/seo/index.ts`)
   - Exports all components and helpers

### Route Updates

1. **Home Page** (`apps/main/src/routes/+page.svelte`)
   - Updated to use `MetadataHead` and `JsonLd` components
   - Includes `createWebsiteSchema()` structured data

2. **Create Page** (`apps/main/src/routes/create/+page.svelte`)
   - Updated to use `MetadataHead` with `noindex={true}`
   - Prevents indexing of content creation pages

### Server Wiring

- Added `seo` import to `internal/server/server.go`
- Instantiated `sitemapHandler` and `ogImageHandler`
- Mounted routes in public bot-facing endpoints group (requires tenant resolution, no authentication)

## Files Created/Modified

### Created
- `internal/seo/types.go` (53 lines)
- `internal/seo/ogimage.go` (67 lines)
- `internal/seo/sitemap.go` (111 lines)
- `internal/seo/seo_test.go` (78 lines)
- `internal/db/queries/seo.sql` (18 lines)
- `apps/main/src/lib/seo/MetadataHead.svelte` (62 lines)
- `apps/main/src/lib/seo/JsonLd.svelte` (11 lines)
- `apps/main/src/lib/seo/structuredData.ts` (131 lines)
- `apps/main/src/lib/seo/index.ts` (4 lines)

### Modified
- `internal/server/server.go` (added SEO imports and route wiring)
- `apps/main/src/routes/+page.svelte` (added MetadataHead and JsonLd)
- `apps/main/src/routes/create/+page.svelte` (added MetadataHead with noindex)

## Verification

### Backend
- ✅ `go build ./...` — Clean build
- ✅ `go vet ./...` — No issues
- ✅ `go test ./internal/seo/...` — 4/4 tests pass

### Frontend
- ✅ `bun run check` — 0 errors, 10 warnings (pre-existing)
- ✅ `bun run build` — Build succeeds

## Key Decisions

1. **V1 OG Image Generation**: Used placeholder colored background instead of real text rendering to avoid adding `golang.org/x/image` dependency. Text rendering deferred to Phase 7.

2. **Sitemap Caching**: 1-hour cache header (`max-age=3600`) balances freshness with performance.

3. **Tenant Scoping**: All queries scoped by `tenant_id` per multi-tenancy architecture.

4. **No Authentication Required**: SEO endpoints (sitemap, OG images) are public for search engine crawlers.

5. **MetadataHead Component**: Single component handles all meta tags, reducing duplication across pages.

## Known Issues / Deferrals

1. **OG Image Text Rendering**: V1 uses placeholder background. Real text rendering with `golang.org/x/image/font` deferred to Phase 7.

2. **OG Image Caching**: No Valkey caching in V1. Could be added later for performance.

3. **Canonical URL Auto-generation**: Currently relies on `canonical_url` field in content. Auto-generation from slug not implemented.

4. **Page Metadata Database Fields**: `og_title`, `og_description`, `og_image` columns not in database schema. Using `title` as fallback. Migration may be needed later.

## Next Steps

1. **Phase 7**: Implement real OG image text rendering
2. **Database Migration**: Add `description`, `og_title`, `og_description`, `og_image` columns to `contents` table if needed
3. **Valkey Caching**: Add caching for OG images and sitemap
4. **Canonical URL Auto-generation**: Implement auto-canonical URL from slug