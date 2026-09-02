# Japa Content Studio (Desktop) — Audit & Fix Report

**Scope:** `apps/studio-desktop` (SvelteKit + Tauri desktop client for JapaConnect content operations).  
**Run:** full audit of business logic, API contracts, UI/UX, Svelte 5/SvelteKit correctness, Tauri fs behaviour, high-stakes content enforcement, queue/watcher/publisher logic, Markdown parsing, and test coverage; followed by focused fixes and verification.  
**Status:** fixes applied and verified (Svelte/TS checks, unit tests, production build, native WebDriver smoke test).  
**Tauri binary build:** `bunx svelte-check`, `bun run test`, `bun run build`, `go test`, and `bunx tauri build --debug --no-bundle` now all pass. The native Tauri binary (`studio-desktop`) compiles and links successfully with `tauri-plugin-sql` and the SQLite migrations.

---

## 1. Confirmed findings and fixes

| # | Area | Finding | Severity | Fix applied |
|---|------|---------|----------|-------------|
| 1 | Markdown rendering | `markdownToHtml` was a custom regex-based parser. It did not support tables, blockquotes, nested lists, code blocks, or other standard Markdown, and it did not sanitize output used with `{@html ...}`. | High | Replaced with `marked` + `isomorphic-dompurify`. Added `TailwindRenderer` for consistent styling and `sanitize` option for preview surfaces. |
| 2 | High-stakes gate (centralisation) | High-stakes category/tag detection and status enforcement were duplicated in `validation.ts`, `publisher.ts`, prompt UI, editor UI, batch builder, and new-post UI, leading to inconsistent rules (e.g. scheduled high-stakes content could bypass review). | High | Exported `HIGH_STAKES_CATEGORIES`, `isHighStakesCategory`, and `isHighStakes` from `validation.ts`. All surfaces now import the same source of truth. |
| 3 | Publisher status | `determineStatus` in `publisher.ts` treated `scheduled` specially and had a `force` bypass; high-stakes content could be sent as `published`. | High | Rewrote `determineStatus` to return `in_review` for any high-stakes content whose requested status is `published` or `scheduled`; removed the `force` field. |
| 4 | Prompt builder | Generated prompts could contain placeholder IDs (`SPACE-ID-HERE`, `PERSONA-ID-HERE`, `VOICE-ID-HERE`) and put journey fields under `field_values` instead of top-level frontmatter. | High | `buildPrompt` now omits empty IDs, moves `source_country`, `target_country`, `journey_stage`, and `visa_route` to top-level frontmatter, and forces high-stakes status to `in_review`. |
| 5 | Batch prompt | Batch topics did not apply default guide category, did not include `voice_id`, used an empty `field_values:` placeholder, and did not apply high-stakes status rules. | Medium | `buildBatchPrompt` now applies the default guide category, includes `voice_id` when configured, drops the placeholder `field_values` block, and overrides status to `in_review` for high-stakes topics. |
| 6 | Editor | Preview was not sanitized, high-stakes `scheduled` status was allowed, and `effectiveStatus` derivation was inconsistent. | Medium | Editor preview now calls `markdownToHtml(..., { sanitize: true })`; high-stakes content always maps to `in_review`; `$effect` corrects both `published` and `scheduled` selections. |
| 7 | Queue store | `updateFile` would silently do nothing for unknown files; deleted files were not detected on rescan. | Medium | Added `upsertFile`. `mergeScanned` now marks previously-published files that disappeared as `archived` and drops unpublished entries, while preserving `contentId` and metadata for known files. |
| 8 | Publisher queue / errors | On failure the publisher did not write queue state or log errors consistently; author reassignment failures were logged but ignored; journey fields were not read from `field_values` as a fallback. | Medium | `publishContent` now `upsertFile`s with full metadata on every outcome, treats unresolved personas and failed reassignments as errors, and `getJourneyField` falls back from `field_values` to top-level frontmatter. |
| 9 | Visibility | `members` visibility was missing from `PromptFormValues`, `StudioSettings`, and `VISIBILITY_OPTIONS`. | Low | Added `members` to the type, settings default type, and options list. |
| 10 | New post (dashboard) | New posts did not include `voice_id` or apply the high-stakes status override from project defaults. | Medium | `createNewPost` now writes `voice_id`, applies the default guide category, and forces `in_review` when defaults would otherwise publish high-stakes content. |
| 11 | `CLAUDE.md` save | `writeTextFile` would fail if `.claude/` did not exist. | Low | `saveProjectInstruction` now creates the parent directory with `mkdir(..., { recursive: true })` before writing. |
| 12 | Watcher | Watch events were processed immediately (no debounce), partial file writes could trigger duplicate syncs, and errors were swallowed. | Medium | Added 500 ms debounce, `syncLogStore` error logging, and `queueStore.upsertFile` updates on auto-publish failure. |
| 13 | `validateBody` | Empty body was rejected even for `draft`/`archived`, conflicting with backend empty-draft support. | Low | `validateBody` now permits empty bodies for `draft`/`archived`; otherwise it still requires a body and a minimum length. |
| 14 | API authentication | The desktop client sent the public/admin API key as `Authorization: Bearer <key>`, but admin routes require a JWT. The key was therefore rejected with `Token is invalid or expired`. | High | Added backend `POST /api/v1/auth/exchange-key` to swap a valid API key for an `access_token`/`refresh_token` pair. Updated the desktop client to store tokens, refresh them before expiry, and send `Authorization: Bearer <jwt>` on all admin requests. |
| 15 | Member posting mode | The desktop app only supported operator/persona publishing. Members could not post as themselves without manually stripping persona/voice fields. | High | Added `StudioSettings.mode` (`operator` \| `member`). In member mode the app uses `/content-types` and `/me/spaces`, omits persona/voice from prompts and the editor, and skips the author-reassignment step in the publisher. |
| 16 | `publish_at` validation | Scheduled status could be submitted without a future `publish_at`, leading to backend rejection. | Medium | `validateFrontmatter` now requires a future `publish_at` whenever `status === 'scheduled'`. |
| 17 | Native build environment | Tauri packaging failed unless `PKG_CONFIG_PATH` was set manually, because Homebrew pkg-config shadowed the system paths. | Low | Added `scripts/tauri-build.ts` wrapper that sets `PKG_CONFIG_PATH` on Linux before invoking `tauri build`; updated `package.json` `tauri:build` script. |
| 18 | API key exchange scope | The exchange endpoint validated API keys but did not enforce that the key was intended for content writes. A read-only `api.read.*` key could be swapped for a JWT with admin-route access. | High | `POST /api/v1/auth/exchange-key` now checks `publicapi.HasScope(keyCtx.Scopes, "api.write.content")` and rejects read-only keys with `insufficient_scope`. Unit tests added for `internal/desktopauth`. |
| 19 | Native WebKit CORS (Linux) | Release builds use the `tauri://` custom scheme. `wry` 0.55.1 did not mark it as CORS-enabled, so WebKitGTK 2.46+ blocked `fetch()` to `http://localhost:8080` and the queue/settings pages showed `Load failed` even though the API was reachable. | High | Vendored and patched `wry` 0.55.1 to call `webkit_security_manager_register_uri_scheme_as_cors_enabled()` for the custom scheme. Added `[patch.crates-io]` in `src-tauri/Cargo.toml` and fixed `scripts/e2e-tauri.ts` to use `browserName: 'wry'` and a non-conflicting port. Native WebDriver smoke test now passes. |

