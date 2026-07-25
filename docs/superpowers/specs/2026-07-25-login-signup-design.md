# Login & Signup — Design Spec

**Date:** 2026-07-25
**Surface:** JapaConnect Main App (`apps/main`, port 3000, SSR)
**Governing spec:** `.jeos/sections/02-authentication-sessions.md` (OAuth social login, Section 02 line 36)
**Related ADRs:** ADR-058 (unified app shell + closed exception list), ADR-032 (root route resolution)

## Goal

Implement login and signup for the Main App, supporting email/password and OAuth (Google, Apple, Facebook). New users land in the existing onboarding wizard; returning users land on the Primary timeline. Auth is reachable two ways: dedicated routes (`/login`, `/signup`) for direct navigation / external links, and a modal dialog triggered from the guest homepage and any "Sign in to continue" affordance in the shell.

## API-Readiness Gate

All endpoints verified against `.jeos/conventions/api-routes-catalog.md` and probed against the running backend on `localhost:8080`:

| Endpoint | Method | Status | Notes |
|---|---|---|---|
| `/auth/register` | POST | 400 (empty body) | exists, returns `{ access_token, user }` |
| `/auth/login` | POST | 400 (empty body) | exists |
| `/auth/logout` | POST | 200 | exists |
| `/auth/forgot-password` | POST | 400 (empty body) | exists |
| `/auth/reset-password` | GET / POST | 405 / 400 | exists |
| `/auth/verify-email` | GET | 405 (needs token) | exists |
| `/auth/resend-verification` | POST | 400 (empty body) | exists |
| `/auth/oauth/{provider}/initiate` | POST | 400 (empty body) | exists, returns `{ redirect_url }` |
| `/auth/oauth/{provider}/callback` | GET | — | exists, **but behavior is broken — see Backend Fixes** |
| `/onboarding/status` | GET | auth | exists, used to decide post-signup redirect |

CSRF: `/api/v1/auth/*` is exempt from CSRF middleware (`internal/server/server.go:762`), so auth proxy endpoints need no CSRF token. The `jc_refresh` cookie is set HttpOnly by the backend, scoped to `/api/v1/auth`.

## Backend Fixes (governed by Section 02 line 36)

The spec is the source of truth: *"On first OAuth login, create account if no matching email exists; if a matching email exists, merge the OAuth identity onto the existing account."* The current backend violates this and returns JSON from the browser-facing callback. Both must be fixed.

### Fix 1 — `HandleCallback` creates account on first OAuth login
`internal/auth/oauth_handlers.go` `HandleCallback`:
- Existing OAuth link → issue token (already works)
- Matching email exists → link `oauth_accounts` row to existing user, issue token (already works)
- **No existing link AND no matching email → create user** with `password_hash = NULL` (OAuth-only account per Section 02 schema, line 105), generate a unique username from email local-part + provider suffix, create `oauth_accounts` row, issue token. **This replaces the current `account_not_found` 403.**

### Fix 2 — `HandleCallback` redirects to the frontend
The provider redirects the browser to the backend callback URL (`{RedirectBase}/api/v1/auth/oauth/{provider}/callback`). Returning JSON renders raw text in a browser. Standard OAuth pattern:
- **Success:** set `jc_token` + `jc_refresh` cookies, then `http.Redirect(w, r, frontendBase + redirectPath, http.StatusFound)`. `redirectPath` is `/onboarding` for new users (check onboarding status), else the `redirect` value stored at initiate time, defaulting to `/`.
- **Error:** `http.Redirect(w, r, frontendBase + "/login?oauth_error=" + code, http.StatusFound)`.
- `frontendBase` from `JC_FRONTEND_BASE_URL` env var (Rule 3 — no hardcoded URLs).

### Fix 3 — `HandleInitiate` accepts + forwards `redirect`
`HandleInitiate` currently takes only `provider`. Add a `redirect` query/body field, store it alongside the OAuth state cookie (short-lived, signed), so `HandleCallback` knows where to send the user after success.

### Catalog update
`.jeos/conventions/api-routes-catalog.md` — update the OAuth callback note: now performs browser redirect (not JSON), supports first-time signup per Section 02.

## Architecture Decisions

### AD1 — Auth routes are a 4th shell exception
ADR-058's closed list of shell exceptions is: blog single, `/create`, guest homepage. Login/signup are not listed. This spec **extends the closed list** with a 4th exception: the `(auth)` route group renders a centered-card layout, not `AppShell`. Rationale: auth surfaces are pre-session; showing the full shell (Compose, Notifications, left nav) to a logged-out user is incoherent. The exception is narrowly scoped to `(auth)/*` routes only.

