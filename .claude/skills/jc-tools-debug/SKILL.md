---
name: jc-tools-debug
description: Use when debugging issues in the JapaConnect Tools Web Surface (apps/tools, port 3005, tools.japaconnect.com). Covers host-detection failures, shell rendering issues, API errors, SSR/CSR boundary leaks, and LayerChart problems.
---

# JapaConnect Tools Surface — Debug Skill

Debugging guide for `apps/tools/` (§78, ADR-086). Start here when something is wrong with the tools surface.

## Surface Overview

SvelteKit SSR · Port 3005 · `tools.japaconnect.com`. Sixth SvelteKit app. Same Go backend, same `/api/v1`, no community navigation.

## Common Issues & Fixes

### 1. Host Detection Fails (wrong shell renders)

**Symptom:** Single-tool domain shows hub shell, or hub domain shows single-tool shell.

**Debug path:**
1. Check `event.locals.toolSurface` in `hooks.server.ts` — add temporary `console.log('host:', host, 'surface:', toolSurface)`
2. Verify `system_settings` has the tool's domain keys configured (`{tool}.subdomain`, `{tool}.custom_domain`)
3. Check that the public settings fetch (`GET /site/settings/public`) returns the domain keys
4. Verify host comparison logic — custom domains take precedence over subdomains
5. Check `JC_TOOLS_HUB_HOSTS` env var if the hub isn't being detected

**Common causes:**
- `system_settings` keys not seeded (backend phase incomplete)
- Host comparison case-sensitive (normalize to lowercase)
- Custom domain not yet pointed at the server (DNS)
- Settings cached at module level instead of per-request

### 2. Build Fails — Missing Env Exports

**Symptom:** `"PUBLIC_JC_UMAMI_URL" is not exported by "\0virtual:env/static/public"`

**Fix:** Use `$env/dynamic/public` for optional env vars, not `$env/static/public`:
```typescript
// ❌ Wrong — build fails if var not set
import { PUBLIC_JC_UMAMI_URL } from '$env/static/public';

// ✅ Correct — optional, runtime-checked
import { env } from '$env/dynamic/public';
const umamiEnabled = !!env.PUBLIC_JC_UMAMI_URL;
```

### 3. Build Fails — Wrong Lucide Package

**Symptom:** `No version matching "^0.460.0" found for specifier "@lucide/svelte"`

**Fix:** The monorepo uses `lucide-svelte@^1.0.1` (not `@lucide/svelte`). Check `apps/*/package.json` for the canonical version.

### 4. Build Fails — LayerChart Missing Export

**Symptom:** `"Gridset" is not exported by "node_modules/layerchart/dist/index.js"`

**Fix:** `Gridset` is not exported in LayerChart 2.0.1. Use the compound `LineChart` component instead of composing `Chart` + `Svg` + `Axis` + `Gridset` + `Line`:
```svelte
// ❌ Wrong — Gridset not exported
import { Chart, Svg, Line, Axis, Gridset } from 'layerchart';

// ✅ Correct — use LineChart compound
import { LineChart } from 'layerchart';
<LineChart {data} x="timestamp" y="rate" color="provider" />
```

### 5. Svelte Parse Error — Each Block Missing Paren

**Symptom:** `Expected token )` in `{#each ... as item, i (i}`

**Fix:** The keyed each block needs a closing paren: `{#each items as item, i (i)}`

### 6. API Errors — 404 on Tool Endpoints

**Symptom:** Tool page shows empty state or error; API returns 404.

**Debug path:**
1. Check `api-routes-catalog.md` — is the route marked as live or "V2 planned"?
2. If "V2 planned" — the backend phase isn't complete yet; do not mock (Rule 16)
3. If live — check the API base URL (`JC_API_BASE` env var, or `/api/v1` via Vite proxy)
4. Check CSRF token — mutations without a valid CSRF token return 403
5. Check auth — user-specific endpoints return 401 without a valid session

### 7. SSR/CSR Boundary Leak

**Symptom:** User A's progress appears in User B's cached SSR response.

**Debug path:**
1. Check `+page.server.ts` — does it fetch user-specific data unconditionally?
2. User data must only be fetched SSR when `event.locals.user` is present
3. Mutations must be client-side via `$lib/api/client` (not in load functions)
4. Check that the Vite proxy passes the `jc_token` cookie as a Bearer header

### 8. Registration Prompt Not Appearing

**Symptom:** Anonymous user clicks "save" and nothing happens.

**Debug path:**
1. Check that the mutation handler checks `user` before calling the API
2. If `!user`, set `showRegisterPrompt = true`
3. Verify the modal renders and links to `PUBLIC_MAIN_ORIGIN/signup`
4. Check that the modal's backdrop click closes it

### 9. Chart Not Rendering

**Symptom:** Remittance trend chart area is blank.

**Debug path:**
1. Check that `history` state is populated (API call to `/remittance/history`)
2. Verify data format — `timestamp` must be a `Date` object, not a string
3. Check container has a fixed height (LayerChart needs explicit dimensions)
4. Verify `loadingHistory` is false before rendering

### 10. Cookie Auth Not Working Across Subdomains

**Symptom:** User logged in on `japaconnect.com` but not on `tools.japaconnect.com`.

**Debug path:**
1. Check cookie `domain=.japaconnect.com` (covers all subdomains)
2. Verify the Vite proxy passes the cookie to the API as a Bearer header
3. For custom domains (not `.japaconnect.com`), cookie auth doesn't work — OAuth flow required (§78)

## Debugging Tools

- **`svelte-kit dev`** with `VITE_DEBUG=1` for verbose logging
- **Browser DevTools Network tab** — check API responses and CSRF headers
- **`FT.PROFILE`** (Redis Search) — for search-related tool queries
- **Server logs** — `hooks.server.ts` logs host detection; API client logs errors

## See Also

ADR-086, §78, `ROADMAP-TOOLS.md`, `jc-tools-develop`, `jc-tools-audit`, `jc-tools-refactor`.