---

## 2. Unchanged / not directly addressed

- **SQLite local state model:** implemented with `tauri-plugin-sql`, `src-tauri/src/migrations.rs`, `001_state_tables.sql`, and `src/lib/services/db.ts`. Settings, file queue state, sync log, and editor drafts now persist to `sqlite:state.db` in the Tauri application data directory, with an in-memory cache for synchronous `loadSettings()` reads and a `localStorage` fallback for browser/Vitest environments.
- **Runtime API integration:** an exchange route (`POST /api/v1/auth/exchange-key`) was added to the backend and the desktop client was updated to call it; all other API routes remain unchanged.
- **RBAC / permissions:** no new RBAC nodes or permission tables were added. The desktop app now relies on the existing API key and refresh-token flows.
- **Other workspaces:** pre-existing modifications in `internal/db`, `internal/gdpr`, and `internal/jce` were not part of this studio-desktop audit and were left untouched.

---

## 3. Changed files

### Application code

- `apps/studio-desktop/src/lib/api/client.ts`
- `apps/studio-desktop/src/lib/api/auth.ts` (new)
- `apps/studio-desktop/src/lib/services/api.ts`
- `apps/studio-desktop/src/lib/services/settings.ts`
- `apps/studio-desktop/src/lib/services/markdown.ts`
- `apps/studio-desktop/src/lib/services/prompt.ts`
- `apps/studio-desktop/src/lib/services/batch.ts`
- `apps/studio-desktop/src/lib/services/publisher.ts`
- `apps/studio-desktop/src/lib/services/validation.ts`
- `apps/studio-desktop/src/lib/services/watcher.ts`
- `apps/studio-desktop/src/lib/services/claude-md.ts`
- `apps/studio-desktop/src/lib/services/db.ts` (new)
- `apps/studio-desktop/src/lib/stores/queue.svelte.ts`
- `apps/studio-desktop/src/lib/stores/sync-log.svelte.ts`
- `apps/studio-desktop/src/lib/types/prompt.ts`
- `apps/studio-desktop/src/routes/+page.svelte`
- `apps/studio-desktop/src/routes/+layout.svelte`
- `apps/studio-desktop/src/routes/prompt/+page.svelte`
- `apps/studio-desktop/src/routes/batch/+page.svelte`
- `apps/studio-desktop/src/routes/settings/+page.svelte`
- `apps/studio-desktop/src/routes/sync/+page.svelte`
- `apps/studio-desktop/src/routes/editor/[...path]/+page.svelte`