### AD2 — Hybrid: dedicated routes + modal, shared form components
- **Direct navigation / external links / email deep-links** → `/login` or `/signup` routes (4th exception layout).
- **Guest browsing the site** → "Sign in" / "Get started" buttons in the guest homepage header and shell affordances open `AuthDialog.svelte` (Bits UI Dialog) without a route change.
- Both surfaces render the **same** `LoginForm.svelte` / `SignupForm.svelte` presentational components. Single source of truth for fields, validation, OAuth buttons, error display.

### AD3 — Server-side cookie proxy for the modal
The modal runs client-side and cannot set HttpOnly cookies. Both the modal and the route form actions go through **server-side proxy endpoints** that call the backend, set the `jc_token` cookie, and return the user. Routes use SvelteKit form actions (`?/login`, `?/signup`); the modal `fetch`es `/api/auth/*` `+server.ts` endpoints. Both call the same internal `authenticateLogin` / `authenticateSignup` helpers in `$lib/server/auth.ts`.

### AD4 — Post-signup: onboarding wizard first
`POST /auth/register` returns a session immediately and sends a verification email; email verification is **not** required to log in (Section 02 — verification is a separate, non-blocking flow). After successful signup or first-time OAuth login, the user is redirected to `/onboarding` (the existing onboarding wizard, backed by `/onboarding/*` endpoints). Returning users go to the `?redirect=` target or `/`. A dismissible "verify your email" banner with a Resend link shows on the Primary timeline until `email_verified_at` is set.

## Routing & Layout Structure

```
src/routes/
├── (app)/            ← existing shell routes (unchanged)
└── (auth)/           ← NEW — 4th exception, centered-card layout
    ├── +layout.server.ts        # auth gate: redirect authed → /onboarding (if incomplete) or ?redirect or /
    ├── +layout.svelte           # AuthShell.svelte wrapper
    ├── login/
    │   ├── +page.server.ts      # ?/login form action + load
    │   └── +page.svelte
    ├── signup/
    │   ├── +page.server.ts      # ?/signup form action + load
    │   └── +page.svelte
    ├── forgot-password/
    │   ├── +page.server.ts
    │   └── +page.svelte
    ├── reset-password/
    │   ├── +page.server.ts      # load: validate ?token= via GET /auth/reset-password
    │   └── +page.svelte
    └── verify-email/
        ├── +page.server.ts      # load: call GET /auth/verify-email?token=
        └── +page.svelte

src/routes/api/auth/              # server-side proxies for the modal
├── login/+server.ts              # POST → /auth/login, set jc_token, return { user, redirect }
├── signup/+server.ts             # POST → /auth/register, set jc_token, return { user, redirect: '/onboarding' }
├── logout/+server.ts             # POST → /auth/logout, clear jc_token
└── oauth/+server.ts              # GET ?provider=&redirect= → POST /auth/oauth/{provider}/initiate, return { redirect_url }
```

**`(auth)/+layout.server.ts`** — auth gate: if `event.locals.user` is present **and the route is not `verify-email`** (authed users legitimately click email verification links), fetch `GET /onboarding/status`; if `completed === false`, `throw redirect(303, '/onboarding')`; else `throw redirect(303, searchParams.get('redirect') ?? '/')`. Logged-out users pass through. The `verify-email` route is exempt from the gate so authed users can complete verification.

**`(auth)/+layout.svelte`** — `AuthShell.svelte`: full-viewport flex center, `--color-bg` background, brand logo + tagline above the card, the card (surface bg, 1px border, 12px radius, max-width 440px, ~1.5rem padding), and a footer slot for legal links. **Not** `AppShell`. Theme attribute already on `<html>` from `hooks.server.ts` (dark mode works with no extra wiring; never Tailwind `dark:`).

## Component Architecture

```
src/lib/components/auth/
├── AuthShell.svelte        # centered-card wrapper (route layout only)
├── LoginForm.svelte        # presentational — props: redirectTarget?, onSuccess: (user) => Promise<void>
├── SignupForm.svelte       # presentational — props: redirectTarget?, onSuccess: (user) => Promise<void>
├── AuthDialog.svelte       # Bits UI Dialog hosting LoginForm | SignupForm via internal tab
├── OAuthButtons.svelte     # Google / Apple / Facebook — shared by both forms
├── PasswordField.svelte    # Bits UI input + show/hide toggle + strength meter (signup)
└── AuthFooter.svelte       # switch between login/signup links + legal links
```

