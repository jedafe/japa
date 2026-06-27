# Section 43 -- Bot & Crawler Management: Implementation Complete

**Date:** 2026-06-27
**Section:** 43
**Phase:** 6
**Status:** Complete

## Overview

Implemented bot and crawler management for JapaConnect. Covers a known-bots
registry with 9 pre-seeded bots, dynamic robots.txt generation, LLMS.txt
generator for AI crawlers, sitemap quality gates, and bot detection middleware.
All tables are tenant-scoped.

## Files Created (14)

| # | File | Lines | Purpose |
|---|------|-------|---------|
| 1 | `migrations/000028_bot_crawler_management.sql` | -- | 3 tables: known_bots, bot_events, sitemap_quality_rules + 9 seed rows |
| 2 | `internal/db/queries/bots.sql` | 45 | 6 sqlc queries |
| 3 | `internal/db/gen/bots.sql.go` | -- | sqlc-generated Go code |
| 4 | `internal/bots/types.go` | -- | Bot type/policy constants, registry entry struct |
| 5 | `internal/bots/store.go` | -- | CRUD for bot registry, events, quality rules |
| 6 | `internal/bots/service.go` | -- | Business logic: registry management, bot lookup |
| 7 | `internal/bots/handlers.go` | -- | 7 HTTP handlers (admin CRUD, robots.txt, LLMS.txt) |
| 8 | `internal/bots/middleware.go` | -- | Bot detection middleware, context helpers |
| 9 | `internal/bots/robots.go` | -- | Dynamic robots.txt generator from registry |
| 10 | `internal/bots/llms.go` | -- | LLMS.txt and LLMS-full.txt generator for AI crawlers |
| 11 | `internal/bots/quality.go` | -- | Sitemap quality gate evaluation |
| 12 | `internal/bots/bots_test.go` | -- | Type/policy constant tests, handler helper tests |
| 13 | `internal/bots/robots_test.go` | -- | robots.txt generation tests (blocked, allowed, limited, mixed) |
| 14 | `internal/bots/llms_test.go` | -- | LLMS.txt generation tests (basic, links, full, recent posts, tags, spaces) |
| 15 | `internal/bots/quality_test.go` | -- | Quality gate tests (space/content indexable, defaults) |
| 16 | `internal/bots/middleware_test.go` | -- | Middleware tests (no tenant, no UA, context, filtering, matching) |

## Database Schema

Migration `000028_bot_crawler_management.sql` creates:

- **known_bots** -- Registry of known bots/crawlers. 9 pre-seeded entries:
  Googlebot, Bingbot, GPTBot, ChatGPT-User, CCBot, Anthropic-ai, PerplexityBot,
  Bytespider, Amazonbot. Each has type (search_engine, ai_crawler, archive,
  scraper, other), policy (allow, block, limit), and crawl_delay.

- **bot_events** -- Event log of bot visits. Records bot_id, path, status_code,
  response_time. Used for analytics and quality monitoring.

- **sitemap_quality_rules** -- Admin-configurable rules for sitemap quality
  gates. Evaluates whether content/spaces should be included in sitemaps.

## Key Decisions

1. **9 pre-seeded bots** -- covers the major search engines (Google, Bing),
   AI crawlers (GPTBot, ChatGPT-User, CCBot, Anthropic-ai, PerplexityBot,
   Bytespider, Amazonbot). Additional bots added via admin API.

2. **Dynamic robots.txt** -- generated from the known_bots registry on each
   request. Admin changes take effect immediately without server restart.

3. **LLMS.txt standard** -- generates LLMS.txt and LLMS-full.txt files per
   the llms-txt.org standard. Provides AI crawlers with structured site
   information, recent posts, tags, and spaces.

4. **Sitemap quality gates** -- configurable rules evaluate content/spaces
   before including in sitemaps. Prevents low-quality or incomplete content
   from being indexed.

5. **Bot detection middleware** -- matches User-Agent strings against the
   registry. Sets bot info in request context for downstream handlers.

6. **Bot type classification** -- type field (search_engine, ai_crawler,
   archive, scraper, other) enables different policies per crawler category.
   AI crawlers can be blocked without affecting search engine indexing.

## API Routes (7)

| Method | Path | Handler |
|--------|------|---------|
| GET | /robots.txt | ServeRobotsTxt |
| GET | /llms.txt | ServeLLMsTxt |
| GET | /llms-full.txt | ServeLLMsFullTxt |
| GET | /api/v1/admin/bots | ListBots |
| POST | /api/v1/admin/bots | RegisterBot |
| PATCH | /api/v1/admin/bots/{id} | UpdateBot |
| DELETE | /api/v1/admin/bots/{id} | RemoveBot |

## Permission Keys (1)

- `bots.manage` -- Manage bot registry (admin)

## Test Results

57 tests covering:
- Bot type constants (search_engine, ai_crawler, archive, scraper, other)
- Policy constants (allow, block, limit)
- robots.txt generation (blocked bots, allowed bots, limited bots, default
  policy, empty registry, mixed policies, crawl delay format)
- LLMS.txt generation (basic, with links, no links, full version, recent
  posts, tags, spaces, all sections, empty data)
- Sitemap quality gates (space indexable, content indexable, defaults)
- Middleware (no tenant, no user agent, context helpers, user agent filtering,
  bot matching by UA string)
- Handler helpers (writeJSON, writeError, parsePageSize, parseOffset)

## Build Verification

| Command | Result |
|---------|--------|
| `go test ./internal/bots/...` | ok -- 57 tests pass |
| `go build ./...` | success |
| `go vet ./...` | clean |

## Deferrals

- Bot visit analytics dashboard (depends on Section 40 analytics)
- Automatic bot detection from access logs
- Crawl budget management (rate limiting per bot)
- Sitemap XML generation (currently only quality gates)
