# Section 15 — Block Editor (TipTap): Implementation Complete

**Date:** 2026-06-25
**Phase:** 2
**Section:** 15
**Status:** Complete

## Overview

Section 15 implements the rich-text block editor that powers content creation
across JapaConnect. The backend provides three new packages — a TipTap-based SSR
renderer that shells out to Bun for server-side HTML output, a mention extraction
and notification pipeline with pluggable checker interfaces for cross-section
dependencies, and a link preview fetcher with SSRF protection that blocks 17
private/reserved CIDR ranges. The frontend ships eight Svelte 5 components built
on TipTap v3.27.1, including a custom editor wrapper (no `@tiptap/svelte` package
exists), slash-command palette, mention/tag/interlink popups, media embeds, link
preview cards, and syntax-highlighted code blocks.

## Components Implemented

### internal/renderer Package (3 files + 1 script)

**renderer.go** (90 lines)

- `Renderer` struct that shells out to a Bun script for TipTap SSR rendering
- Accepts TipTap JSON content and returns rendered HTML
- Valkey cache integration — cache key derived from content hash, TTL-based
  invalidation
- `Render(ctx, jsonContent)` method with cache check, Bun subprocess execution,
  and cache write
- Graceful degradation — if the Bun binary is not found, returns an error that
  the handler maps to 503

**handlers.go** (85 lines)

- `Handlers` struct with `*Renderer` dependency
- `NewHandlers` constructor
- `POST /api/v1/content/render` handler — requires authentication and tenant
  context, validates the request body contains TipTap JSON, calls the renderer,
  and returns the HTML string in the standard `{"data": ...}` envelope
- Error mapping: renderer errors to 503, invalid input to 400, auth failures to
  401

**renderer_test.go** (58 lines)

- 2 unit tests: renderer cache key generation, handler request validation

**scripts/tiptap-ssr.ts** (90 lines)

- Bun script that imports TipTap core and extensions, accepts JSON content on
  stdin, and writes rendered HTML to stdout
- Extensions loaded: StarterKit, Underline, Link, Image, Mention, Placeholder,
  CharacterCount
- Runs as a subprocess spawned by the Go renderer — not a long-lived server

### internal/mentions Package (3 files)

**mentions.go** (158 lines)

- `ExtractMentions` — walks a TipTap JSON document and collects all mention
  nodes (type `mention` with `entity_type` and `id` attributes)
- `FilterMentions` — applies three pluggable checker interfaces to determine
  which mentions should trigger notifications:
  - `BlockChecker` — whether the mentioning user is blocked by the mentioned
    user (Section 10, not yet implemented)
  - `SpaceMemberChecker` — whether the mentioned user is a member of the space
    where the content lives (Section 19, not yet implemented)
  - `UserStatusChecker` — whether the mentioned user's account is active,
    suspended, or deleted
- Returns a filtered list of mention targets eligible for notification

**stubs.go** (31 lines)

- `NoopBlockChecker`, `NoopSpaceMemberChecker`, `NoopUserStatusChecker` —
  implementations that return permissive defaults (nothing blocked, everyone is
  a member, all users active)
- Used until Sections 10 (Blocking) and 19 (Spaces Membership) provide real
  implementations
- Allows the mentions package to compile and test independently

**mentions_test.go** (176 lines)

- 5 unit tests with mock implementations of all three checker interfaces:
  - `ExtractMentions` walks nested TipTap JSON correctly
  - `FilterMentions` removes blocked users
  - `FilterMentions` removes non-space-members
  - `FilterMentions` removes inactive users
  - `FilterMentions` with noop checkers passes all mentions through

### internal/linkpreview Package (4 files)

**ssrf.go** (100 lines)

- `IsPrivateIP` — checks an `net.IP` against 17 private/reserved CIDR ranges:
  - IPv4: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 127.0.0.0/8,
    169.254.0.0/16, 100.64.0.0/10, 192.0.0.0/24, 198.18.0.0/15,
    198.51.100.0/24, 203.0.113.0/24, 224.0.0.0/4, 240.0.0.0/4, 255.255.255.255/32
  - IPv6: ::1/128, fc00::/7, fe80::/10, ff00::/8
- `ValidateURL` — parses the URL, resolves the hostname via DNS, checks every
  resolved IP against `IsPrivateIP`, and rejects the URL if any IP is private
- `IsBlockedDomain` — checks against a blocklist of known internal/metadata
  domains (169.254.169.254 for cloud metadata, localhost variants, etc.)