**Design principles:**
- Forms are **pure presentational components** — they take `redirectTarget?` and an `onSuccess` callback, call the API, and on success invoke `onSuccess(session)`. They never know whether they're in a route or a modal.
- Routes pass `goto(redirect)` as `onSuccess`; the modal passes `() => { invalidateAll(); dialog.close(); }`.
- **Validation** runs client-side (Zod schemas in `$lib/auth/schemas.ts`) for instant feedback; the backend is the source of truth. `ApiError.code` maps to field-level messages.
- **`OAuthButtons`** calls `/api/auth/oauth?provider=…&redirect=…`, gets `{ redirect_url }`, `window.location.assign(redirect_url)`. No frontend OAuth callback route — the backend redirects back to the frontend with cookies set (after Backend Fix 2).
- **`PasswordField`** uses Bits UI; the strength meter on signup reflects password policy (length, character classes). Breach check is server-side only — the breach DB is not shipped to the client.

**State management (Svelte 5 Runes):**
```svelte
let { redirectTarget, onSuccess }: Props = $props();
let email = $state('');
let password = $state('');
let loading = $state(false);
let errors = $state<{ email?: string; password?: string; form?: string }>({});

async function submit() {
  errors = {};
  loading = true;
  try {
    const res = await api.post('/auth/login', { email, password });
    if (res.error) { mapApiError(res.error, errors); return; }
    setSessionCookie(res.data.access_token);
    await onSuccess(res.data.user);
  } finally { loading = false; }
}
```

## Data Flow

**Login (route, form action):**
1. User submits → `?/login` action in `login/+page.server.ts`
2. Action calls `authenticateLogin(event, { email, password })` → proxies to `POST /auth/login`, gets `{ access_token, user }`
3. Action sets `jc_token` cookie via `setSessionCookie(event, accessToken)`
4. Action checks `GET /onboarding/status` — if incomplete, `throw redirect(303, '/onboarding')`; else `throw redirect(303, redirectTarget ?? '/')`

**Login (modal):**
1. User submits → `fetch('/api/auth/login', { method: 'POST', body })`
2. `+server.ts` POST handler calls the same `authenticateLogin()`, sets cookie, returns `{ user, redirect }`
3. Client calls `invalidateAll()` (re-runs `hooks.server.ts` → `event.locals.user` populated) and `goto(redirect)` if onboarding needed, else closes dialog

**Signup** — identical, but the proxy always returns `redirect: '/onboarding'` for new users; both route and modal redirect there.

**Logout** — `POST /api/auth/logout` proxy clears `jc_token` + calls backend `POST /auth/logout`; then `invalidateAll()`.

**OAuth** — see Backend Fixes + AD2. Flow: `OAuthButtons` → `/api/auth/oauth` proxy → backend `/initiate` → provider → backend `/callback` (creates account if needed, sets cookies, redirects to frontend) → `hooks.server.ts` resolves session.

## Error Handling

`$lib/auth/errors.ts` exports `mapApiError(code) → { field, messageKey }`:

| `ApiError.code` | HTTP | Field | Message key |
|---|---|---|---|
| `missing_fields` | 400 | form | `auth.errors.missing_fields` |
| `invalid_credentials` | 401 | form | `auth.errors.invalid_credentials` |
| `email_exists` | 409 | email | `auth.errors.email_exists` |
| `weak_password` | 400 | password | `auth.errors.weak_password` |
| `password_breached` | 400 | password | `auth.errors.password_breached` |
| `invalid_body` | 400 | form | `auth.errors.invalid_body` |
| `tenant_unavailable` | 503 | form | `auth.errors.tenant_unavailable` |
| 429 (any) | 429 | form | `auth.errors.rate_limited` |
| `unsupported_provider` | 400 | form | `auth.errors.oauth_unsupported_provider` |
| `state_error` / `invalid_state` | 400 | form | `auth.errors.oauth_state_error` |

Backend already applies `mw.RateLimit("tight")` to `/auth/*` (`internal/server/server.go:774`). The frontend shows a generic "Too many attempts, try again in a minute" on 429.

## Types

Added to `$lib/types/index.ts`:
```ts
export interface AuthSession { access_token: string; user: AuthUser; }
export interface AuthUser { id: string; email: string; username: string; }
export type AuthErrorCode =
  | 'missing_fields' | 'invalid_credentials' | 'email_exists'
  | 'weak_password' | 'password_breached' | 'invalid_body'
  | 'tenant_unavailable' | 'unsupported_provider' | 'state_error' | 'invalid_state';
```

## Testing

