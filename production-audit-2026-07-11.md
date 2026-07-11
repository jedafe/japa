# Production-Readiness Audit — 2026-07-11 (late session)

**Verdict at audit time: NOT production ready.** Backend fully green; 3 of 5 frontend
apps could not produce a production build; two critical runtime bugs verified.
Feature-rich: yes. Bug-free: no.

Method: every quality gate re-run from scratch (no trust in prior logs), plus fresh
passes over functional, UI/UX, integration, performance, security, and logic categories.

---

## Gate results (as found)

| Gate | Result |
|---|---|
| `go build ./...` | ✅ pass |
| `go vet ./...` | ✅ pass |
| `go test ./...` | ✅ pass (all packages) |
| `make lint` (golangci-lint) | ❌ 1042 issues (~80% style: wsl 381, revive 135; substantive: errcheck 50, gosec 28, staticcheck 6, unused 6, nilerr 1, noctx 2) |
| vitest main | ✅ 209/209 (49 files) |
| vitest admin | ✅ 72/72 (24 files) |
| vitest control | ✅ 30/30 (10 files) |
| vitest learn | ✅ 2/2 |
| vitest docs | ✅ 12/12 |
| svelte-check main | ❌ 15 errors / 7 warnings |
| svelte-check admin | ❌ 12 errors / 1 warning |
| svelte-check control | ✅ 0 errors / 17 warnings |
| svelte-check learn | ❌ 2 errors |
| svelte-check docs | ✅ 0 errors / 4 warnings |
| `vite build` main | ❌ FAIL — 6 MISSING_EXPORT (`$env/static/public`) |
| `vite build` admin | ❌ FAIL — 1 MISSING_EXPORT (`PUBLIC_JC_WS_URL`) |
| `vite build` learn | ❌ FAIL — 2 MISSING_EXPORT (`PUBLIC_JC_UMAMI_*`) |
| `vite build` control | ✅ pass |
| `vite build` docs | ✅ pass |

Root cause of test-vs-build divergence: vitest mocks `$env/static/public` and stores,
so 523 green tests never exercised the env contract or store wiring.

---

## 🔴 Blockers

### B1. Production builds fail for main, admin, learn — env contract incomplete
Code imports `PUBLIC_JC_API_URL / WS_URL / SITE_URL / GLITCHTIP_DSN / UMAMI_URL /
UMAMI_SITE_ID` from `$env/static/public`, but:
- `apps/main/.env` still has only obsolete `VITE_*` names (no `PUBLIC_JC_*` at all)
- `apps/admin/.env` missing `PUBLIC_JC_WS_URL`
- `apps/learn/.env` missing `PUBLIC_JC_UMAMI_URL`, `PUBLIC_JC_UMAMI_SITE_ID`
- `.env.example` files do not document the full contract for any app

### B2. main SSR API base double-suffixed → all server-side calls 404
`apps/main/src/lib/server/api.ts:8` appends `/api/v1`, but `apps/main/.env` sets
`PRIVATE_API_BASE_URL=http://localhost:8080/api/v1` → `/api/v1/api/v1/...`.
(The ENV-3 code fix landed; the config never followed.)

### B3. Admin permission gating bricked for ALL users (incl. super-admins)
Two duplicate `hasPermission` implementations:
- `+layout.ts` calls `fetchPermissions()` → fills `$lib/stores/permissions` store
- `+layout.svelte:3` imports `hasPermission` from `$lib/stores/auth` (different store),
  which is set from `data.permissions ?? []` — but the universal `+layout.ts` load
  returns only `{ token, user }`, so it is ALWAYS `[]`
→ All 13 gated sections (`/users`, `/content`, `/moderation`, …) render AccessDenied.

### B4. main organisations/[slug]/manage page: every action throws
Page calls `api('/organizations/…', {...})` as a function; main's `$lib/api` exports
`api` as a method object (no call signature — callable only in the admin app; page
likely copy-pasted). Save settings / invite / remove member / change role all throw
TypeError, swallowed by try/catch.

### B5. CI gates backend only; deploy is a stub
`.github/workflows/ci.yml` runs only `go build` + `go test` — no frontend
check/test/build. `deploy.yml` is an `echo` placeholder. None of B1–B4 could be
caught by the pipeline as configured.

---

## 🟡 Non-blocking bugs (verified)

1. admin users/[userId]/+page.svelte:156 — sends `{ groups }` where API expects
   `{ group }` (update silently no-ops)
2. admin organisations/+page.svelte:145-146 — renders `org.domain`, not on the
   `Organisation` type (renders undefined)
3. admin reactions/+page.svelte:253 — renders `.name`, not on `ReactionType`
4. admin spam/+page.svelte:150 + users/[userId]/+page.svelte:132 — API helpers
   called with 1 arg where 2 expected
5. admin +layout.svelte:57 — `wsClient.connect(data.token)` type mismatch
   (`string|null` vs `string|undefined`)
6. admin gdpr/+page.svelte:96, moderation/alerts/+page.svelte:143 — required
   `children` snippet missing on component usage
7. main organisations/[slug]/manage/+page.server.ts:44 — `error(404, string)`
   doesn't satisfy `App.Error` overloads