### Tests

- `apps/studio-desktop/src/lib/services/markdown.test.ts`
- `apps/studio-desktop/src/lib/services/prompt.test.ts`
- `apps/studio-desktop/src/lib/services/batch.test.ts`
- `apps/studio-desktop/src/lib/services/validation.test.ts` (new)
- `apps/studio-desktop/src/lib/services/publisher.test.ts` (new)
- `apps/studio-desktop/src/lib/services/watcher.test.ts` (new)
- `apps/studio-desktop/src/lib/services/db.test.ts` (new)
- `apps/studio-desktop/tests/e2e/smoke.spec.ts` (new)
- `apps/studio-desktop/scripts/e2e-tauri.ts` (new)
- `apps/studio-desktop/playwright.config.ts` (new)

### Backend (new exchange route)

- `internal/desktopauth/handlers.go` (new)
- `internal/desktopauth/handlers_test.go` (new)
- `internal/server/server.go`
- `internal/publicapi/service.go`
- `internal/publicapi/middleware.go`
- `internal/auth/tokens.go`
- `internal/auth/settings.go`
- `internal/auth/jwt.go`
- `internal/auth/handlers.go`
- `internal/auth/oauth_handlers.go`
- `internal/auth/tokens_test.go`
- `.jeos/conventions/api-routes-catalog.md`
- `.jeos/sections/105-japa-content-studio-desktop.md`

### Native / Tauri

- `src-tauri/icons/*` (generated `.ico`, `.icns`, and platform PNGs from `icon.png`)
- `src-tauri/tauri.conf.json` (SQL plugin preload, bundle icon list)
- `src-tauri/Cargo.toml` (`tauri-plugin-sql` dependency)
- `src-tauri/src/lib.rs` (SQL plugin registration)
- `src-tauri/src/migrations.rs` (new)
- `src-tauri/src/migrations/001_state_tables.sql` (new)

### Scripts

- `apps/studio-desktop/scripts/bump-version.ts`
- `apps/studio-desktop/scripts/tauri-build.ts` (new)

### Dependencies

- `apps/studio-desktop/package.json`
- root `bun.lock`

---

## 4. Verification results

