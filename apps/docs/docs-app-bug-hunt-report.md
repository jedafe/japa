# Docs App Bug Hunt Report

**Target:** `apps/docs` (JapaConnect Documentation site)
**Verification:** `bunx svelte-check` (0 errors), `bun run build` (success), `bun run test` (9 passed)

## Summary

This pass audited the entire Docs SvelteKit app against the backend API catalog (`/api/v1/docs/*`, `/me/api-keys`, `/docs/openapi.json`, etc.) and the project bug-hunting rules. Several real functional bugs were found and fixed in `apps/docs`. A few backend-gaps remain where the frontend cannot close the loop without a service/schema change; these are documented below.

## Backend/Frontend Route Map

| Backend route (Go) | Docs app consumption | Status |
|---|---|---|
| `GET /docs/{audience}` | `routes/docs/[audience]/+page.server.ts` | OK |
| `GET /docs/{audience}/{slug}` | `routes/docs/[audience]/[slug]/+page.server.ts` | OK |
| `GET /docs/nav` | `lib/server/nav.ts` (layout load) | OK |
| `GET /docs/search` | `routes/docs/search/+page.server.ts`, palette via `/api/docs/search` proxy | FIXED |
| `GET /docs/audience-gates` | `hooks.server.ts` | OK |
| `GET /docs/audiences` | now consumed in `hooks.server.ts` | FIXED |
| `POST /docs/{audience}/{slug}/feedback` | via `/api/docs/feedback` proxy | FIXED |
| `GET /docs/openapi.json` | `docs/openapi.json/+server.ts` and playground | OK |
| `GET /me/api-keys` | `routes/docs/api/keys/+page.server.ts` | FIXED types |
| `POST /me/api-keys` | `routes/docs/api/keys/+page.server.ts` | FIXED payload |
| `DELETE /me/api-keys/{id}` | `routes/docs/api/keys/+page.server.ts` | OK |
| `GET /docs/statuses` | not consumed (requires `docs.manage`) | expected admin UI |
| `GET /admin/docs/*` | not consumed by Docs app; intended for admin/control panels | expected missing |
| `POST /admin/docs/ai-generate` | not consumed by Docs app | expected missing |

## Fixed bugs

### 1. API key type drift: frontend used wrong field names (`app_name`, `environment`)

**Severity:** High — creates 400/validation errors on key creation and displays undefined values.

**Evidence:** `internal/publicapi` `apiKeyRow` and `CreateAPIKeyInput` use `Name` and `RateLimitTier`; the DB migration `000038_public_api.sql` defines `name text NOT NULL` and `rate_limit_tier text NOT NULL DEFAULT 'basic'`. The Docs app was sending `app_name` and `environment`, which the backend ignores.

**Fix:**
- `lib/types/index.ts`: `ApiKeySummary` now uses `name` and `rate_limit_tier`.
- `routes/docs/api/keys/+page.server.ts` create action sends `name` and `rate_limit_tier`.
- `routes/docs/api/keys/+page.svelte` form/table use the new fields with a tier select (`basic`, `standard`, `premium`, `unlimited`).
- `routes/docs/api/playground/+page.svelte` key selector uses `k.name`.
- CSS updated from `.env-badge` to `.tier-badge`.

### 2. Unconsumed canonical route: `GET /docs/audiences`

**Severity:** Medium — hardcoded `AUDIENCE_ORDER` in `lib/types` violates Rule 16 and can drift if the backend adds/reorders audiences.

**Fix:**
- `hooks.server.ts` now fetches `GET /docs/audiences` and stores it in `event.locals.audienceOrder`.
- `app.d.ts`, `routes/+layout.server.ts` expose `audienceOrder` in `PageData`.
- `routes/docs/+page.svelte`, `routes/docs/search/+page.svelte`, `lib/components/shell/DocNavTree.svelte` use `data.audienceOrder` for ordering/filtering.
- `routes/docs/search/+page.server.ts` validates the search audience against `event.locals.audienceOrder`.

### 3. Nav collapse persistence did not actually save anything

**Severity:** Medium — every page load reset `navCollapsed` to `false`.

**Evidence:** `routes/api/nav-collapse/+server.ts` was calling `PATCH /me/account` with `{ ui: { nav_collapsed } }`, but `UpdateAccountRequest` in `internal/users/types.go` only accepts `username`, `email`, `display_name`. The `ui` field is not stored anywhere.