- DNS resolution check prevents DNS rebinding — the resolved IP is checked, not
  just the hostname

**fetcher.go** (194 lines)

- `Fetcher` struct with Valkey cache and HTTP client
- `Fetch(ctx, url)` method — validates URL (SSRF check), checks cache, performs
  HTTP GET with timeout, parses Open Graph tags from HTML, caches result
- `parseOGTags` — lightweight HTML walker that extracts `og:title`,
  `og:description`, `og:image`, `og:url` from `<meta property="...">` tags
  without a full DOM parser
- Cache key: SHA-256 hash of the URL, TTL-based
- Response struct: `Title`, `Description`, `Image`, `URL`, `Favicon`

**handlers.go** (68 lines)

- `Handlers` struct with `*Fetcher` dependency
- `NewHandlers` constructor
- `GET /api/v1/link-preview` handler — requires authentication and tenant
  context, validates the `url` query parameter, calls the fetcher, returns the
  preview data in the standard envelope
- Error mapping: SSRF violations to 403, fetch failures to 502, invalid URL to
  400

**ssrf_test.go** (76 lines)

- 3 unit tests: `IsPrivateIP` correctly identifies private ranges,
  `ValidateURL` rejects URLs resolving to private IPs, `IsBlockedDomain`
  rejects cloud metadata endpoints

**fetcher_test.go** (67 lines)

- 3 unit tests: `parseOGTags` extracts OG tags from HTML, cache hit returns
  cached result without HTTP call, fetch timeout returns error

### Server Wiring

- `renderer.NewHandlers(renderer.NewRenderer(valkeyClient))` and
  `linkpreview.NewHandlers(linkpreview.NewFetcher(valkeyClient))` created in
  `NewServer`
- 2 new API routes registered under `/api/v1`
- 2 new permission keys bootstrapped at startup
- `server.go` now 384 lines (was 366 after Section 14)

### Frontend: TipTap Editor Components (9 files)

TipTap v3.27.1 installed via `bun add` — 9 packages:
`@tiptap/core`, `@tiptap/pm`, `@tiptap/starter-kit`, `@tiptap/extension-underline`,
`@tiptap/extension-link`, `@tiptap/extension-image`, `@tiptap/extension-mention`,
`@tiptap/extension-placeholder`, `@tiptap/extension-character-count`

All components in `apps/main/src/lib/components/editor/`:

**Editor.svelte** (226 lines)

- Main editor wrapper using Svelte 5 runes (`$state`, `$derived`, `$effect`)
- Initializes TipTap editor instance with configurable extensions
- Two-way binding via `bind:value` pattern — accepts initial content as TipTap
  JSON, emits updates on transaction
- Toolbar with formatting buttons (bold, italic, underline, strike, link,
  heading levels, bullet/ordered list, code block, image)
- Character count display using `@tiptap/extension-character-count`
- Custom Svelte wrapper since no `@tiptap/svelte` package exists — manages the
  editor lifecycle via `$effect` (create on mount, destroy on unmount)

**SlashCommand.svelte** (106 lines)

- `/` command palette — appears when user types `/` in the editor
- Keyboard navigation (arrow keys, Enter to select, Escape to dismiss)
- Fixed positioning relative to the cursor
- Command categories: headings, lists, code blocks, images, embeds
- Mock command data — real command registry deferred to Section 45

**MentionPopup.svelte** (131 lines)

- `@` user mention popup — appears when user types `@`
- Keyboard navigation with fixed positioning
- Displays user avatar, display name, and username
- Inserts a TipTap mention node with `entity_type: "user"` and the user's ID
- Mock user data — real API integration deferred to Section 45

**TagPopup.svelte** (131 lines)

- `#` tag search popup — appears when user types `#`
- Keyboard navigation with fixed positioning
- Inserts a TipTap mention node with `entity_type: "tag"` and the tag slug
- Mock tag data — real API integration deferred to Section 45

**InterlinkPopup.svelte** (150 lines)

- `[[` content/space interlink popup — appears when user types `[[`
- Keyboard navigation with fixed positioning
- Searches across content and spaces for interlinking
- Inserts a custom TipTap node with `entity_type` and `id` attributes
- Mock data — real API integration deferred to Section 45

**MediaEmbed.svelte** (149 lines)

- YouTube, Vimeo, image, and video embed rendering
- URL pattern matching for provider detection (YouTube, Vimeo)
- Responsive iframe embedding with 16:9 aspect ratio
- Fallback rendering for unrecognized URLs (plain link)

