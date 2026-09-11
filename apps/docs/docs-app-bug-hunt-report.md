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
| R4 | `/newsletter/subscribe` CSRF for guests may fail if no `csrf_token` cookie | Medium | The docs hook only fetches `/csrf` for authenticated users; guest newsletter submits need either a guest CSRF cookie or an exempt public route. Not a Docs-app-only fix. |
| R5 | OpenAPI download proxy at `docs/openapi.json` always fetches from `API_BASE` without `event.locals.token` | Low | The route is public (`/docs/openapi.json`), so no auth needed, but tenant/host handling is correct. |

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