**Fix:**
- `routes/api/nav-collapse/+server.ts` now sets a durable `jc_nav_collapsed` cookie instead.
- `hooks.server.ts` reads the cookie and sets `event.locals.navCollapsed`.
- `routes/+layout.server.ts` returns `navCollapsed` from the cookie.
- Removed the unused `SessionUser.ui` field from `lib/types/index.ts`.

### 4. Search palette bypassed session auth

**Severity:** High — the palette fetched `/api/v1/docs/search` directly from the browser. The `jc_token` cookie is HttpOnly, so the client cannot set the `Authorization: Bearer` header. Authed users would only search public audiences.

**Fix:**
- Added `routes/api/docs/search/+server.ts`, a same-origin SvelteKit proxy that forwards the request with the session token.
- `lib/components/docs/SearchPalette.svelte` now fetches `/api/docs/search?...`.

### 5. Helpful widget could not authenticate and persisted failed votes

**Severity:** High — same HttpOnly-token problem; `apiPost('/docs/.../feedback')` would 401 in production. Also, `localStorage` was updated before the request, leaving failed votes sticky.

**Fix:**
- Added `routes/api/docs/feedback/+server.ts` proxy.
- `lib/components/docs/HelpfulWidget.svelte` now posts to the proxy, reverts the optimistic UI on failure, and only persists the vote in `localStorage` after a successful response.
- Exported `getCookie` from `lib/api/client.ts` so the widget can read the `csrf_token` cookie.

### 6. Playground quick endpoints rendered non-HTTP OpenAPI keys

**Severity:** Low — OpenAPI path objects may contain `parameters`, `servers`, etc. These were rendered as clickable quick actions and produced invalid requests.

**Fix:** `routes/docs/api/playground/+page.svelte` now filters `Object.entries(methods)` to only recognized HTTP methods (`get`, `post`, `put`, `patch`, `delete`, `head`, `options`).

### 7. Sidebar locale switch used `window.location` instead of `goto()`

**Severity:** Low — violates the app’s instant-navigation convention.

**Fix:** `lib/components/shell/SidebarWidgetRenderer.svelte` now uses `goto()` after setting the `jc_locale` cookie.

### 8. Client `apiGet` omitted `credentials: 'include'`

**Severity:** Medium — `apiGet` (used by widgets fetching runtime config and public data) would not send cookies on cross-origin deployments.

**Fix:** `lib/api/client.ts` `apiGet` now includes `credentials: 'include'`.

## Remaining findings (not fixed in this pass)

| # | Finding | Severity | Notes |
|---|---|---|---|
| R1 | `GET /docs/statuses` has no Docs app admin UI | Low | Requires `docs.manage`; the admin surface lives in `apps/admin` / `apps/control`, not `apps/docs`. |
| R2 | `POST /admin/docs/ai-generate` has no Docs app UI | Low | Admin-only; intentional. |
| R3 | `SidebarWidgetRenderer` silently no-ops for many widget types | Low | Registry rejects non-docs widget types; no UI requested. |
| R4 | `/newsletter/subscribe` CSRF for guests — **verified working** (guest submit returns 200; backend issues a `csrf_token` cookie independently) | Resolved | Verified live with `curl` + Playwright. |
| R5 | OpenAPI download proxy at `docs/openapi.json` always fetches from `API_BASE` without `event.locals.token` | Low | The route is public (`/docs/openapi.json`), so no auth needed, but host forwarding is correct. |

---

# Second Pass — Live Browser Audit (Playwright, guest + authenticated)

**Scope:** `/`, `/docs`, `/docs/{audience}`, `/docs/{audience}/{slug}`, `/docs/api`, `/docs/api/{slug}`, `/docs/api/keys`, `/docs/api/playground`, `/docs/search`, locale-prefixed routes (`/fr/*`), invalid slugs, mobile viewport (375px), sidebar widgets (nav tree, audience switcher, TOC, helpful widget, ad slot, newsletter, CTA, social links, user dropdown, theme/language toggles).

**Method:** Playwright MCP browser (guest + `jc_token` authed session), `curl` route-status matrix, backend route/contract review, live DB seed inspection.

## Fixed bugs

### 9. Authenticated users got HTTP 500 on every page (critical)

**Severity:** Critical — the entire Docs app was unusable for signed-in users.

**Cause:** `hooks.server.ts` treated `GET /me/permissions` as a bare `string[]`, but the backend returns `{ data: { permissions: string[] } }` (unwrapped by `apiGet` to `{ permissions: [...] }`). `new Set(user.permissions)` received a non-iterable object → unhandled throw → 500.

