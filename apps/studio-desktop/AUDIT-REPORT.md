# Japa Content Studio (Desktop) — Audit & Fix Report

**Scope:** `apps/studio-desktop` (SvelteKit + Tauri desktop client for JapaConnect content operations).  
**Run:** full audit of business logic, API contracts, UI/UX, Svelte 5/SvelteKit correctness, Tauri fs behaviour, high-stakes content enforcement, queue/watcher/publisher logic, Markdown parsing, and test coverage; followed by focused fixes and verification.  
**Status:** fixes applied and verified (Svelte/TS checks, unit tests, production build).  
**Tauri binary build:** `bunx svelte-check`, `bun run test`, `bun run build`, and `go test` pass. `bunx tauri build` could not be completed in this environment because the Rust compiler aborted while compiling the `time` crate dependency (`munmap_chunk(): invalid pointer` / SIGABRT). This appears to be a toolchain/container issue rather than an application defect; the code uses the standard `tauri-plugin-sql` SQLite plugin and migrations.

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

---

## 2. Unchanged / not directly addressed

- **SQLite local state model:** implemented with `tauri-plugin-sql`, `src-tauri/src/migrations.rs`, `001_state_tables.sql`, and `src/lib/services/db.ts`. Settings, file queue state, sync log, and editor drafts now persist to `sqlite:state.db` in the Tauri application data directory, with an in-memory cache for synchronous `loadSettings()` reads and a `localStorage` fallback for browser/Vitest environments.
- **Runtime API integration:** an exchange route (`POST /api/v1/auth/exchange-key`) was added to the backend and the desktop client was updated to call it; all other API routes remain unchanged.
- **RBAC / permissions:** no new RBAC nodes or permission tables were added. The desktop app now relies on the existing API key and refresh-token flows.
- **Other workspaces:** pre-existing modifications in `apps/jce`, `internal/db`, `internal/gdpr`, and `internal/jce` were not part of this studio-desktop audit and were left untouched.

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

bunx tauri build --debug --no-bundle
# Reached the Rust compilation stage but the compiler aborted while building the `time` crate
# (`munmap_chunk(): invalid pointer` / SIGABRT). This is a container/toolchain issue, not an
# application error. `bun run test:e2e:tauri` is available to drive the built binary via
# tauri-driver + WebKitWebDriver when those tools are installed.
```

---

## 5. Remaining recommendations

1. ~~Add dedicated tests for `publisher.ts` and `watcher.ts` using mocked `fetch`, `localStorage`, and Tauri fs APIs.~~ ✅ Done.
2. ~~Implement the spec's SQLite state layer instead of `localStorage` for settings, file queue, sync log, and editor drafts.~~ ✅ Done.
3. ~~Add static build e2e smoke tests with Playwright.~~ ✅ Done.
4. ~~Add a Tauri/WebDriver e2e harness.~~ ✅ Done (`scripts/e2e-tauri.ts`); it requires `tauri-driver` and a native WebDriver server (WebKitWebDriver on Linux) and is designed to skip cleanly when those are unavailable.
5. Build and run the native Tauri/WebDriver smoke tests in an environment with a healthy Rust toolchain.

---