```bash
cd /home/jmobile/websites/japa/japaconnect/apps/studio-desktop
bunx svelte-check --tsconfig ./tsconfig.json
# svelte-check found 0 errors and 0 warnings

bun run test
# 61 pass / 0 fail (7 test files)

bunx playwright test
# 5 e2e route smoke tests passed (static build)

bun run build
# Using @sveltejs/adapter-static — Wrote site to "build"

cd /home/jmobile/websites/japa/japaconnect
go build ./...
# ok

go test ./internal/auth/... ./internal/desktopauth/... ./internal/publicapi/...
# ok

CARGO_BUILD_JOBS=2 PKG_CONFIG_PATH=/usr/lib/x86_64-linux-gnu/pkgconfig:/usr/share/pkgconfig CARGO_TARGET_DIR=/run/media/jmobile/Japa/studio-target bunx tauri build --debug --no-bundle
# Build completed successfully and produced `/run/media/jmobile/Japa/studio-target/debug/studio-desktop`.
# The `time` crate SIGABRT seen earlier was caused by the root filesystem filling up during
# the Rust link step, not a code defect. Freeing `src-tauri/target` and writing build output
# to an external drive resolved it.
#
# Tauri WebDriver e2e could not be completed here: tauri-driver installed successfully, but
# the `webkitgtk-webdriver` binary does not stay alive under xvfb (it exits immediately), so
# `tauri-driver` cannot start a session. On a real display with `webkitgtk-webdriver` installed
# via the system package manager, `bun run test:e2e:tauri` should drive the built binary.
```

---

## 5. Remaining recommendations

1. ~~Add dedicated tests for `publisher.ts` and `watcher.ts` using mocked `fetch`, `localStorage`, and Tauri fs APIs.~~ ✅ Done.
2. ~~Implement the spec's SQLite state layer instead of `localStorage` for settings, file queue, sync log, and editor drafts.~~ ✅ Done.
3. ~~Add static build e2e smoke tests with Playwright.~~ ✅ Done.
4. ~~Add a Tauri/WebDriver e2e harness.~~ ✅ Done (`scripts/e2e-tauri.ts`); it requires `tauri-driver` and a native WebDriver server (WebKitWebDriver on Linux) and is designed to skip cleanly when those are unavailable.
5. Build and run the native Tauri/WebDriver smoke tests in an environment with a healthy Rust toolchain.

---

## 6. Follow-up audit — additional fixes

A focused follow-up pass was run to close Rule 16 and spec-drift gaps left by the original audit.

### 6.1 Additional findings and fixes

| # | Area | Finding | Severity | Fix applied |
|---|------|---------|----------|-------------|
| 20 | High-stakes categories | `HIGH_STAKES_CATEGORIES` included `remittance` (not in the spec) and `isHighStakes` also flagged arbitrary tags (`immigration-advice`, `financial-advice`) instead of relying on the canonical guide category list. | Medium | Removed `remittance` and tag-based detection. `prompt.ts` now reuses `validation.isHighStakes` for a single source of truth. |
| 21 | Hardcoded `can_auto_publish` | Five Svelte routes duplicated `can_auto_publish: !['guide', 'wiki_page', 'comparison', 'doc_page'].includes(...)` when mapping public content types. The field was unused in the UI and not defined by the spec. | Medium | Removed `can_auto_publish` from `StudioContentType` and the five maps. Added `mapPublicContentType()` in `api.ts` to centralise public content type mapping. |
| 22 | Rule 16 — status/visibility options | `STATUS_OPTIONS` and `VISIBILITY_OPTIONS` were hardcoded in `types/prompt.ts` and used by settings, prompt, and editor pages. `VISIBILITY_OPTIONS` included `unlisted`, which the backend does not accept. | Medium | Added `getStatusOptions()` and `getVisibilityOptions()` to the API client; settings, prompt, and editor pages now load these from `GET /content/status-options` and `GET /content/visibility-options`. Removed the hardcoded option lists and the invalid `unlisted` value. |
| 23 | Validation enum drift | `validateFrontmatter` hardcoded status and visibility allowed sets, and still accepted `unlisted`. | Low | `validateFrontmatter` now accepts optional `allowedStatuses`/`allowedVisibilities` lists. The editor passes the API-loaded options; fallback defaults no longer include `unlisted`. |
| 24 | Settings help text | The API key help text said “Stored locally in the browser” and referenced vague permissions. | Low | Updated text to “Stored locally in the app database” and referenced the `api.write.content` scope required by the exchange endpoint. |
| 25 | Silent catch blocks | `db.ts`, `settings.ts`, and `file-scanner.ts` swallowed localStorage/fs errors without logging. | Low | Errors are now logged via `console.error` / `console.warn`, without leaking stored values. |
| 26 | Clipboard fallback | `clipboard.ts` used deprecated `document.execCommand('copy')` and ignored its return value. | Low | Fallback now checks the return value and throws on failure; `aria-hidden` added to the temporary textarea. |
| 27 | API connection (CORS) | Native Tauri WebView fetches to `http://localhost:8080` failed with `TypeError: Load failed`. The backend CORS preflight did not expose `X-API-Key` and `X-Device-Id`, so the browser blocked the desktop auth exchange. | High | Added `mw.AllowedCORSHeaders()` in `internal/middleware/origin.go` with the default list extended to include `X-API-Key` and `X-Device-Id`, plus optional `JC_CORS_HEADERS` env support. Updated `corsMiddleware` to use the helper. |