**LinkPreview.svelte** (122 lines)

- Rich link preview cards — fetches preview data from `/api/v1/link-preview`
- Displays title, description, image thumbnail, and favicon
- Loading state with skeleton placeholder
- Error state with fallback to plain link
- Calls the backend link preview endpoint (SSRF-protected)

**CodeBlock.svelte** (120 lines)

- Syntax-highlighted code blocks using TipTap's code block extension
- Language selector dropdown
- Copy-to-clipboard button
- Line numbers display
- Theme-aware styling (light/dark mode via CSS custom properties)

**index.ts** (9 lines)

- Barrel exports for all 8 editor components
- Single import point: `import { Editor, SlashCommand, ... } from '$lib/components/editor'`

## Test Summary

| Package | Test Count | Result |
|---------|-----------|--------|
| internal/config | 4 | PASS |
| internal/server | 9 | PASS |
| internal/tenant | 3 | PASS |
| internal/auth | 22 | PASS |
| internal/rbac | 4 | PASS |
| internal/features | 5 | PASS |
| internal/geoip | 7 | PASS |
| internal/valkey | 6 | PASS |
| internal/customfields | 37 | PASS |
| internal/spaces | 13 | PASS |
| internal/content | 10 | PASS |
| internal/renderer | 2 | PASS |
| internal/mentions | 5 | PASS |
| internal/linkpreview | 6 | PASS |
| **Total** | **131** | **ALL PASS** |

## Build Verification

| Command | Result |
|---------|--------|
| `go test ./... -count=1` | ok — all 14 packages pass |
| `go build ./...` | success |
| `go vet ./...` | clean |
| Frontend TypeScript check | clean |
| Frontend build (SSR) | 252ms |
| Frontend build (client) | 3.11s |

## API Routes Added

2 routes registered under `/api/v1`:

| # | Method | Path | Auth | Permission | Description |
|---|--------|------|------|------------|-------------|
| 1 | POST | `/content/render` | Required | `content.render` | Render TipTap JSON to HTML via SSR |
| 2 | GET | `/link-preview` | Required | `link_preview.fetch` | Fetch Open Graph preview for a URL |

## Permission Keys Registered

2 new permission keys bootstrapped at server startup (added to the existing
`bootstrapPermissions` slice alongside the 31 from Sections 05, 06, and 14):

| Key | Description |
|-----|-------------|
| `content.render` | Render TipTap content to HTML |
| `link_preview.fetch` | Fetch link previews (SSRF-protected) |

All 33 permissions (11 Section 06 + 9 Section 05 + 11 Section 14 + 2 Section 15)
are assigned to the `super_admin_role`, which is granted to the `super_admin`
system group. Every operation is idempotent.

## Files Created

