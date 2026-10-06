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

---

# Third Pass — Search/Filters/Pagination/Sort/Actions Audit (Playwright, guest + authenticated)

**Scope:** `/docs` landing, `/docs/search` (query, audience filter, page-size), `/docs/api` (endpoint search, method filter, tag grouping), `/docs/api/keys` (create/rename/rotate/revoke/IP-allowlist), `/docs/api/playground` (prefill, live send), `HelpfulWidget`, language toggle, `AudienceSwitcher`, and every dropdown field (Autocomplete consistency).

**Method:** Playwright MCP browser (guest + `jc_token` authed session), live API smoke tests (`curl`), Meilisearch reindex + excerpt verification, `svelte-check` on all touched apps.

## Fixed bugs

### 26. Search page-size control missing; absent `limit` defaulted to `5` instead of `20`

**Severity:** Medium — no page-size UI existed, and `Number(null) === 0` meant a param-less `?q=test` URL produced `limit=5` (clamped minimum) rather than the intended `20` default.

**Fix:**
- `routes/docs/search/+page.server.ts` checks `searchParams.has('limit')` before converting; absent → `20`, integer → clamp `[5, 50]`, non-integer → `20`.
- `routes/docs/search/+page.svelte` adds a page-size `AutocompleteSelect` (10/20/50) that navigates with `q`, `audience`, `limit` preserved.
- Verified: `?q=test` shows 20/page; `?q=test&limit=50` and `?q=test&audience=users&limit=50` compose correctly.

### 27. API landing page had no endpoint search or method filter

**Severity:** Medium — 90+ operations rendered as one undifferentiated list; no way to locate an endpoint.

**Fix:** `routes/docs/api/+page.svelte` adds a text filter (path/summary/description), a method `AutocompleteSelect` (driven by the methods actually present in the spec — GET/POST/PUT/DELETE; PATCH absent because the spec has zero PATCH ops), tag grouping, and per-endpoint "Try it" links into `/docs/api/playground?method=…&path=…`.

### 28. Playground sent requests to un-prefixed paths

**Severity:** High — OpenAPI paths like `/public/scholarships` were requested verbatim against the docs origin, missing `/api/v1` — quick-endpoint links produced 404s.

**Fix:** `routes/docs/api/playground/+page.svelte` `apiPath()` normalizes spec paths to `/api/v1/...` (relative → prefixed, absolute `/x` → `/api/v1/x`, already-prefixed unchanged). Verified live: prefill `/public/scholarships` → request to `/api/v1/public/scholarships`; authenticated send to `/api/v1/me` returned 200 with real data.

### 29. API keys page: hardcoded scope catalog exposed admin-only scopes; missing key-management actions

**Severity:** High — the create form offered `mcp`, `api.write.content`, etc., which self-service `POST /me/api-keys` rejects; and the canonical self-service catalog `GET /developer/scopes` was unconsumed. Rename (`PATCH`), rotate (`POST .../rotate`), and IP allowlist (`GET/POST/DELETE .../ip-allowlist`) routes existed in the backend with no UI.

**Fix:**
- `routes/docs/api/keys/+page.server.ts` fetches `GET /developer/scopes` (canonical catalog) and per-key allowlists; adds `rename`, `rotate`, `addIp`, `removeIp` form actions; parses the `api.keys.max_per_user` setting.
- `routes/docs/api/keys/+page.svelte` — scope picker via `Autocomplete` multi-select restricted to `data.scopes` (admin-granted scopes still render as read-only chips on existing keys); rename/rotate dialogs; IP allowlist dialog with add/remove; status badges (`active`/`suspended`/`revoked`); one-time create/rotate secret banners preserved.
- Verified live: create (secret shown once), rename, rotate (grace-period copy), revoke, IP allowlist add/remove — all round-trip against the backend.

### 30. Keys page `use:enhance` callbacks suppressed `update()` — form results never landed

**Severity:** High — six forms returned `({ update }) => async () => { ...custom... }` without calling `update()`, so `form` never populated and `invalidateAll` never ran. Created keys appeared to vanish; the dialog stayed open. (Found when the same click submitted twice and both rows hit the backend.)

**Fix:** every enhance callback now calls `await update()` before custom handling. Verified: create → secret banner + row; revoke → status flip; rename → row update — all in one submission.

### 31. `HelpfulWidget` let guests vote and implied toggle/switch semantics the backend doesn't have

**Severity:** Medium — `POST /docs/{audience}/{slug}/feedback` is increment-only aggregate (no un-vote, no switch) and requires auth; guests got an actionable widget that could only 401.

**Fix:** `HelpfulWidget.svelte` shows a sign-in link (with `redirect`) to guests; authed users get buttons that lock after a successful vote. Verified both states live.

### 32. Dropdown fields not using `Autocomplete`