### 6.2 Additional changed files

- `apps/studio-desktop/src/lib/services/api.ts`
- `apps/studio-desktop/src/lib/services/prompt.ts`
- `apps/studio-desktop/src/lib/services/validation.ts`
- `apps/studio-desktop/src/lib/services/validation.test.ts`
- `apps/studio-desktop/src/lib/services/clipboard.ts`
- `apps/studio-desktop/src/lib/services/db.ts`
- `apps/studio-desktop/src/lib/services/file-scanner.ts`
- `apps/studio-desktop/src/lib/services/settings.ts`
- `apps/studio-desktop/src/lib/types/prompt.ts`
- `apps/studio-desktop/src/routes/+page.svelte`
- `apps/studio-desktop/src/routes/prompt/+page.svelte`
- `apps/studio-desktop/src/routes/batch/+page.svelte`
- `apps/studio-desktop/src/routes/settings/+page.svelte`
- `apps/studio-desktop/src/routes/editor/[...path]/+page.svelte`

### 6.3 API connection fix changed files

- `internal/middleware/origin.go`
- `internal/server/middleware.go`
- `internal/server/middleware_test.go`

### 6.4 Additional verification

```bash
cd /home/jmobile/websites/japa/japaconnect/apps/studio-desktop
bunx svelte-check --tsconfig ./tsconfig.json
# svelte-check found 0 errors and 0 warnings

bun run test
# 60 pass / 0 fail (7 test files)

bun run build
# Using @sveltejs/adapter-static — Wrote site to "build"

bunx playwright test
# 5 e2e route smoke tests passed

cd /home/jmobile/websites/japa/japaconnect
CARGO_BUILD_JOBS=2 PKG_CONFIG_PATH=/usr/lib/x86_64-linux-gnu/pkgconfig:/usr/share/pkgconfig CARGO_TARGET_DIR=/run/media/jmobile/Japa/studio-target cargo check
# studio-desktop: `cargo check` finished successfully
```

### 6.5 API connection and binary verification

```bash
# Backend preflight now exposes X-API-Key and X-Device-Id
curl -i -X OPTIONS -H "Origin: tauri://localhost" -H "Access-Control-Request-Method: POST" -H "Access-Control-Request-Headers: X-API-Key, X-Device-Id" http://localhost:8080/api/v1/auth/exchange-key
# Access-Control-Allow-Headers: Content-Type, Authorization, Idempotency-Key, X-Request-ID, X-CSRF-Token, X-API-Key, X-Device-Id

# Rebuilt the release Tauri binary with the latest Svelte frontend
cd /home/jmobile/websites/japa/japaconnect/apps/studio-desktop
CARGO_BUILD_JOBS=2 PKG_CONFIG_PATH=/usr/lib/x86_64-linux-gnu/pkgconfig:/usr/share/pkgconfig CARGO_TARGET_DIR=/run/media/jmobile/Japa/studio-target bunx tauri build --no-bundle
# Built application at: /run/media/jmobile/Japa/studio-target/release/studio-desktop

# Tauri WebDriver connection test (scripts/test-conn.ts)
bun scripts/test-conn.ts
# exchange-key result: status 401 "invalid_api_key" — request reached the handler

# Native Tauri smoke test
CARGO_TARGET_DIR=/run/media/jmobile/Japa/studio-target bun run test:e2e:tauri
# [e2e:tauri] Smoke test passed.
```

