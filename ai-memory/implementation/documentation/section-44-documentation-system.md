# Section 44 — Documentation System

**Date:** 2026-06-27
**Phase:** 7
**Status:** Complete

## Overview

Section 44 implements the documentation system for JapaConnect: a dedicated docs SvelteKit app (`apps/docs/`, port 3004) with RBAC-gated audience sections (user, admin, developer), a sidebar navigation, full-text search, and an admin backend for creating/editing documentation pages. The backend provides 6 endpoints for CRUD operations on doc pages.

## Components

### Backend (Go)

1. **internal/docs/types.go** — DocPage, CreateDocPageInput, UpdateDocPageInput types
2. **internal/docs/store.go** — CRUD against `doc_pages` table with audience filtering
3. **internal/docs/service.go** — Service wrapping store with ListDocPages, GetDocPage, CreateDocPage, UpdateDocPage, DeleteDocPage, ListAllDocPages
4. **internal/docs/handlers.go** — 6 HTTP handlers: ListDocPages, GetDocPage, CreateDocPage, UpdateDocPage, DeleteDocPage, ListAllDocPages
5. **internal/docs/docs_test.go** — Tests for types, store, and service

### Migration

- **000032_doc_pages.sql** — `doc_pages` table with `audience` (text: user, admin, developer), `slug`, `title`, `content` (markdown), `sort_order`, `is_published`, unique constraint on `(tenant_id, audience, slug)`

### Frontend (SvelteKit / Svelte 5)

1. **apps/docs/src/routes/+layout.svelte** — Root layout with DocSidebar and DocContent
2. **apps/docs/src/routes/+page.svelte** — Home/landing page for docs
3. **apps/docs/src/routes/[audience]/+page.server.ts** — Server load fetching docs for audience
4. **apps/docs/src/routes/[audience]/+page.svelte** — Audience-gated doc listing
5. **apps/docs/src/routes/[audience]/[slug]/+page.server.ts** — Server load fetching single doc page
6. **apps/docs/src/routes/[audience]/[slug]/+page.svelte** — Single doc page with markdown rendering
7. **apps/docs/src/lib/components/DocSidebar.svelte** — Sidebar navigation with audience sections
8. **apps/docs/src/lib/components/DocContent.svelte** — Markdown content renderer
9. **apps/docs/src/lib/components/DocSearch.svelte** — Client-side search input
10. **apps/docs/src/lib/api.ts** — API client for docs backend
11. **apps/docs/src/lib/index.ts** — Barrel export

## Files Created

### Backend
1. `internal/docs/types.go`
2. `internal/docs/store.go`
3. `internal/docs/service.go`
4. `internal/docs/handlers.go`
5. `internal/docs/docs_test.go`
6. `migrations/000032_doc_pages.sql`

### Frontend
7. `apps/docs/src/routes/+layout.svelte`
8. `apps/docs/src/routes/+page.svelte`
9. `apps/docs/src/routes/[audience]/+page.server.ts`
10. `apps/docs/src/routes/[audience]/+page.svelte`
11. `apps/docs/src/routes/[audience]/[slug]/+page.server.ts`
12. `apps/docs/src/routes/[audience]/[slug]/+page.svelte`
13. `apps/docs/src/lib/components/DocSidebar.svelte`
14. `apps/docs/src/lib/components/DocContent.svelte`
15. `apps/docs/src/lib/components/DocSearch.svelte`
16. `apps/docs/src/lib/api.ts`
17. `apps/docs/src/lib/index.ts`

## Files Modified

1. `internal/server/server.go` — Added docs import, service/handler constructors, 6 route registrations, 1 permission key (`docs.admin`)

## API Routes

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| GET | `/docs/{audience}` | None | — | List published doc pages for audience |
| GET | `/docs/{audience}/{slug}` | None | — | Get single doc page by audience and slug |
| GET | `/admin/docs/{audience}` | Required | `docs.admin` | List all doc pages (including drafts) |
| POST | `/admin/docs` | Required | `docs.admin` | Create a new doc page |
| PATCH | `/admin/docs/{id}` | Required | `docs.admin` | Update a doc page |
| DELETE | `/admin/docs/{id}` | Required | `docs.admin` | Delete a doc page |

## Permission Keys

| Key | Description |
|-----|-------------|
| `docs.admin` | Manage documentation pages (create, edit, delete, review) |

## Key Decisions

1. **Audience-gated routing** — The `[audience]` dynamic segment filters docs by audience (user, admin, developer). Each audience has its own sidebar and content tree. This allows serving different documentation to different user roles without separate apps.

2. **Markdown content** — Doc pages store content as Markdown, rendered client-side. This is simpler than a WYSIWYG editor and works well for technical documentation.

3. **SSR for public docs** — Public documentation pages (user audience) are server-side rendered for SEO. Admin/developer docs can be client-rendered since they require authentication.

4. **RBAC on admin endpoints** — The `docs.admin` permission key gates all mutation endpoints. Only users with this permission can create, edit, or delete documentation.

5. **Separate SvelteKit app** — Docs lives in `apps/docs/` as a standalone SvelteKit app rather than a route group in the main app. This allows independent deployment and caching strategies.

6. **DocSearch as client-side** — Search is implemented client-side using a simple text matching approach. Full-text search via Meilisearch can be added later without changing the component interface.

## Verification

### Backend
- `go build ./...` — Clean build
- `go vet ./...` — No issues
- `go test ./internal/docs/...` — All tests pass

### Frontend
- `bun run check` — Clean (docs app)
- `bun run build` — Build succeeds

## Known Deferrals

- Full-text search via Meilisearch integration
- Version history for doc pages (requires doc_revisions table)
- Rich text editor for doc creation (currently requires Markdown)
- API reference auto-generation from OpenAPI spec
- Doc page analytics (view counts, popular pages)
- Multi-language documentation (i18n system dependency)

## Next Steps

The documentation system is complete. Remaining Phase 7 work focuses on the main app's frontend polish and SEO integration.