8. main manage page: unknown `description` prop (dropped) + invalid Badge
   `variant="primary"` (falls back to default)
9. Backend lint: 50 errcheck + 6 staticcheck worth triage; gosec 28 are
   low-severity int→int32 conversions + test-only items

## ✅ Security (strong — no critical findings)
- SQLi structurally impossible: 100% sqlc-generated parameterized pgx; zero
  string-built SQL
- XSS: all 7 `{@html}` sites individually verified safe (escaping, whitelist
  sanitizer on write, or trusted admin-only RBAC-gated content)
- bcrypt cost 12; cookies HttpOnly+Secure+SameSite=Strict; tight Redis-backed
  rate limits on `/auth/*`; CSP/X-Frame-Options/nosniff headers; env-driven CORS
  allowlist; no hardcoded secrets

## ✅ Feature surface
63 backend domains, ~859 registered routes, 5 SvelteKit apps, 99 migrations
(65 with indexes), e2e specs for all apps. Known deferred stubs (unchanged from
morning audit): MediaEmbed fetch-by-ID, profile content feed, control queue jobs
list, `GET /admin/ai/usage` stub, wallet synchronous task placeholder.

---

## Resolution checklist

- [x] B1: complete `.env` + `.env.example` for all five apps (`PUBLIC_JC_*` set)
- [x] B2: strip `/api/v1` from main `PRIVATE_API_BASE_URL`
- [x] B3: single source of truth for admin permissions; layout gate reads real data
- [x] B4: rewrite org-manage page against main's object-style API
- [x] B5: add frontend check/test/build jobs to CI
- [x] Non-blocking 1–8 (type errors → 0 across all apps)
- [x] Verify: check+test+build green ×5 apps; backend gates still green
      (2026-07-12: all 15 frontend gates OK — main/admin/control/learn/docs
      each check=OK test=OK build=OK; `go build`/`go vet` OK)

## RESOLUTION LOG — 2026-07-12

### B1+B2 — env contract (fixed, build-verified)
All five `apps/*/.env` and `.env.example` rewritten to document the full
contract: `PRIVATE_API_BASE_URL` (host only, code appends `/api/v1`),
`JC_COOKIE_DOMAIN` (commented for dev), and each app's exact
`$env/static/public` import set. main's obsolete `VITE_*` vars removed.
main/admin/learn `vite build` re-run: all pass.

### B3 — admin permissions (fixed)
- `$lib/stores/permissions` is now the single source of truth;
  `$lib/stores/auth` re-exports the same store (no more duplicate list).
- `permissions.ts` `hasPermission`/`hasAny`/`hasAll` now honor the `'*'`
  super-admin wildcard (previously only auth.ts did — Sidebar/AI pages
  would have ignored wildcard admins).
- `fetchPermissions()` returns the list; `+layout.ts` returns it as
  `data.permissions`; dead `user`/`setPermissions` imports removed.
- `+layout.svelte` gate now subscribes reactively via `$permissions`
  (plain function call in `$derived` was untracked).
- `wsClient.connect(data.token ?? undefined)` type fix.

### B4 — main org-manage page (fixed)
All calls now use the object API: `api.updateOrganization`,
`api.inviteToOrganization`, `api.listOrganizationInvitations`,
`api.removeOrganizationMember`, `api.updateOrganizationMember`.
Badge variant `primary`→`info`; EmptyState `description`→`message`;
`error(404, {code, message})` conforms to `App.Error`; local Invitation
type aligned (optional timestamps).

### Non-blocking drift — fixed (several were worse than typed)
1. **users/[userId] group change**: was `{groups}` vs helper `{group}` vs
   backend `{group_id}` (three-way drift; PATCH endpoint is add-only).
   Rewrote as a diff against the RBAC membership routes:
   `POST /admin/user-groups/{id}/members` + `DELETE .../members/{userId}`;
   added `addGroupMember`/`removeGroupMember` helpers, removed broken
   `changeGroup`. suspendUser now sends `{status:'suspended'}`.
2. **admin spam page was fully non-functional** (audit typed-error was the
   tip): all three tabs parsed `{items,total_pages}` but backend returns
   bare `{"data":[...]}` arrays → always empty; IP-block create sent
   `ip_cidr` w/o required `block_type` → always 400; user-block delete
   needs composite key. Realigned types/parsing to backend models
   (`cidr`, `block_type`, `blocker_id`/`blocked_id`), added block-type
   select, composite-key unblock, removed dead pagination controls.
3. **admin EmptyState crashed when used self-closing** (unconditional
   `{@render children()}` — gdpr + moderation/alerts empty states would
   throw at runtime). children now optional + guarded.
4. organisations list: `org.domain` (nonexistent) → `org.slug`;
   reactions modal: `.name` → `.label` (backend field).
5. gdpr modal onConfirm null-guard for `selectedRequest`.

### B5 — CI (fixed)
`ci.yml` gained a `frontend` matrix job (main/admin/control/learn/docs):
bun install --frozen-lockfile → `cp .env.example .env` → check → test →
build. This gate would have caught every blocker in this audit.

### Type-check state after fixes
main 0 errors · admin 0 errors · control 0 errors · learn 0 errors/0 warnings · docs 0 errors