**Severity:** Low — `AudienceSwitcher` (custom listbox) and the `language_toggle` widget (custom dropdown) bypassed the shared component.

**Fix:** both migrated to `AutocompleteSelect` (which wraps `Autocomplete.svelte`). Audit confirmed zero raw `<select>` elements remain; `UserDropdown` (a nav menu, not a field) is unchanged. All field-type dropdowns (audience, language, page-size, method filter, key tier, key scopes, playground method, playground key) now go through the shared component.

### 33. `jc_locale` language switch broken cross-app — sibling apps set it HttpOnly

**Severity:** High — `apps/main`, `apps/academy`, `apps/tools` `hooks.server.ts` set `jc_locale` via `event.cookies.set` without `httpOnly: false` (SvelteKit defaults HttpOnly). Cookies ignore ports, so visiting `/fr/...` on `localhost:3000` planted an HttpOnly `jc_locale=fr` that the docs app could neither read (`document.cookie`) nor overwrite — the switcher silently no-oped and the UI stayed French (`cookieStore.set` threw "Cookie was malformed").

**Fix:** `httpOnly: false` added to the `jc_locale` set in `apps/main/src/hooks.server.ts`, `apps/academy/src/hooks.server.ts`, `apps/tools/src/hooks.server.ts` (docs already fixed in pass 2). Verified round-trip in one session sharing the jar: `/fr/docs/users` → English → `/docs/users` (en) → French → `/fr/docs/users`.

### 34. cURL examples used `Authorization: Bearer` for API-key auth

**Severity:** Medium — API-key auth uses `X-API-Key`; docs examples drifted from the contract.

**Fix:** `EndpointCard.svelte` and `docs/api/[slug]/+page.svelte` samples now emit `X-API-Key: <key>`. Playground was already correct.

### 35. Landing page miscounted nested pages; API card showed "0 pages"

**Severity:** Low — the users audience has nested nav children; the counter only read top-level entries, and the API card (spec-driven, not doc pages) displayed a misleading `0 pages`.

**Fix:** `routes/docs/+page.svelte` counts recursively and hides the count for the `api` audience. Verified: `3 pages` on Utilisateurs, no label on API.

### 36. Stale Meilisearch excerpt + misc gaps

**Severity:** Medium — `doc_pages` index carried a pre-fix excerpt (`""`-style escaped JSON).

**Fix:** `source .env && go run ./cmd/reindex/` — 4 doc pages reindexed, 0 errors. Verified: `/api/v1/docs/search?q=test` returns `"# Hello World This is a test page."`.

**i18n:** ~25 missing keys added to `en.json`/`fr.json` (`api.*` filter/action strings, `common.no_results`/`optional`/`remove`, `landing.pages`, `page.feedback_sign_in`/`feedback_recorded`, `search.per_page`).

## Verified-accurate findings (no bug)

| # | Observation | Verdict |
|---|---|---|
| V1 | Method filter lacks PATCH | Spec has 0 PATCH ops (get:128, post:10, put:1, delete:2) — filter is correct |
| V2 | `GET /api/v1/docs/openapi.json` → `invalid_audience` | Spec is deliberately mounted at `/docs/openapi.json` outside `/api/v1` (§6.4 dedupe) — frontend fetch is correct |
| V3 | Playground key selector showed two same-named keys | One was the pre-rotation secret inside its 24h grace window — correct |
| V4 | `DELETE /me/api-keys/{id}` returns 200 but key remains listed | Soft-delete → `status=revoked` — matches contract |

## Verification (pass 3)

| Check | Result |
|---|---|
| `svelte-check` apps/docs, apps/main, apps/academy, apps/tools | 0 errors, 0 warnings (all four) |
| Search `?q=test` default page size | 20 (was 5); 10/20/50 selectable; `limit` in URL |
| Search audience + limit compose | `?q=test&audience=users&limit=50` correct |
| API landing filter/method/tag grouping | live; method list matches spec exactly |
| Playground prefill + authed send | `/public/scholarships`→`/api/v1/...`; `/api/v1/me` → 200 real data |
| Keys: create/rename/rotate/revoke/IP-allowlist | all round-trip; one-time secrets shown once; admin scopes not selectable |
| HelpfulWidget guest/authed | sign-in link / vote + lock |
| Language toggle en→fr→en | works end-to-end (cookie + prefix + cookie back) |
| AudienceSwitcher | navigates correctly |
| Landing page counts | `3 pages` users; no `0 pages` on API |
| Console errors/warnings | 0 / 0 |
| Test API keys created during audit | revoked (cleanup) |

## Files changed (pass 3)