**Frontend (Vitest + Testing Library Svelte):**
- `LoginForm.test.ts` — renders fields, validates via Zod, maps `ApiError.code` to field errors, calls `onSuccess` on 200, shows loading state
- `SignupForm.test.ts` — password strength meter, `email_exists` / `weak_password` / `password_breached` error mapping, calls `onSuccess`
- `AuthDialog.test.ts` — tab switches between login/signup, closes on success, opens via trigger
- `+page.server.ts` form action tests — redirect logic (onboarding incomplete → `/onboarding`, else `?redirect=`), cookie setting, gate redirect for authed users
- Mock the `/api/auth/*` proxy endpoints, not the backend directly

**Backend (Go table-driven + httptest, per jc-backend-develop):**
- `oauth_handlers_test.go` (new) — first OAuth login with no existing email creates account + links OAuth; existing email merges; existing link re-issues token; callback redirects to frontend (not JSON); error cases redirect to `/login?oauth_error=`
- `handlers_test.go` — existing register/login tests stay green

## i18n

Every user-facing string via `$t('key')`, added to `apps/main/src/lib/i18n/locales/en.json` AND `fr.json`:
- `auth.login.{title, subtitle, submit, switch_to_signup}`
- `auth.signup.{title, subtitle, submit, switch_to_login}`
- `auth.fields.{email, password, username, password_show, password_hide}`
- `auth.password.strength_{weak, fair, good, strong}`
- `auth.oauth.{continue_with_google, continue_with_apple, continue_with_facebook, or_continue_with}`
- `auth.errors.{missing_fields, invalid_credentials, email_exists, weak_password, password_breached, invalid_body, tenant_unavailable, rate_limited, oauth_unsupported_provider, oauth_state_error}`
- `auth.forgot_password.{title, submit, sent_confirmation}`
- `auth.reset_password.{title, submit, success, invalid_token, expired_token}`
- `auth.verify_email.{verifying, success, invalid_token, already_verified}`
- `auth.footer.{terms, privacy}`
- `auth.banner.{verify_email_prompt, resend, verification_sent}`

## File Inventory

**Frontend — `apps/main/src/`:**
- `routes/(auth)/+layout.server.ts`, `+layout.svelte`
- `routes/(auth)/login/+page.server.ts`, `+page.svelte`
- `routes/(auth)/signup/+page.server.ts`, `+page.svelte`
- `routes/(auth)/forgot-password/+page.server.ts`, `+page.svelte`
- `routes/(auth)/reset-password/+page.server.ts`, `+page.svelte`
- `routes/(auth)/verify-email/+page.server.ts`, `+page.svelte`
- `routes/api/auth/login/+server.ts`
- `routes/api/auth/signup/+server.ts`
- `routes/api/auth/logout/+server.ts`
- `routes/api/auth/oauth/+server.ts`
- `lib/server/auth.ts` — `setSessionCookie`, `clearSessionCookie`, `authenticateLogin`, `authenticateSignup`
- `lib/auth/schemas.ts` — Zod schemas
- `lib/auth/errors.ts` — `mapApiError`
- `lib/components/auth/{AuthShell, LoginForm, SignupForm, AuthDialog, OAuthButtons, PasswordField, AuthFooter}.svelte`
- `lib/components/auth/*.test.ts`
- `lib/types/index.ts` — additions
- `lib/i18n/locales/{en, fr}.json` — additions

**Backend — `internal/auth/`:**
- `oauth_handlers.go` — FIX: `HandleCallback` creates account on first login, redirects to frontend
- `oauth.go` — FIX: `HandleInitiate` accepts + forwards `redirect`
- `oauth_handlers_test.go` — NEW
- `internal/config/` — `JC_FRONTEND_BASE_URL` env var (Rule 3)

**Catalogs:**
- `.jeos/conventions/api-routes-catalog.md` — OAuth callback note update

## Verification Plan

- `cd apps/main && bun run test` — frontend unit/component tests
- `cd apps/main && bun run build` — SvelteKit build (typecheck + SSR)
- `go build ./... && go vet ./...` — backend compiles
- `go test ./internal/auth/...` — backend auth tests
- Manual: signup → onboarding → logout → login → forgot → reset → verify-email → OAuth Google

## Out of Scope

- Onboarding wizard itself (already designed + implemented in a worktree, backed by `/onboarding/*` endpoints)
- TOTP 2FA challenge flow (`/auth/totp/verify-challenge`, `/auth/totp/backup`) — separate task
- Settings → link OAuth account flow (existing `/me/*` settings surface)
- Session management UI (`GET /me/sessions`, `DELETE /me/sessions/{id}`) — separate task