**Fix:** `hooks.server.ts` normalizes both shapes (`Array.isArray(res) ? res : res?.permissions ?? []`). Verified: authed `/docs` now 200; `/docs/api/keys`, `/docs/api/playground` render with real key data.

### 10. Locale-prefixed routes 404'd (`/fr/docs`)

**Severity:** High — the language toggle produced 404s; SEO/i18n broken.

**Cause:** `hooks.server.ts` mutated `event.url.pathname`, which does not affect SvelteKit route matching — route matching requires the `reroute` hook.

**Fix:** new `src/hooks.ts` exports a universal `reroute` hook stripping enabled locale prefixes (`/{lang}/docs/...` → `/docs/...`) before matching. `hooks.server.ts` still records `event.locals.locale`, sets the cookie, and now 404s prefixes that aren't enabled languages (`/de/docs` → 404). Verified: `/fr/docs` 200 in French, `/fr/docs/users/test-documentation-page` 200, `/de/docs` 404.

### 11. Language toggle silently broken for anyone who visited a prefixed URL

**Severity:** High — `switchLanguage` writes `jc_locale` via `document.cookie`, but the server set that cookie **HttpOnly**. Browsers refuse JS overwrites of HttpOnly cookies, so the write was silently dropped and the UI stayed in the old locale. Additionally, switching back to the default language produces a same-URL `goto()` that never re-ran loads.