```
apps/docs/src/lib/components/docs/EndpointCard.svelte
apps/docs/src/lib/components/docs/HelpfulWidget.svelte
apps/docs/src/lib/components/shell/AudienceSwitcher.svelte
apps/docs/src/lib/components/shell/SidebarWidgetRenderer.svelte
apps/docs/src/lib/i18n/en.json, fr.json
apps/docs/src/lib/types/index.ts
apps/docs/src/routes/docs/+page.svelte
apps/docs/src/routes/docs/api/+page.server.ts, +page.svelte
apps/docs/src/routes/docs/api/[slug]/+page.svelte
apps/docs/src/routes/docs/api/keys/+page.server.ts, +page.svelte
apps/docs/src/routes/docs/api/playground/+page.server.ts, +page.svelte
apps/docs/src/routes/docs/search/+page.server.ts, +page.svelte
apps/main/src/hooks.server.ts    (jc_locale httpOnly:false)
apps/academy/src/hooks.server.ts   (jc_locale httpOnly:false)
apps/tools/src/hooks.server.ts   (jc_locale httpOnly:false)
```

**Ops:** `go run ./cmd/reindex/` (doc_pages reindexed, excerpt corrected).

---

# Fourth Pass — Contract Drift + Consistency Sweep

**Scope:** dead contract fields, cross-component consistency, locale-prefixed scoping, i18n coverage audit, DOM-level verification.

## Fixed bugs

### 37. `breadcrumb` / `prev` / `next` were dead fields — backend never returns them

**Severity:** Medium — `DocPage` in `internal/docs/types.go` has no `breadcrumb`, `prev`, or `next` fields, so the breadcrumb nav and prev/next cards (§44 ROADMAP 02.03/02.05) could never render.

**Fix:** `docs/[audience]/[slug]/+page.svelte` derives them from the nav tree (`GET /docs/nav` — already loaded by the layout): breadcrumb walks `parent_id`; prev/next flatten the audience tree in display order. Server fields still win if the backend ever sends them. Verified live: `/docs/users/new-title` renders "Précédent → Console Debug" and "Suivant → Test Documentation Page".

### 38. `last_reviewed_by` rendered a raw UUID

**Severity:** Low — the field is a `pgtype.UUID` (reviewer user id), not a display name; the widget appended `· <uuid>` after the review date.

**Fix:** `HelpfulWidget.svelte` no longer renders it (prop retained for API-shape parity, documented as internal id).

### 39. Nav tree expanded every audience section (contradicted documented behavior)

**Severity:** Low — `DocNavTree` used `expanded[tree.audience] ?? true` while the component documents "active audience expanded, others collapsed".

**Fix:** default flipped to `?? false` — active audience expands via the existing `$effect`, others start collapsed. Verified: on `/docs/users/*` the users section lists its pages, API section collapsed.

### 40. `DocContent` doubled article padding

**Severity:** Low — `.doc-content` applied the same `2.5rem 1.5rem 4rem` padding + max-width inside the already-padded `.doc-article` column → ~5rem vertical inset and a narrower content column than the header.

**Fix:** `.doc-content` reduced to a plain wrapper; the `.doc-article` column owns layout.

### 41. Search palette lost API scoping on locale-prefixed URLs

**Severity:** Medium — `page.url.pathname.startsWith('/docs/api')` fails on `/fr/docs/api/...`, so palette searches on localized API pages were unscoped.

**Fix:** `SearchPalette.svelte` strips the enabled-locale prefix before the `/docs/api` check (same approach as `DocNavTree`). Verified live: on `/fr/docs/api`, palette fires `/api/docs/search?q=test&limit=10&audience=api`.

### 42. Missing i18n keys for Autocomplete internals

**Severity:** Low — `common.clear` / `common.loading` (used by `Autocomplete.svelte`) had no dictionary entries; French UI fell back to English strings.

**Fix:** added to `en.json` (`Clear` / `Loading…`) and `fr.json` (`Effacer` / `Chargement…`).

## Verified clean (no bug)

- `+error.svelte` Tailwind-style vars (`--foreground`, `--destructive`, `--muted-foreground`) all exist in `app.css`.
- `MethodBadge` classes map to `--method-*` vars for all five methods.
- `access-denied` route, `/de/docs` → 404, feedback/search proxies, nav-collapse cookie endpoint — all verified.
- Zero `console.log`/`debugger`/TODO leftovers; zero missing i18n keys after additions; `svelte-check` 0 errors/0 warnings.

## Files changed (pass 4)

```
apps/docs/src/routes/docs/[audience]/[slug]/+page.svelte  (breadcrumb/prev/next from nav tree)
apps/docs/src/lib/components/docs/HelpfulWidget.svelte    (drop UUID render)
apps/docs/src/lib/components/docs/DocContent.svelte       (dedupe padding)
apps/docs/src/lib/components/docs/SearchPalette.svelte    (locale-aware api scoping)
apps/docs/src/lib/components/shell/DocNavTree.svelte      (collapse non-active sections)
apps/docs/src/lib/i18n/en.json, fr.json                   (common.clear, common.loading)
```