| # | File | Purpose |
|---|------|---------|
| 1 | `internal/renderer/renderer.go` | TipTap SSR renderer (Bun subprocess + Valkey cache) |
| 2 | `internal/renderer/handlers.go` | POST /content/render handler |
| 3 | `internal/renderer/renderer_test.go` | 2 renderer tests |
| 4 | `internal/renderer/scripts/tiptap-ssr.ts` | Bun SSR script with TipTap extensions |
| 5 | `internal/mentions/mentions.go` | ExtractMentions, FilterMentions, checker interfaces |
| 6 | `internal/mentions/stubs.go` | Noop checker implementations for Sections 10/19 |
| 7 | `internal/mentions/mentions_test.go` | 5 mention tests with mocks |
| 8 | `internal/linkpreview/ssrf.go` | IsPrivateIP (17 CIDRs), ValidateURL, IsBlockedDomain |
| 9 | `internal/linkpreview/fetcher.go` | Link preview fetcher with Valkey cache, OG tag parser |
| 10 | `internal/linkpreview/handlers.go` | GET /link-preview handler |
| 11 | `internal/linkpreview/ssrf_test.go` | 3 SSRF protection tests |
| 12 | `internal/linkpreview/fetcher_test.go` | 3 fetcher tests |
| 13 | `apps/main/src/lib/components/editor/Editor.svelte` | Main TipTap editor wrapper (Svelte 5 runes) |
| 14 | `apps/main/src/lib/components/editor/SlashCommand.svelte` | / command palette |
| 15 | `apps/main/src/lib/components/editor/MentionPopup.svelte` | @ user mention popup |
| 16 | `apps/main/src/lib/components/editor/TagPopup.svelte` | # tag search popup |
| 17 | `apps/main/src/lib/components/editor/InterlinkPopup.svelte` | [[ content/space interlink popup |
| 18 | `apps/main/src/lib/components/editor/MediaEmbed.svelte` | YouTube/Vimeo/image/video embeds |
| 19 | `apps/main/src/lib/components/editor/LinkPreview.svelte` | Rich link preview cards |
| 20 | `apps/main/src/lib/components/editor/CodeBlock.svelte` | Syntax-highlighted code blocks |
| 21 | `apps/main/src/lib/components/editor/index.ts` | Barrel exports |

## Files Modified

| # | File | Change |
|---|------|--------|
| 1 | `internal/server/server.go` | Added `renderer` and `linkpreview` imports; created renderer and linkpreview handlers; registered 2 new routes; added 2 permission keys to `bootstrapPermissions` (384 lines total) |
| 2 | `apps/main/package.json` | Added 9 TipTap packages (v3.27.1) |
| 3 | `apps/main/bun.lock` | Updated lockfile with TipTap dependency tree |

## Key Decisions

- **No `@tiptap/svelte` package exists** — TipTap does not ship an official
  Svelte integration. A custom Svelte 5 wrapper was built using `$effect` to
  manage the editor lifecycle (create on mount, destroy on unmount) and `$state`
  for content binding. This is the standard approach for Svelte 5 + TipTap.
- **SSR via Bun subprocess, not embedded JS** — the Go backend shells out to a
  Bun script for TipTap rendering rather than embedding a JavaScript runtime.
  This keeps the Go binary lean and lets the TipTap extension set evolve
  independently. The Bun script is invoked per-render with Valkey caching to
  amortize subprocess overhead.
- **SSRF protection blocks 17 private/reserved CIDR ranges** — the link preview
  fetcher resolves DNS before connecting and checks every resolved IP against
  the private range list. This prevents DNS rebinding attacks where a hostname
  resolves to a private IP. The blocklist also includes cloud metadata endpoints
  (169.254.169.254) that are not in standard private ranges.
- **Mention filtering uses interfaces for cross-section dependencies** — the
  `BlockChecker`, `SpaceMemberChecker`, and `UserStatusChecker` interfaces allow
  the mentions package to compile and test without Sections 10 (Blocking) and
  19 (Spaces Membership). Noop stubs provide permissive defaults until the real
  implementations are wired in.
- **Link preview fetch is synchronous** — the current implementation performs the
  HTTP GET inline during the request. Async fetching via Asynq is deferred to
  Phase 6 when the task queue infrastructure is built. Valkey caching ensures
  repeated requests for the same URL do not trigger redundant fetches.
- **Media resolution (mediaId to URL) is a TODO placeholder** — the MediaEmbed
  component accepts media IDs but resolves them to placeholder URLs. The real
  media resolution pipeline depends on Section 34 (Media System).

## Known Issues and Deferrals

- **Popup components use mock data** — SlashCommand, MentionPopup, TagPopup, and
  InterlinkPopup all use hardcoded mock data for their suggestions. Real API
  integration (searching users, tags, content, spaces) is deferred to Section 45
  (Post Create Page) which wires the editor into the full content creation flow.
- **Asynq async link preview deferred to Phase 6** — the link preview fetch is
  synchronous. When Asynq is wired, the fetcher will enqueue a background task
  for slow URLs and return a cached result if available, falling back to a
  synchronous fetch only for cache misses within the request timeout.
- **Mention notification dispatch deferred** — `ExtractMentions` and
  `FilterMentions` identify who should be notified, but the actual notification
  creation (writing to a notifications table, pushing via WebSocket) is deferred
  to the notification system (Section 23).
- **Block checker and space member checker are noop stubs** — the real
  implementations depend on Section 10 (Blocking) and Section 19 (Spaces
  Membership). Until those sections are built, all mentions pass through the
  filter unconditionally.
- **TipTap SSR script is not a long-lived server** — each render spawns a new
  Bun process. This is acceptable with Valkey caching for the expected render
  volume, but if render throughput becomes a bottleneck, the script can be
  converted to a long-lived Bun server with an HTTP API without changing the Go
  renderer interface.

## Next Steps

Section 16 — Page Builder. This section depends on:

- Section 15 editor components (the page builder composes editor instances into
  a drag-and-drop layout)
- Section 14 content rows (pages are a content type stored in `contents`)
- Section 06 custom fields (page layouts are stored as `field_values` JSONB)