**Fix:** `jc_locale` is now set with `httpOnly: false` (it's a UI preference like `jc_theme`, not a credential); `switchLanguage` uses `goto(href, { invalidateAll: true })`. Verified round-trip: `/docs` (en) → click French → `/fr/docs` French → click English → `/docs` English.

### 12. Search page rendered a permanent loading skeleton

**Severity:** High — `/docs/search?q=...` SSR'd the skeleton and never showed results.

**Cause:** `+page.svelte` checked `navigating && ...`, but `navigating` imported from `$app/state` is **never null** — it's a getter-backed object whose props return `null` when idle (verified in `@sveltejs/kit` runtime source). So the skeleton branch was always taken and both results and the empty state were unreachable.

**Fix:** condition now checks `navigating.to && navigating.to.url.searchParams.get('q') !== data.q`. Verified: `/docs/search?q=debug` shows "Console Debug".

### 13. Invalid audience returned access-denied instead of 404

**Severity:** Medium — `/docs/foobar` showed "Access Denied" for a section that doesn't exist; leaks the audience-gating concept and misleads crawlers.

**Fix:** `[audience]/+page.server.ts` and `[audience]/[slug]/+page.server.ts` first check the slug against `event.locals.audienceOrder` (canonical list from `GET /docs/audiences`); unknown → 404, known-but-inaccessible → `/docs/access-denied`. Verified: `/docs/foobar` → 404, `/docs/admin` → access-denied.

### 14. Root-relative menu/CTA links 404'd on the docs origin

**Severity:** High — admin-authored footer/sidebar links (`/signup`, `/login`, `/privacy`, `/terms`, `/`) are main-app routes; on `docs.japaconnect.com` they 404. The seeded CTA widget "Try JapaConnect" → `/signup` was the most prominent offender.

**Fix:** `resolveMenuHref()` added to `lib/utils/safeHtml.ts` — absolute URLs and `/docs...` stay local; other root-relative paths get `PUBLIC_MAIN_ORIGIN` prefixed. Applied in `GuestFooter.svelte` and `SidebarWidgetRenderer.svelte` (CTA). Migration `000680_docs_footer_and_api_key_limit_fixes.up.sql` also repoints stale seed rows (`/docs/getting-started` → `/docs/users`, deletes `/docs/guides` — neither slug exists as an audience) and sets `api.keys.max_per_user` `is_public=true` so the keys page can read the real limit. Verified: all footer links + CTA resolve to `http://localhost:3000/...` in dev.

### 15. String-valued document bodies rendered a blank article

**Severity:** High — `doc_pages.content` is `jsonb` and may hold a plain string, markdown, HTML, or a JSON-encoded string. `renderDocContent` only handled object-shaped TipTap JSON; everything else produced an empty `<article>` (e.g. the live `test-documentation-page`).

**Fix:** `lib/tiptap/serialize.ts` now parses string content as JSON first (TipTap path), then renders via `marked` (markdown/plain text), escapes raw text as a fallback paragraph, and sanitizes with `sanitizeContentHtml`. `marked@14.1.4` added via `bun add`. Verified: `/docs/users/test-documentation-page` renders `Hello World` heading + paragraph.

### 16. Search excerpts showed escaped JSON (`""`)

**Severity:** Medium — `internal/search/indexer.go` `tiptapPlainText` only dispatched on `{`, `[`, `<`; a JSON-quoted string body fell through and was indexed verbatim.

**Fix:** added a `"` case — `json.Unmarshal` the string and recurse into `tiptapPlainText`. New regression test `internal/search/indexer_test.go` covers quoted strings, markdown, TipTap docs, HTML, and malformed input. **Deployment note:** the running `japa-api` binary predates this fix; the stale `""` excerpt in the live index clears after a backend restart + page re-save/reindex.

### 17. Sidebar `user_dropdown` duplicated the theme toggle

**Severity:** Medium — `UserDropdown.svelte` baked in its own theme toggle button, so authed users saw two identical toggles in the sidebar topbar next to the admin-managed `theme_toggle` widget (which cannot be removed via admin config while hardcoded).

**Fix:** internal toggle removed from `UserDropdown`; theme switching is now solely the admin-managed `theme_toggle` widget.

### 18. Mobile users lost account/theme/language access

**Severity:** Medium — the sidebar topbar is `display:none` below 768px, hiding the user dropdown and both toggles.

**Fix:** `AppShell.svelte` mobile topbar now renders the non-search topbar widgets (`theme_toggle`, `language_toggle`, …) plus `UserDropdown` (authed) / `GuestAuthButtons` (guest). Verified at 375px: all controls visible, no horizontal overflow, drawer works.

### 19. API keys page miscounted the limit and mis-parsed the setting

**Severity:** Medium — revoked keys counted toward `maxKeys` (backend only counts active), and `system_settings` may store the value as JSON string `"5"` which `Number(undefined)`-style reads missed.

**Fix:** `+page.svelte` derives `activeKeyCount` (excludes `revoked`); `+page.server.ts` parses with `Number(...)` accepting int or string. Verified: authed page shows `3/5` against the real public setting.

### 20. OpenAPI fallback listed non-HTTP keys as endpoints

**Severity:** Medium — iterating `spec.paths[path]` emitted `parameters`/`servers`/`summary` as fake endpoint rows.

**Fix:** explicit `HTTP_METHODS` allowlist in `docs/api/+page.server.ts` and `docs/api/[slug]/+page.server.ts`. Verified: `/docs/api` lists 91 real operations, no metadata rows.

### 21. Helpful widget: duplicate submissions + hidden guest error

**Severity:** Medium — rapid clicks fired parallel votes (backend aggregates — no dedupe), and guests saw generic "Feedback failed" instead of the proxy's "Sign in to leave feedback" (SvelteKit `error()` serializes as `{message}`, not `{error:{message}}`).

**Fix:** `submitting` in-flight guard + `disabled` buttons in `HelpfulWidget.svelte`; error reader accepts both envelopes. Verified: guest vote shows "Sign in to leave feedback"; authed vote persists (`aria-pressed`, localStorage).

### 22. Search palette scoped `/docs/api/*` to the wrong audience

**Severity:** Medium — the palette derived scope from `params.audience`, but `/docs/api` is a static route with no audience param, so API-page searches were unscoped.

**Fix:** `SearchPalette.svelte` derives `'api'` when the pathname starts with `/docs/api`.

### 23. Locale-prefixed active-nav highlighting

**Severity:** Low — nav links point at `/docs/...` while the browser URL is `/fr/docs/...`, so the current page never highlighted under a locale prefix.

**Fix:** `DocNavTree.svelte` strips the enabled-locale segment from `page.url.pathname` before comparing.

### 24. Broken sitemap link + relative favicons

**Severity:** Low — `<link rel="sitemap" href="/docs-sitemap.xml">` 404'd (backend serves `/sitemap*.xml` only); `%sveltekit.assets%` rendered `./favicon.png`, resolving to `/fr/favicon.png` on prefixed routes → 404s in console.

**Fix:** removed the bogus sitemap link from `+layout.svelte`; `app.html` uses absolute `/favicon*.png` paths. Verified: no favicon 404s on `/fr/docs`.

### 25. Sidebar resolution coupling + root error page

**Severity:** Low — the two `/sidebars/resolve` calls were coupled; and non-`/docs` 404s rendered SvelteKit's default error page (no shell).

**Fix:** `+layout.server.ts` uses `Promise.allSettled` per position; new `routes/+error.svelte` renders 404/500 inside `AppShell`. Verified: `/nonexistent` → styled 404.

## Verification (pass 2)

| Check | Result |
|---|---|
| Guest `/docs`, `/docs/users`, doc pages, `/docs/api`, `/docs/search` | 200, clean render, 0 console errors |
| Authed `/docs` | 200 (was 500) — user dropdown, audience gating live |
| `/fr/docs`, `/fr/docs/users/...` | 200, French UI; `/de/docs` → 404 |
| `/docs/foobar` | 404 (was access-denied); `/docs/admin` → access-denied |
| Language toggle round-trip en→fr→en | works (cookie + prefix) |
| Search page `?q=debug` | result renders (was perpetual skeleton) |
| Doc with markdown content | renders (was blank) |
| Helpful vote authed / guest | persists / "Sign in to leave feedback" |
| Footer links + CTA | resolve to `PUBLIC_MAIN_ORIGIN` |
| `/docs/api` endpoint list | 91 real HTTP ops, no `parameters` leaks |
| `/docs/api/keys` authed | real keys listed, `3/5` active count |
| Mobile 375px | drawer + topbar widgets + account reachable, no overflow |
| `bun run check` / `bun run test` / `bun run build` | 0 errors / 9 pass / success |
| `go test ./internal/search/` `go vet` `gofmt` | pass / clean / clean |

**Note:** `apps/main` workspace changes in this tree are unrelated pre-existing work and were not touched.

## Files changed (pass 2)

```
apps/docs/src/hooks.ts                                    (new — reroute)
apps/docs/src/routes/+error.svelte                        (new)
apps/docs/src/hooks.server.ts
apps/docs/src/app.html
apps/docs/src/lib/tiptap/serialize.ts
apps/docs/src/lib/utils/safeHtml.ts
apps/docs/src/lib/components/shell/AppShell.svelte
apps/docs/src/lib/components/shell/UserDropdown.svelte
apps/docs/src/lib/components/shell/GuestFooter.svelte
apps/docs/src/lib/components/shell/SidebarWidgetRenderer.svelte
apps/docs/src/lib/components/shell/DocNavTree.svelte
apps/docs/src/lib/components/docs/HelpfulWidget.svelte
apps/docs/src/lib/components/docs/SearchPalette.svelte
apps/docs/src/lib/i18n/en.json, fr.json
apps/docs/src/routes/+layout.svelte, +layout.server.ts
apps/docs/src/routes/docs/[audience]/+page.server.ts
apps/docs/src/routes/docs/[audience]/[slug]/+page.server.ts
apps/docs/src/routes/docs/api/+page.server.ts
apps/docs/src/routes/docs/api/[slug]/+page.server.ts
apps/docs/src/routes/docs/api/keys/+page.server.ts, +page.svelte
apps/docs/src/routes/docs/search/+page.server.ts, +page.svelte
internal/search/indexer.go
internal/search/indexer_test.go                           (new)
migrations/000680_docs_footer_and_api_key_limit_fixes.{up,down}.sql (new)
```

## Files changed

```
apps/docs/src/app.d.ts
apps/docs/src/hooks.server.ts
apps/docs/src/lib/api/client.ts
apps/docs/src/lib/components/docs/HelpfulWidget.svelte
apps/docs/src/lib/components/docs/SearchPalette.svelte
apps/docs/src/lib/components/shell/DocNavTree.svelte
apps/docs/src/lib/components/shell/SidebarWidgetRenderer.svelte
apps/docs/src/lib/i18n/en.json
apps/docs/src/lib/i18n/fr.json
apps/docs/src/lib/types/index.ts
apps/docs/src/routes/+layout.server.ts
apps/docs/src/routes/api/nav-collapse/+server.ts
apps/docs/src/routes/api/docs/search/+server.ts   (new)
apps/docs/src/routes/api/docs/feedback/+server.ts (new)
apps/docs/src/routes/docs/+page.svelte
apps/docs/src/routes/docs/[audience]/+page.server.ts
apps/docs/src/routes/docs/api/keys/+page.server.ts
apps/docs/src/routes/docs/api/keys/+page.svelte
apps/docs/src/routes/docs/api/playground/+page.svelte
apps/docs/src/routes/docs/search/+page.server.ts
apps/docs/src/routes/docs/search/+page.svelte
apps/docs/docs-app-bug-hunt-report.md
```

## Verification

```bash
cd apps/docs
bunx svelte-kit sync
bunx svelte-check --tsconfig ./tsconfig.json
# 0 errors, 0 warnings

bun run build
# success (adapter-node)

bun run test
# 9 passed
```

Playwright is not configured for `apps/docs`; existing unit and build checks passed. Manual browser checks of the search palette, feedback, and API key flows are recommended once the dev server is running against a seeded backend.
