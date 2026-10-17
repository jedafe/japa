---
name: jc-tools-debug
description: Use when debugging issues in the JapaConnect Tools Web Surface (apps/tools, port 3005, tools.japa.to). Covers host-detection failures, shell rendering issues, API errors, SSR/CSR boundary leaks, and LayerChart problems.
---

# JapaConnect Tools Surface — Debug Skill

Debugging guide for `apps/tools/` (§78, ADR-086). Start here when something is wrong with the tools surface.

## Surface Overview

SvelteKit SSR · Port 3005 · `tools.japa.to`. Sixth SvelteKit app. Same Go backend, same `/api/v1`, no community navigation.

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

**Symptom:** User logged in on `japa.to` but not on `tools.japa.to`.

**Debug path:**
1. Check cookie `domain=.japa.to` (covers all subdomains)
2. Verify the Vite proxy passes the cookie to the API as a Bearer header
3. For custom domains (not `.japa.to`), cookie auth doesn't work — OAuth flow required (§78)

### 11. V2 Functional Tool — Missing `?dest=` Filter (Geo-Aware Pattern)

**Symptom:** V2 functional tool page shows global/unfiltered data instead of destination-specific content.

**Debug path:**
1. Check the API call in `+page.server.ts` or `+page.svelte` — does it include `?dest=` query param?
2. Verify the destination selector is rendered and its value is bound to the API call
3. Check that the destination selector is the first UI input (geo-aware pattern requires it before tool-specific content)
4. Verify the API endpoint accepts and filters by `?dest=` param (check `api-routes-catalog.md`)

**Common causes:**
- Destination selector not yet implemented or not wired to API calls
- API call hardcoded without `?dest=` param
- Selector value not reactive (not using `$state` or `$derived`)

### 12. V2 Functional Tool — Hardcoded Nationality

**Symptom:** Tool page assumes a specific nationality (e.g. "Nigerian passport" hardcoded) instead of using destination country as primary axis.

**Debug path:**
1. Search the tool page source for hardcoded country names, ISO codes, or nationality references
2. The geo-aware pattern requires destination country as primary data axis — nationality is a secondary filter, never hardcoded
3. Replace hardcoded nationality with destination selector-driven API calls

### 13. V2 Functional Tool — Missing Destination Selector

**Symptom:** V2 tool page renders tool-specific content immediately without a country selector.

**Debug path:**
1. Every v2 functional tool page must render a destination country selector as the first UI input
2. The selector drives all downstream API calls via reactive `?dest=` param
3. If the selector is missing, add it before any tool-specific content section

### 14. Feature-Flag Returning 403 Instead of 404 (ADR-042)

**Symptom:** Disabled v2 functional tool endpoint returns HTTP 403 instead of 404.

**Debug path:**
1. Check the backend feature-flag middleware — ADR-042 requires 404 (not 403) for disabled features
2. 403 leaks the existence of the endpoint; 404 does not
3. The Tools app should handle 404 gracefully — hide the tool card from the hub grid, show "coming soon" state
4. If the backend returns 403, file a backend bug (ADR-042 violation)

### 15. Missing ADR-088 AI Generation Columns

**Symptom:** V2 tool content API response missing `ai_generated`, `ai_model`, `hitl_reviewed_by`, or `ai_generated_at` fields.

**Debug path:**
1. Check the backend migration — v2 tool content tables must carry these standard columns (ADR-088)
2. `is_japabot_indexed` computed column should also be present for RAG-retrievable content
3. If columns are missing in the DB, file a backend bug (ADR-088 violation)
4. If columns exist but API doesn't return them, check the API response struct

### 16. HITL Review Bypassed — Unreviewed Content Visible

**Symptom:** AI-generated tool content visible to end users without human review.

**Debug path:**
1. Check the API response — does it include content with `hitl_reviewed_by IS NULL`?
2. The backend should filter to only `hitl_reviewed_by IS NOT NULL` for public endpoints
3. If the backend returns unreviewed content, file a backend bug (ADR-088 HITL gate violation)
4. The frontend should not attempt to display unreviewed content even if received

### 17. Tool-Domain Host Detection Failure for New V2 Tools

**Symptom:** New v2 functional tool domain (e.g. `visaroute.japa.to`) shows hub shell instead of single-tool shell.

**Debug path:**
1. Check `system_settings` — are the `{slug}.subdomain` and `{slug}.custom_domain` keys seeded for the new tool?
2. Verify `hooks.server.ts` recognizes the new tool slug in the host→surface map
3. Check that `GET /site/settings/public` returns the new tool-domain keys
4. Verify the tool slug in `src/lib/tools/registry.ts` matches the settings key prefix
5. Check host comparison logic — normalize to lowercase, custom domains take precedence

## Debugging Tools

- **`svelte-kit dev`** with `VITE_DEBUG=1` for verbose logging
- **Browser DevTools Network tab** — check API responses and CSRF headers
- **`FT.PROFILE`** (Redis Search) — for search-related tool queries
- **Server logs** — `hooks.server.ts` logs host detection; API client logs errors

## See Also

ADR-086, §78, `ROADMAP-TOOLS.md`, ADR-088 (AI content generation), ADR-042 (feature-flag 404), `.jeos/skills/geo-aware-tool-pattern.md`, §§90–97 §§99–101 (V2 functional tools), `jc-tools-develop`, `jc-tools-audit`, `jc-tools-refactor`.
